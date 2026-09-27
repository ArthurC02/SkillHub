package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAFailingPackageReportsItsExitCodeWithoutAnError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":        "module failing\n\ngo 1.21\n",
		"fails_test.go": "package failing\n\nimport \"testing\"\n\nfunc TestFails(t *testing.T) { t.Fatal(\"red on purpose\") }\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out strings.Builder
	code, err := testReport(dir, dir, nil, &out)
	if err != nil {
		t.Fatalf("a failing test run is a verdict, not an error: %v", err)
	}
	if code != 1 {
		t.Fatalf("exit code = %d, want go test's own 1", code)
	}
	if !strings.Contains(out.String(), "0 passed, 0 skipped, 1 failed") {
		t.Fatalf("report does not count the failure:\n%s", out.String())
	}
}

func TestASkipWithNoMessageIsCountedUnderNoReasonGiven(t *testing.T) {
	t.Parallel()
	events := `{"Action":"skip","Package":"p","Test":"TestQuiet"}`
	s, err := summarize(strings.NewReader(events), io_Discard{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Skipped != 1 || s.Reasons["(no reason given)"] != 1 {
		t.Fatalf("got %+v, want one skip under \"(no reason given)\"", s)
	}
}
