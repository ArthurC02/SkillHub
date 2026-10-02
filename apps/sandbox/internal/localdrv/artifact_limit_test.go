package localdrv

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtifactsPastTheReadLimitKeepTheWholeFilesBeforeTheCut(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(strings.Repeat("x", 600)), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	raw, err := archiveWithin(dir, 1600)

	if err != nil {
		t.Fatalf("archive past the limit = %v, want the cut stream so the caller can keep what fits", err)
	}
	reader := tar.NewReader(bytes.NewReader(raw))
	first, err := reader.Next()
	if err != nil || first.Name != "a.txt" {
		t.Fatalf("first entry = %+v, %v; want a.txt whole", first, err)
	}
	if body, err := io.ReadAll(reader); err != nil || len(body) != 600 {
		t.Fatalf("a.txt read %d bytes, %v; want all 600", len(body), err)
	}
	if _, err := reader.Next(); err == nil || errors.Is(err, io.EOF) {
		t.Errorf("after a.txt the stream ended cleanly (%v); want it visibly cut so the run is marked truncated", err)
	}
}
