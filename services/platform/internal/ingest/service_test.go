package ingest

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/services/platform/internal/skillpkg"
)

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

func TestPackageFSNotAZip(t *testing.T) {
	if _, err := packageFS([]byte("plain text")); !errors.Is(err, ErrBadArchive) {
		t.Fatalf("want ErrBadArchive, got %v", err)
	}
}

func TestPackageFSRootSkillMD(t *testing.T) {
	fsys, err := packageFS(zipBytes(t, map[string]string{"SKILL.md": skillMD}))
	if err != nil {
		t.Fatal(err)
	}
	if r := skillpkg.Validate(fsys); r.Blocked {
		t.Fatalf("valid package blocked: %+v", r.Findings)
	}
}

func TestPackageFSSingleDirRoot(t *testing.T) {
	// GitHub archive shape: everything under one top-level directory.
	fsys, err := packageFS(zipBytes(t, map[string]string{
		"pdf-tools-main/SKILL.md":  skillMD,
		"pdf-tools-main/notes.txt": "hi",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(fsys, "SKILL.md"); err != nil {
		t.Fatalf("SKILL.md not found through single-dir root: %v", err)
	}
}

func TestPackageFSZipBomb(t *testing.T) {
	old := maxUnpackedBytes
	maxUnpackedBytes = 1024
	defer func() { maxUnpackedBytes = old }()

	data := zipBytes(t, map[string]string{"SKILL.md": skillMD, "big.txt": strings.Repeat("x", 4096)})
	if _, err := packageFS(data); !errors.Is(err, ErrBadArchive) {
		t.Fatalf("want ErrBadArchive for bomb, got %v", err)
	}
}
