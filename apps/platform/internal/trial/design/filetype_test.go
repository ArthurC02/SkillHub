package testlab

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

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

func TestInspectZipAppliesUnpackBudget(t *testing.T) {
	entries := map[string]string{}
	for i := range MaxFilesPerTestCase + 1 {
		entries[string(rune('a'+i%26))+strings.Repeat("x", i)+".txt"] = "row"
	}
	if _, err := detectContentType(zipOf(t, entries)); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("accepted archive of %d files: err = %v", len(entries), err)
	}

	entries["[Content_Types].xml"] = "<Types/>"
	if _, err := detectContentType(zipOf(t, entries)); err != nil {
		t.Fatalf("rejected an OOXML document for its internal part count: %v", err)
	}
}

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
		if _, err := w.Write([]byte{0x03, 0x00}); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestInspectZipKeepsTheUnpackBudgetForOOXML(t *testing.T) {
	bomb := zipDeclaring(t, map[string]uint64{
		"[Content_Types].xml":      32,
		"xl/worksheets/sheet1.xml": uint64(MaxTestCaseBytes) + 1,
	})
	if _, err := detectContentType(bomb); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("accepted an OOXML-named archive declaring more than %s unpacked: err = %v",
			humanMB(MaxTestCaseBytes), err)
	}

	ok := zipDeclaring(t, map[string]uint64{
		"[Content_Types].xml":      32,
		"xl/worksheets/sheet1.xml": 4096,
	})
	if _, err := detectContentType(ok); err != nil {
		t.Fatalf("rejected an ordinary OOXML document: %v", err)
	}
}

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
