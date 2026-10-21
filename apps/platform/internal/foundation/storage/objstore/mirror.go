package objstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"time"
)

const (
	mirroredPrefix  = "objects/"
	tombstonePrefix = "tombstones/"
)

type MirrorPolicy struct {
	Now       time.Time
	Retention time.Duration
	Sample    int
}

type MirrorReport struct {
	Copied     int
	Tombstoned int
	Expired    int
	Revived    int
	Sampled    int
}

type mirrorRun struct {
	source, backup *Client
	policy         MirrorPolicy
	sourceSizes    map[string]int64
	mirrored       map[string]int64
	tombstones     map[string]int64
	report         MirrorReport
}

func Mirror(ctx context.Context, source, backup *Client, policy MirrorPolicy) (MirrorReport, error) {
	if source.Location() == backup.Location() {
		return MirrorReport{}, fmt.Errorf("the backup is the bucket it backs up (%s); it must be another location", source.Location())
	}
	run := &mirrorRun{source: source, backup: backup, policy: policy}
	steps := []func(context.Context) error{
		run.takeInventories, run.copyFromSource, run.ageOutGone, run.dropOrphanTombstones, run.verify,
	}
	for _, step := range steps {
		if err := step(ctx); err != nil {
			return run.report, err
		}
	}
	return run.report, nil
}

func (r *mirrorRun) takeInventories(ctx context.Context) error {
	var err error
	if r.sourceSizes, err = r.source.Inventory(ctx, ""); err != nil {
		return err
	}
	if r.mirrored, err = r.backup.Inventory(ctx, mirroredPrefix); err != nil {
		return err
	}
	r.tombstones, err = r.backup.Inventory(ctx, tombstonePrefix)
	return err
}

func (r *mirrorRun) copyFromSource(ctx context.Context) error {
	for _, key := range slices.Sorted(maps.Keys(r.sourceSizes)) {
		if size, ok := r.mirrored[mirroredPrefix+key]; !ok || size != r.sourceSizes[key] {
			if err := copyObject(ctx, r.source, r.backup, key); err != nil {
				return err
			}
			r.report.Copied++
		}
		if _, ok := r.tombstones[tombstonePrefix+key]; ok {
			if err := r.backup.Remove(ctx, tombstonePrefix+key); err != nil {
				return err
			}
			r.report.Revived++
		}
	}
	return nil
}

func (r *mirrorRun) ageOutGone(ctx context.Context) error {
	for _, mirroredKey := range slices.Sorted(maps.Keys(r.mirrored)) {
		key := strings.TrimPrefix(mirroredKey, mirroredPrefix)
		if _, ok := r.sourceSizes[key]; ok {
			continue
		}
		_, alreadyGone := r.tombstones[tombstonePrefix+key]
		expired, err := ageOut(ctx, r.backup, key, r.tombstones, r.policy)
		switch {
		case err != nil:
			return err
		case expired:
			r.report.Expired++
		case !alreadyGone:
			r.report.Tombstoned++
		}
	}
	return nil
}

func (r *mirrorRun) dropOrphanTombstones(ctx context.Context) error {
	for _, tombstone := range slices.Sorted(maps.Keys(r.tombstones)) {
		key := strings.TrimPrefix(tombstone, tombstonePrefix)
		_, inSource := r.sourceSizes[key]
		_, inBackup := r.mirrored[mirroredPrefix+key]
		if !inSource && !inBackup {
			if err := r.backup.Remove(ctx, tombstone); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *mirrorRun) verify(ctx context.Context) error {
	if err := requireComplete(ctx, r.backup, r.sourceSizes); err != nil {
		return err
	}
	var err error
	r.report.Sampled, err = compareSample(ctx, r.source, r.backup, r.sourceSizes, r.policy)
	return err
}

func copyObject(ctx context.Context, source, backup *Client, key string) error {
	body, size, err := source.Open(ctx, key)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	return backup.PutFrom(ctx, mirroredPrefix+key, body, size)
}

func ageOut(ctx context.Context, backup *Client, key string, tombstones map[string]int64, policy MirrorPolicy) (bool, error) {
	tombstone := tombstonePrefix + key
	if _, ok := tombstones[tombstone]; !ok {
		return false, backup.Put(ctx, tombstone, []byte(policy.Now.UTC().Format(time.RFC3339Nano)))
	}
	recorded, err := backup.Get(ctx, tombstone)
	if err != nil {
		return false, err
	}
	goneSince, err := time.Parse(time.RFC3339Nano, string(recorded))
	if err != nil {
		return false, fmt.Errorf("backup tombstone %s does not hold a time: %w", tombstone, err)
	}
	if policy.Now.Sub(goneSince) < policy.Retention {
		return false, nil
	}
	if err := backup.Remove(ctx, mirroredPrefix+key); err != nil {
		return false, err
	}
	return true, backup.Remove(ctx, tombstone)
}

func requireComplete(ctx context.Context, backup *Client, sourceSizes map[string]int64) error {
	mirrored, err := backup.Inventory(ctx, mirroredPrefix)
	if err != nil {
		return err
	}
	var missing []string
	for _, key := range slices.Sorted(maps.Keys(sourceSizes)) {
		if size, ok := mirrored[mirroredPrefix+key]; !ok || size != sourceSizes[key] {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("the backup is missing %d object(s) the source holds, first %s", len(missing), missing[0])
	}
	return nil
}

func compareSample(ctx context.Context, source, backup *Client, sourceSizes map[string]int64, policy MirrorPolicy) (int, error) {
	sample := dailySample(slices.Collect(maps.Keys(sourceSizes)), policy.Now, policy.Sample)
	for _, key := range sample {
		want, err := digest(ctx, source, key)
		if err != nil {
			return 0, err
		}
		got, err := digest(ctx, backup, mirroredPrefix+key)
		if err != nil {
			return 0, err
		}
		if !bytes.Equal(want, got) {
			return 0, fmt.Errorf("the backup copy of %s differs from the source", key)
		}
	}
	return len(sample), nil
}

// dailySample orders keys by a digest salted with the day, so each day checks
// a different slice of the bucket while a rerun on the same day checks the same one.
func dailySample(keys []string, now time.Time, size int) []string {
	day := now.UTC().Format(time.DateOnly)
	rank := func(key string) []byte { sum := sha256.Sum256([]byte(day + "\x00" + key)); return sum[:] }
	slices.SortFunc(keys, func(a, b string) int { return bytes.Compare(rank(a), rank(b)) })
	return keys[:min(size, len(keys))]
}

func digest(ctx context.Context, store *Client, key string) ([]byte, error) {
	body, _, err := store.Open(ctx, key)
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, body); err != nil {
		return nil, fmt.Errorf("objstore read %s: %w", key, err)
	}
	return hash.Sum(nil), nil
}
