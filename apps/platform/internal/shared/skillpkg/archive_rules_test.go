package skillpkg

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"
)

func zipInOrder(t *testing.T, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range names {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(name, "/") {
			continue
		}
		if _, err := w.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func requireRefusal(t *testing.T, data []byte, want ArchiveRefusal, wantDetail string) {
	t.Helper()
	_, err := PackageFS(data)
	var refused *ArchiveError
	if !errors.As(err, &refused) {
		t.Fatalf("PackageFS returned %v, want a %s refusal", err, want)
	}
	if refused.Refusal != want || refused.Detail != wantDetail {
		t.Fatalf("refusal = %s %q, want %s %q", refused.Refusal, refused.Detail, want, wantDetail)
	}
}

func TestAnEntryNameThatIsEmptyOrNotUTF8IsRefusedBeforeAnyOtherNameRule(t *testing.T) {
	for _, tc := range []struct{ label, name string }{
		{"empty", ""},
		{"NUL byte", "a\x00b.txt"},
		{"invalid UTF-8", "bad\xff.txt"},
	} {
		t.Run(tc.label, func(t *testing.T) {
			requireRefusal(t, zipInOrder(t, "SKILL.md", tc.name),
				ArchiveUnsafeName, "archive has an empty or invalid UTF-8 entry name")
		})
	}
}

func TestANonASCIIUTF8EntryNameIsAccepted(t *testing.T) {
	if _, err := PackageFS(zipInOrder(t, "SKILL.md", "參考/指南.md")); err != nil {
		t.Fatalf("a valid UTF-8 name was refused: %v", err)
	}
}

func TestAnUnknownCompressionMethodIsRefusedAsUnsupportedNotAsCorrupt(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if _, err := zw.CreateRaw(&zip.FileHeader{Name: "odd.bin", Method: 99}); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	requireRefusal(t, buf.Bytes(), ArchiveUnsupported, `unsupported compression method 99 for "odd.bin"`)
}

func TestAFileAndADirectoryOfTheSameNameConflictInEitherOrder(t *testing.T) {
	t.Run("file after its descendant", func(t *testing.T) {
		requireRefusal(t, zipInOrder(t, "a/b", "a"),
			ArchiveUnsafeName, `archive file "a" conflicts with a descendant entry`)
	})
	t.Run("file before its descendant", func(t *testing.T) {
		requireRefusal(t, zipInOrder(t, "a", "a/b"),
			ArchiveUnsafeName, `archive file "a" is an ancestor of "a/b"`)
	})
	t.Run("directory entry after its descendant", func(t *testing.T) {
		if _, err := PackageFS(zipInOrder(t, "a/b", "a/")); err != nil {
			t.Fatalf("a directory entry for a parent that already has a child was refused: %v", err)
		}
	})
}
