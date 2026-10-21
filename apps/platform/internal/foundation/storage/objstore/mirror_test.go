package objstore

import (
	"context"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"
)

const backupRetention = 35 * 24 * time.Hour

var mirrorStart = time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)

func newMirrorPair(t *testing.T) (source, backup *Client) {
	t.Helper()
	ctx := context.Background()
	stores := make([]*Client, 2)
	for i, bucket := range []string{"skillhub", "skillhub-backup"} {
		store, stop, err := NewInProcess(bucket)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(stop)
		if err := store.EnsureBucket(ctx); err != nil {
			t.Fatal(err)
		}
		stores[i] = store
	}
	return stores[0], stores[1]
}

func put(t *testing.T, store *Client, key, body string) {
	t.Helper()
	if err := store.Put(context.Background(), key, []byte(body)); err != nil {
		t.Fatal(err)
	}
}

func mirrorAt(t *testing.T, source, backup *Client, now time.Time) MirrorReport {
	t.Helper()
	report, err := Mirror(context.Background(), source, backup, MirrorPolicy{Now: now, Retention: backupRetention, Sample: 10})
	if err != nil {
		t.Fatalf("Mirror at %s: %v", now, err)
	}
	return report
}

func backupKeys(t *testing.T, backup *Client) []string {
	t.Helper()
	sizes, err := backup.Inventory(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	return slices.Sorted(maps.Keys(sizes))
}

func TestMirrorCopiesEverySourceObjectOnceUnderTheObjectsPrefix(t *testing.T) {
	source, backup := newMirrorPair(t)
	put(t, source, "packages/a.zip", "package a")
	put(t, source, "purge-ledger/user-1", "purge")

	first := mirrorAt(t, source, backup, mirrorStart)
	second := mirrorAt(t, source, backup, mirrorStart.Add(24*time.Hour))

	if first.Copied != 2 || second.Copied != 0 {
		t.Fatalf("copied = %d then %d, want 2 then 0", first.Copied, second.Copied)
	}
	if got, want := backupKeys(t, backup), []string{"objects/packages/a.zip", "objects/purge-ledger/user-1"}; !slices.Equal(got, want) {
		t.Fatalf("backup keys = %v, want %v", got, want)
	}
	copied, err := backup.Get(context.Background(), "objects/packages/a.zip")
	if err != nil || string(copied) != "package a" {
		t.Fatalf("backup copy = %q, %v", copied, err)
	}
}

func TestMirrorCopiesAgainAnObjectWhoseSizeChanged(t *testing.T) {
	source, backup := newMirrorPair(t)
	put(t, source, "ledger", "v1")
	mirrorAt(t, source, backup, mirrorStart)
	put(t, source, "ledger", "version two")

	report := mirrorAt(t, source, backup, mirrorStart.Add(time.Hour))

	copied, err := backup.Get(context.Background(), "objects/ledger")
	if report.Copied != 1 || err != nil || string(copied) != "version two" {
		t.Fatalf("copied = %d, backup = %q, %v", report.Copied, copied, err)
	}
}

func TestMirrorKeepsADeletedObjectUntilItHasBeenGoneForTheRetention(t *testing.T) {
	cases := map[string]struct {
		goneFor     time.Duration
		wantExpired bool
	}{
		"one nanosecond short of the retention": {backupRetention - time.Nanosecond, false},
		"exactly the retention":                 {backupRetention, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			source, backup := newMirrorPair(t)
			put(t, source, "datasets/d1.csv", "rows")
			mirrorAt(t, source, backup, mirrorStart)
			if err := source.Remove(context.Background(), "datasets/d1.csv"); err != nil {
				t.Fatal(err)
			}

			noticed := mirrorAt(t, source, backup, mirrorStart.Add(time.Hour))
			if noticed.Tombstoned != 1 || noticed.Expired != 0 {
				t.Fatalf("first run after deletion = %+v, want one tombstone and nothing expired", noticed)
			}
			later := mirrorAt(t, source, backup, mirrorStart.Add(time.Hour+tc.goneFor))

			want := []string{"objects/datasets/d1.csv", "tombstones/datasets/d1.csv"}
			if tc.wantExpired {
				want = nil
			}
			if got := backupKeys(t, backup); !slices.Equal(got, want) || (later.Expired == 1) != tc.wantExpired {
				t.Fatalf("after %s: keys = %v, expired = %d", tc.goneFor, got, later.Expired)
			}
		})
	}
}

func TestMirrorForgetsTheDeletionOfAnObjectThatCameBack(t *testing.T) {
	source, backup := newMirrorPair(t)
	put(t, source, "packages/a.zip", "a")
	mirrorAt(t, source, backup, mirrorStart)
	if err := source.Remove(context.Background(), "packages/a.zip"); err != nil {
		t.Fatal(err)
	}
	mirrorAt(t, source, backup, mirrorStart.Add(time.Hour))
	put(t, source, "packages/a.zip", "a")

	revived := mirrorAt(t, source, backup, mirrorStart.Add(2*time.Hour))
	afterRetention := mirrorAt(t, source, backup, mirrorStart.Add(2*time.Hour+backupRetention))

	if revived.Revived != 1 || afterRetention.Expired != 0 {
		t.Fatalf("revived = %d, expired after the retention = %d", revived.Revived, afterRetention.Expired)
	}
	if got := backupKeys(t, backup); !slices.Equal(got, []string{"objects/packages/a.zip"}) {
		t.Fatalf("backup keys = %v", got)
	}
}

func TestMirrorDropsATombstoneWhoseObjectIsGoneFromBothSides(t *testing.T) {
	source, backup := newMirrorPair(t)
	put(t, backup, "tombstones/old.zip", mirrorStart.Format(time.RFC3339Nano))

	mirrorAt(t, source, backup, mirrorStart)

	if got := backupKeys(t, backup); len(got) != 0 {
		t.Fatalf("backup keys = %v, want none", got)
	}
}

func TestMirrorRefusesABackupThatIsTheSourceBucket(t *testing.T) {
	source, _ := newMirrorPair(t)
	put(t, source, "packages/a.zip", "a")

	_, err := Mirror(context.Background(), source, source, MirrorPolicy{Now: mirrorStart, Retention: backupRetention})

	if err == nil || !strings.Contains(err.Error(), "must be another location") {
		t.Fatalf("error = %v", err)
	}
	if keys, _ := source.List(context.Background(), ""); !slices.Equal(keys, []string{"packages/a.zip"}) {
		t.Fatalf("source was written: %v", keys)
	}
}

func TestMirrorRefusesATombstoneThatHoldsNoTime(t *testing.T) {
	source, backup := newMirrorPair(t)
	put(t, backup, "objects/gone.zip", "x")
	put(t, backup, "tombstones/gone.zip", "yesterday")

	_, err := Mirror(context.Background(), source, backup, MirrorPolicy{Now: mirrorStart, Retention: backupRetention})

	if err == nil || !strings.Contains(err.Error(), "tombstones/gone.zip does not hold a time") {
		t.Fatalf("error = %v", err)
	}
}

func TestMirrorFailsWhenASampledBackupCopyDiffersFromTheSource(t *testing.T) {
	source, backup := newMirrorPair(t)
	put(t, source, "packages/a.zip", "aaaa")
	mirrorAt(t, source, backup, mirrorStart)
	put(t, backup, "objects/packages/a.zip", "bbbb")

	_, err := Mirror(context.Background(), source, backup, MirrorPolicy{Now: mirrorStart.Add(time.Hour), Retention: backupRetention, Sample: 1})

	if err == nil || !strings.Contains(err.Error(), "backup copy of packages/a.zip differs") {
		t.Fatalf("error = %v", err)
	}
}

func TestRequireCompleteNamesASourceObjectTheBackupLacks(t *testing.T) {
	_, backup := newMirrorPair(t)
	put(t, backup, "objects/a", "1")

	err := requireComplete(context.Background(), backup, map[string]int64{"a": 1, "b": 1})

	if err == nil || !strings.Contains(err.Error(), "missing 1 object(s) the source holds, first b") {
		t.Fatalf("error = %v", err)
	}
}

func TestDailySampleIsStableWithinADayCappedAndRotatesAcrossDays(t *testing.T) {
	keys := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	today := dailySample(slices.Clone(keys), mirrorStart, 3)
	again := dailySample(slices.Clone(keys), mirrorStart.Add(5*time.Hour), 3)
	if len(today) != 3 || !slices.Equal(today, again) {
		t.Fatalf("same day samples = %v and %v", today, again)
	}
	rotated := false
	for day := 1; day <= 7 && !rotated; day++ {
		rotated = !slices.Equal(today, dailySample(slices.Clone(keys), mirrorStart.AddDate(0, 0, day), 3))
	}
	if !rotated {
		t.Fatal("the sample never changed across a week")
	}
	if all := dailySample(slices.Clone(keys), mirrorStart, 20); len(all) != len(keys) {
		t.Fatalf("a sample larger than the bucket = %d keys", len(all))
	}
}
