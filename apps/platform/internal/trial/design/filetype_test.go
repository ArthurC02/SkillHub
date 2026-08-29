package testlab

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
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
	// user files: a real .xlsx routinely carries more parts than that. The FILE
	// COUNT is the only budget that entry turns off - see the byte-budget test
	// below, which is the half that used to be turned off with it.
	entries["[Content_Types].xml"] = "<Types/>"
	if _, err := detectContentType(zipOf(t, entries)); err != nil {
		t.Fatalf("rejected an OOXML document for its internal part count: %v", err)
	}
}

// zipDeclaring builds a zip whose central directory declares the given unpacked
// sizes without carrying the bytes. That is the shape inspectZip actually reads
// (it never unpacks - the sandbox does, ADR-005), and it is also the shape of the
// attack: deflate lets a 25 MB upload declare gigabytes.
func zipDeclaring(t *testing.T, entries map[string]uint64) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, size := range entries {
		w, err := zw.CreateRaw(&zip.FileHeader{
			Name:               name,
			Method:             zip.Deflate,
			CompressedSize64:   2,
			UncompressedSize64: size,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte{0x03, 0x00}); err != nil { // an empty deflate stream
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestInspectZipKeepsTheUnpackBudgetForOOXML: [Content_Types].xml switches off
// the file-count limit and nothing else.
//
// It used to switch off the byte budget as well, which put the whole PDM-005 §5.1
// unpack allowance for a zip dataset behind one filename - and the only remaining
// upper bound was the 25 MB compressed-side limit, which deflate turns into
// several GB of content the sandbox would be asked to mount.
func TestInspectZipKeepsTheUnpackBudgetForOOXML(t *testing.T) {
	bomb := zipDeclaring(t, map[string]uint64{
		"[Content_Types].xml":      32,
		"xl/worksheets/sheet1.xml": uint64(MaxTestCaseBytes) + 1,
	})
	if _, err := detectContentType(bomb); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("accepted an OOXML-named archive declaring more than %s unpacked: err = %v",
			humanMB(MaxTestCaseBytes), err)
	}

	// And a real spreadsheet's shape still passes: many parts, small total.
	ok := zipDeclaring(t, map[string]uint64{
		"[Content_Types].xml":      32,
		"xl/worksheets/sheet1.xml": 4096,
	})
	if _, err := detectContentType(ok); err != nil {
		t.Fatalf("rejected an ordinary OOXML document: %v", err)
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

// The length cap is by bytes, and every case above is ASCII — so deleting the
// cap entirely left this test green (M2 audit, 2026-08-24), and so did the
// original `name = name[:MaxNameBytes]`, which halves a rune on any name that
// is not. The name reaches a text column, the permission summary and a snapshot
// manifest; the first of those refuses an invalid byte sequence outright.
func TestSanitizeFileNameCutsToBytesWithoutHalvingARune(t *testing.T) {
	long := strings.Repeat("\u9577", MaxNameBytes) + ".csv"

	got := sanitizeFileName(long)
	if len(got) > MaxNameBytes {
		t.Fatalf("name is %d bytes, over the %d cap", len(got), MaxNameBytes)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("sanitizeFileName produced invalid UTF-8: %q", got)
	}
	if got == "" {
		t.Fatal("a long name became no name at all")
	}
}
