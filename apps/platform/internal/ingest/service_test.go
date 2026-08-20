package ingest

import (
	"archive/zip"
	"bytes"
	"testing"
)

// The PackageFS/PackageRoot tests that used to live here moved to
// internal/skillpkg/archive_test.go with the functions themselves (DDD-006,
// 2026-08-20). What is left is the zip fixture the enrichment tests build on.

func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const skillMD = "---\nname: pdf-tools\ndescription: Work with PDFs.\nlicense: MIT\n---\n# PDF\n"
