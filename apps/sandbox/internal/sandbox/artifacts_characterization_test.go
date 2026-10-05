package sandbox

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func unpackFiltered(f filteredArchive, err error) ([]Artifact, []byte, bool, error) {
	return f.manifest, f.archive, f.truncated, err
}

func TestFilterArchiveMarksAnUnreadableArchiveAsTruncated(t *testing.T) {
	whole := tarOf(t, map[string][]byte{"artifacts/out.txt": bytes.Repeat([]byte("x"), 600)})
	for _, tc := range []struct {
		name string
		raw  []byte
	}{
		{name: "a header that is not a tar header", raw: bytes.Repeat([]byte("x"), 1024)},
		{name: "a body cut short", raw: whole[:700]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest, _, truncated, err := unpackFiltered(filterArchive(tc.raw, DefaultLimits))
			if err != nil {
				t.Fatalf("an unreadable archive was reported as an error instead of a truncated collection: %v", err)
			}
			if len(manifest) != 0 || !truncated {
				t.Fatalf("manifest = %#v, truncated %v; want empty and true", manifest, truncated)
			}
		})
	}
}

func TestCollectionCarriesTheTruncationMarkToTheRun(t *testing.T) {
	for _, tc := range []struct {
		name        string
		files       map[string][]byte
		storeStatus int
		wantFiles   int
		wantUploads int
	}{
		{
			name:        "every artifact dropped, nothing uploaded",
			files:       map[string][]byte{"artifacts/NUL": []byte("no")},
			storeStatus: http.StatusOK,
		},
		{
			name:        "one artifact dropped, the rest uploaded",
			files:       map[string][]byte{"artifacts/NUL": []byte("no"), "artifacts/keep.txt": []byte("yes")},
			storeStatus: http.StatusOK,
			wantFiles:   1,
			wantUploads: 1,
		},
		{
			name:        "one artifact dropped and the upload refused",
			files:       map[string][]byte{"artifacts/NUL": []byte("no"), "artifacts/keep.txt": []byte("yes")},
			storeStatus: http.StatusInternalServerError,
			wantUploads: 3,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			uploads := 0
			store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				uploads++
				w.WriteHeader(tc.storeStatus)
			}))
			defer store.Close()

			m := NewManager(&collectDriver{artifacts: tarOf(t, tc.files)}, Config{Provider: "test", Slots: 1}, slog.New(slog.DiscardHandler))
			e := &entry{
				limits:        DefaultLimits,
				artifactGrant: &ObjectGrant{Purpose: "artifact_upload", Access: "write", ObjectKey: "k", URL: store.URL + "/k"},
			}
			m.runs["run-1"] = e

			finished := false
			for attempt := 0; attempt < 3 && !finished; attempt++ {
				finished = m.collect(context.Background(), "run-1", "")
			}
			if !finished {
				t.Fatal("collect never finished for a workload that had announced it was done")
			}
			if !e.artifactsTruncated {
				t.Error("a dropped artifact did not mark the run's collection as truncated")
			}
			if len(e.artifacts) != tc.wantFiles {
				t.Errorf("manifest = %#v, want %d entries", e.artifacts, tc.wantFiles)
			}
			if uploads != tc.wantUploads {
				t.Errorf("object storage received %d upload(s), want %d", uploads, tc.wantUploads)
			}
		})
	}
}
