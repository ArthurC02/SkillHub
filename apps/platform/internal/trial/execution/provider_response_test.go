package run

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestArtifactManifestCannotNameAnObjectOutsideItsWriteGrant(t *testing.T) {
	d := &driver{cur: gen.Run{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}}}
	attempt := gen.RunAttempt{ID: pgtype.UUID{Bytes: [16]byte{2}, Valid: true}}
	pr := ProviderRun{Result: &RunResult{Artifacts: []RunArtifact{{
		FileName: "output.txt", ObjectKey: "datasets/another-workspace/private.csv",
	}}}}

	err := d.recordArtifacts(context.Background(), attempt, pr)
	if err == nil || !strings.Contains(err.Error(), "outside its write grant") {
		t.Fatalf("arbitrary provider object key was accepted: %v", err)
	}
}

func TestArtifactManifestIsValidatedBeforePersistence(t *testing.T) {
	validHash := strings.Repeat("0", 64)
	for _, tc := range []struct {
		name     string
		artifact RunArtifact
	}{
		{"empty name", RunArtifact{ContentHash: validHash}},
		{"leading dash", RunArtifact{FileName: "-output.txt", ContentHash: validHash}},
		{"traversal name", RunArtifact{FileName: "../secret", ContentHash: validHash}},
		{"control name", RunArtifact{FileName: "bad\nname", ContentHash: validHash}},
		{"negative size", RunArtifact{FileName: "out.txt", SizeBytes: -1, ContentHash: validHash}},
		{"oversized file", RunArtifact{FileName: "out.txt", SizeBytes: DefaultResourceLimits().ArtifactFileBytes + 1, ContentHash: validHash}},
		{"bad hash", RunArtifact{FileName: "out.txt", ContentHash: "not-sha256"}},
		{"bad content type", RunArtifact{FileName: "out.txt", ContentHash: validHash, ContentType: "bad\nvalue"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &driver{cur: gen.Run{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}}}
			err := d.recordArtifacts(context.Background(), gen.RunAttempt{}, ProviderRun{
				Result: &RunResult{Artifacts: []RunArtifact{tc.artifact}},
			})
			if err == nil || strings.Contains(err.Error(), "persistence is not configured") {
				t.Fatalf("invalid manifest reached persistence: %v", err)
			}
		})
	}
}

func TestArtifactManifestRejectsDuplicateNamesAndTotalOverflow(t *testing.T) {
	validHash := strings.Repeat("0", 64)
	d := &driver{cur: gen.Run{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}}}
	for name, artifacts := range map[string][]RunArtifact{
		"duplicates": {
			{FileName: "same.txt", ContentHash: validHash},
			{FileName: "same.txt", ContentHash: validHash},
		},
		"portable duplicates": {
			{FileName: "Report.txt", ContentHash: validHash},
			{FileName: "report.txt", ContentHash: validHash},
		},
		"total": {
			{FileName: "a", SizeBytes: 25 << 20, ContentHash: validHash},
			{FileName: "b", SizeBytes: 25 << 20, ContentHash: validHash},
			{FileName: "c", SizeBytes: 25 << 20, ContentHash: validHash},
			{FileName: "d", SizeBytes: 25 << 20, ContentHash: validHash},
			{FileName: "e", SizeBytes: 1, ContentHash: validHash},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := d.recordArtifacts(context.Background(), gen.RunAttempt{}, ProviderRun{
				Result: &RunResult{Artifacts: artifacts},
			}); err == nil || strings.Contains(err.Error(), "persistence is not configured") {
				t.Fatalf("invalid manifest reached persistence: %v", err)
			}
		})
	}
}

func TestArtifactManifestUsesTheRunFrozenLimits(t *testing.T) {
	validHash := strings.Repeat("0", 64)
	withLimits := func(fileLimit int64) *driver {
		policy, err := json.Marshal(policySnapshot{ResourceLimits: ResourceLimits{
			ArtifactFileBytes: fileLimit, ArtifactTotalBytes: fileLimit,
		}})
		if err != nil {
			t.Fatal(err)
		}
		return &driver{cur: gen.Run{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, PolicySnapshot: policy}}
	}
	artifact := func(size int64) ProviderRun {
		return ProviderRun{Result: &RunResult{Artifacts: []RunArtifact{{
			FileName: "out.txt", SizeBytes: size, ContentHash: validHash,
		}}}}
	}

	if err := withLimits(1).recordArtifacts(context.Background(), gen.RunAttempt{}, artifact(2)); err == nil || !strings.Contains(err.Error(), "invalid artifact size") {
		t.Fatalf("lower frozen limit was ignored: %v", err)
	}
	aboveDefault := DefaultResourceLimits().ArtifactFileBytes + 1
	if err := withLimits(aboveDefault).recordArtifacts(context.Background(), gen.RunAttempt{}, artifact(aboveDefault)); err == nil || !strings.Contains(err.Error(), "persistence is not configured") {
		t.Fatalf("higher frozen limit was replaced by today's default: %v", err)
	}
}

func TestArtifactTruncationAcceptsBothProviderContractGenerations(t *testing.T) {
	if !artifactCollectionTruncated(&RunResult{ArtifactsTruncated: true}) {
		t.Fatal("current aggregate truncation marker was ignored")
	}
	if !artifactCollectionTruncated(&RunResult{Artifacts: []RunArtifact{{Truncated: true}}}) {
		t.Fatal("legacy per-entry truncation marker was ignored during rolling deployment")
	}
	if artifactCollectionTruncated(&RunResult{Artifacts: []RunArtifact{{}}}) {
		t.Fatal("a complete artifact collection was reported as truncated")
	}
}

type manifestQueryRecorder struct {
	calls    []string
	recorded []string
	inserted []gen.InsertRunArtifactParams
}

func (q *manifestQueryRecorder) lock(context.Context, pgtype.UUID) error {
	q.calls = append(q.calls, "lock")
	return nil
}
func (q *manifestQueryRecorder) markTruncated(context.Context, gen.MarkRunArtifactsTruncatedParams) (int64, error) {
	q.calls = append(q.calls, "mark")
	return 1, nil
}
func (q *manifestQueryRecorder) recordedNames(context.Context, gen.ListRunArtifactFileNamesParams) ([]string, error) {
	q.calls = append(q.calls, "read-names")
	return q.recorded, nil
}
func (q *manifestQueryRecorder) insert(_ context.Context, p gen.InsertRunArtifactParams) error {
	q.calls = append(q.calls, "insert")
	q.inserted = append(q.inserted, p)
	return nil
}
func (q *manifestQueryRecorder) retireIntent(context.Context, string) error {
	q.calls = append(q.calls, "retire-intent")
	return nil
}

func TestArtifactManifestLocksBeforeAnyWrite(t *testing.T) {
	q := &manifestQueryRecorder{}
	current := gen.Run{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}}
	result := &RunResult{Artifacts: []RunArtifact{{FileName: "out.txt"}}}
	if err := persistArtifactManifest(context.Background(), q, current, "runs/archive", result, true); err != nil {
		t.Fatal(err)
	}
	want := []string{"lock", "mark", "read-names", "insert", "retire-intent"}
	if strings.Join(q.calls, ",") != strings.Join(want, ",") {
		t.Fatalf("manifest operations = %v, want %v", q.calls, want)
	}
}

func TestARedeliveredManifestRecordsOnlyNamesNotYetRecordedAndKeepsThemForTheRetention(t *testing.T) {
	q := &manifestQueryRecorder{recorded: []string{"Report.txt"}}
	current := gen.Run{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}}
	result := &RunResult{Artifacts: []RunArtifact{{FileName: "report.txt"}, {FileName: "chart.png"}}}
	before := time.Now()

	if err := persistArtifactManifest(context.Background(), q, current, "runs/archive", result, false); err != nil {
		t.Fatal(err)
	}

	if len(q.inserted) != 1 || q.inserted[0].FileName != "chart.png" {
		t.Fatalf("inserted = %+v, want only chart.png; report.txt was already recorded under another case", q.inserted)
	}
	if expires := q.inserted[0].ExpiresAt; !expires.Valid || expires.Time.Before(before.Add(runArtifactRetention)) ||
		expires.Time.After(time.Now().Add(runArtifactRetention)) {
		t.Fatalf("expires at %v, want %v after it was recorded", expires.Time, runArtifactRetention)
	}
}

func TestProviderRefusesAResponsePastItsReadLimit(t *testing.T) {
	const limit = 4 << 20
	prefix := `{"ok":true}`
	body := prefix + strings.Repeat(" ", limit-len(prefix)) + "x"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	p := NewProvider("test", srv.URL, "").(*httpProvider)
	var out map[string]any
	if _, err := p.call(context.Background(), http.MethodGet, "/", nil, &out, http.StatusOK); err == nil {
		t.Fatal("provider accepted a response larger than its 4 MiB limit")
	}
}

func TestOversizedProviderErrorKeepsItsHTTPRetryClassification(t *testing.T) {
	const limit = 4 << 20
	for _, tc := range []struct {
		status    int
		retryable bool
	}{{http.StatusUnprocessableEntity, false}, {http.StatusServiceUnavailable, true}} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, strings.Repeat("x", limit+1))
			}))
			defer srv.Close()
			p := NewProvider("test", srv.URL, "").(*httpProvider)
			_, err := p.call(context.Background(), http.MethodGet, "/", nil, nil, http.StatusOK)
			if err == nil || retryable(err) != tc.retryable {
				t.Fatalf("error = %v, retryable = %v; want %v", err, retryable(err), tc.retryable)
			}
		})
	}
}
