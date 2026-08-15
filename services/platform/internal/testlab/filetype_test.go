package testlab

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"
)

// zipOf builds an in-memory archive with the given entries.
func zipOf(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestDetectContentTypeAllows covers the PDM-005 §5.1 allow-list. The file name
// is not an argument to detectContentType at all, which is the point: the type
// comes from the bytes.
func TestDetectContentTypeAllows(t *testing.T) {
	png := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, bytes.Repeat([]byte{0}, 64)...)
	cases := map[string][]byte{
		"csv":   []byte("id,name\n1,alice\n"),
		"json":  []byte(`{"rows":[{"id":1}]}`),
		"yaml":  []byte("rows:\n  - id: 1\n"),
		"xml":   []byte(`<?xml version="1.0"?><rows><row id="1"/></rows>`),
		"pdf":   []byte("%PDF-1.7\n1 0 obj\n"),
		"png":   png,
		"zip":   zipOf(t, map[string]string{"notes.txt": "hello"}),
		"ooxml": zipOf(t, map[string]string{"[Content_Types].xml": "<Types/>", "word/document.xml": "<w/>"}),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := detectContentType(data)
			if err != nil {
				t.Fatalf("rejected valid %s content: %v", name, err)
			}
			if got == "" {
				t.Fatal("accepted content with no recorded type")
			}
		})
	}
}

// TestDetectContentTypeRejectsDisguisedExecutables is the "不信任副檔名" rule: an
// executable stays an executable however it is named, and detectContentType is
// never told the name in the first place.
func TestDetectContentTypeRejectsDisguisedExecutables(t *testing.T) {
	pad := bytes.Repeat([]byte{0}, 128)
	cases := map[string][]byte{
		"elf":     append([]byte{0x7f, 'E', 'L', 'F', 2, 1, 1}, pad...),
		"pe":      append([]byte{'M', 'Z', 0x90}, pad...),
		"macho":   append([]byte{0xcf, 0xfa, 0xed, 0xfe}, pad...),
		"fatmach": append([]byte{0xca, 0xfe, 0xba, 0xbe}, pad...),
		"shebang": []byte("#!/bin/sh\nrm -rf /\n"),
		"gzip":    append([]byte{0x1f, 0x8b, 0x08}, pad...),
		"xz":      append([]byte{0xfd, '7', 'z', 'X', 'Z', 0}, pad...),
		"rar":     append([]byte{'R', 'a', 'r', '!', 0x1a}, pad...),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := detectContentType(data); !errors.Is(err, ErrUnsupportedType) {
				t.Fatalf("accepted %s content: err = %v", name, err)
			}
		})
	}
}

// TestDetectContentTypeRejectsUnknownBinary: anything that is neither text nor
// an allowed container is refused, so the allow-list is closed rather than a
// list of known-bad prefixes.
func TestDetectContentTypeRejectsUnknownBinary(t *testing.T) {
	data := append([]byte{'O', 'g', 'g', 'S', 0x00}, bytes.Repeat([]byte{0x01, 0x00}, 128)...)
	if _, err := detectContentType(data); !errors.Is(err, ErrUnsupportedType) {
		t.Fatalf("accepted unknown binary: err = %v", err)
	}
}

func TestInspectZipRejectsUnsafeEntries(t *testing.T) {
	cases := map[string][]byte{
		"traversal":        zipOf(t, map[string]string{"../escape.txt": "x"}),
		"nested traversal": zipOf(t, map[string]string{"data/../../escape.txt": "x"}),
		"absolute path":    zipOf(t, map[string]string{"/etc/passwd": "x"}),
		"nested archive":   zipOf(t, map[string]string{"inner.zip": "PK\x03\x04"}),
		"nested tarball":   zipOf(t, map[string]string{"inner.tar.gz": "x"}),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := detectContentType(data); !errors.Is(err, ErrUnsupportedType) {
				t.Fatalf("accepted archive with %s: err = %v", name, err)
			}
		})
	}
}

// TestInspectZipAppliesUnpackBudget: a plain archive's contents count against
// the per-test-case file budget, since that is what the sandbox will mount.
func TestInspectZipAppliesUnpackBudget(t *testing.T) {
	entries := map[string]string{}
	for i := range MaxFilesPerTestCase + 1 {
		entries[string(rune('a'+i%26))+strings.Repeat("x", i)+".txt"] = "row"
	}
	if _, err := detectContentType(zipOf(t, entries)); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("accepted archive of %d files: err = %v", len(entries), err)
	}

	// The same entry count inside an OOXML container is one document, not 21
	// user files: a real .xlsx routinely carries more parts than that.
	entries["[Content_Types].xml"] = "<Types/>"
	if _, err := detectContentType(zipOf(t, entries)); err != nil {
		t.Fatalf("rejected an OOXML document for its internal part count: %v", err)
	}
}

// TestSanitizeFileName: the display name can never be read as a path.
func TestSanitizeFileName(t *testing.T) {
	cases := map[string]string{
		"data.csv":               "data.csv",
		"  spaced.csv  ":         "spaced.csv",
		"../../etc/passwd":       "passwd",
		`C:\Users\a\secrets.txt`: "secrets.txt",
		"/absolute/rows.json":    "rows.json",
		"..":                     "",
		"bad\x00name.csv":        "badname.csv",
	}
	for in, want := range cases {
		if got := sanitizeFileName(in); got != want {
			t.Errorf("sanitizeFileName(%q) = %q, want %q", in, got, want)
		}
	}
}
