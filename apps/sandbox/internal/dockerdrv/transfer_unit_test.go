package dockerdrv

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

func TestFetchRejectsAnOversizedGrantedObject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "67108865")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := fetch(context.Background(), srv.URL+"/signed?secret=must-not-leak")
	if err == nil {
		t.Fatal("oversized object was accepted")
	}
	if strings.Contains(err.Error(), srv.URL) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("grant URL leaked into error: %v", err)
	}
}

func TestFetchRejectsOversizedChunkedObject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher := w.(http.Flusher)
		_, _ = w.Write([]byte("first"))
		flusher.Flush()
		_, _ = w.Write([]byte("-chunk-exceeds-limit"))
	}))
	defer srv.Close()

	_, err := fetchWithLimit(context.Background(), srv.URL+"/signed?secret=must-not-leak", 5)
	if err == nil {
		t.Fatal("oversized chunked object was accepted")
	}
	if strings.Contains(err.Error(), srv.URL) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("grant URL leaked into error: %v", err)
	}
}

func TestFetchRefusesToFollowARedirect(t *testing.T) {
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("this must never be fetched"))
	}))
	defer elsewhere.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL, http.StatusFound)
	}))
	defer srv.Close()

	body, err := fetch(context.Background(), srv.URL+"/signed?secret=must-not-leak")
	if err == nil {
		t.Fatalf("a redirected grant URL was followed and returned %q", body)
	}
	if !strings.Contains(err.Error(), "302") {
		t.Errorf("the refusal should report the redirect status it saw; got %v", err)
	}
	if strings.Contains(err.Error(), srv.URL) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("grant URL leaked into error: %v", err)
	}
}

func TestDatasetNamesSkipsEmptyKeysAndUnsafeNames(t *testing.T) {
	req := sandbox.RunRequest{TestCase: sandbox.TestCaseSnapshotRef{DatasetRefs: []sandbox.DatasetRef{
		{ObjectKey: "k1", FileName: "input.csv"},
		{ObjectKey: "", FileName: "no-key.csv"},
		{ObjectKey: "k2", FileName: ".."},
		{ObjectKey: "k3", FileName: "-rf"},
	}}}

	got := datasetNames(req)
	want := map[string]string{"k1": "input.csv"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("datasetNames = %v, want %v", got, want)
	}
}

func TestTheSkillsOwnDirectoryInsideItsPackageReachesTheWorkload(t *testing.T) {
	const declared = "skills/tidy-notes"

	lines := env(sandbox.RunRequest{SkillVersion: sandbox.PackageRef{SourcePath: declared}})
	want := "SKILLHUB_SKILL_SOURCE_PATH=" + declared
	if !slices.Contains(lines, want) {
		t.Fatalf("the workload is never told which directory of the package is the skill;\n"+
			"a plugin's archive root holds no SKILL.md, so the run would install the whole plugin and activate nothing.\ngot %v", lines)
	}

	if bare := env(sandbox.RunRequest{}); !slices.Contains(bare, "SKILLHUB_SKILL_SOURCE_PATH=") {
		t.Errorf("a skill that is its whole package must still get the variable, empty, "+
			"so the workload reads one rule rather than two: got %v", bare)
	}
}
