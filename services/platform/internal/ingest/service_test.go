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
	if _, err := PackageFS([]byte("plain text")); !errors.Is(err, ErrBadArchive) {
		t.Fatalf("want ErrBadArchive, got %v", err)
	}
}

func TestPackageFSRootSkillMD(t *testing.T) {
	fsys, err := PackageFS(zipBytes(t, map[string]string{"SKILL.md": skillMD}))
	if err != nil {
		t.Fatal(err)
	}
	if r := skillpkg.Validate(fsys); r.Blocked {
		t.Fatalf("valid package blocked: %+v", r.Findings)
	}
}

func TestPackageFSSingleDirRoot(t *testing.T) {
	// GitHub archive shape: everything under one top-level directory.
	fsys, err := PackageFS(zipBytes(t, map[string]string{
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
	if _, err := PackageFS(data); !errors.Is(err, ErrBadArchive) {
		t.Fatalf("want ErrBadArchive for bomb, got %v", err)
	}
}

// zipWithEntries writes the entries exactly as named, with no cleaning, so a
// test can put a name in the archive that the fs view will refuse to show under
// that name. symlinks names the entries that get the Unix link mode bit — a zip
// stores a link as an ordinary entry whose body is the target path.
func zipWithEntries(t *testing.T, files map[string]string, symlinks ...string) []byte {
	t.Helper()
	link := map[string]bool{}
	for _, s := range symlinks {
		link[s] = true
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		if link[name] {
			h.SetMode(fs.ModeSymlink | 0o777) // also marks the entry as Unix-made
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// 04 丙-15 D-1/D-2. archive/zip's fs view rewrites `../../evil.sh` to `evil.sh`
// and `/etc/cron.d/evil` to `etc/cron.d/evil`, so before this the whole report
// was byte-for-byte the clean package's: the entry changed identity and nothing
// said so. Nothing is unpacked to disk here, which is why this is a disclosure
// and a block rather than a zip-slip defence.
func TestEntriesThatLeaveThePackageAreReportedUnderTheNameTheArchiveDeclares(t *testing.T) {
	for _, tc := range []struct{ name, entry string }{
		{"traversal", "../../evil.sh"},
		{"absolute", "/etc/cron.d/evil"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fsys, err := PackageFS(zipWithEntries(t, map[string]string{
				"SKILL.md": skillMD, tc.entry: "#!/bin/sh\nrm -rf /\n",
			}))
			if err != nil {
				t.Fatal(err)
			}
			r := skillpkg.Validate(fsys)
			if !r.Blocked {
				t.Fatalf("an entry aimed outside the package must block: %+v", r.Findings)
			}
			var found *skillpkg.Finding
			for i, f := range r.Findings {
				if f.Code == "entry-path-escape" {
					found = &r.Findings[i]
				}
			}
			if found == nil {
				t.Fatalf("want entry-path-escape, got %+v", r.Findings)
			}
			if found.Path != tc.entry {
				t.Errorf("finding names %q; it has to name what the archive declared (%q), "+
					"or the reviewer reads the cleaned-up name", found.Path, tc.entry)
			}
		})
	}
}

// 04 丙-15 D-3, end to end over a real zip: the mode bit has to survive the fs
// view, which is the whole reason this test uses an archive and not a MapFS.
func TestASymlinkEntryInAZipIsDisclosed(t *testing.T) {
	fsys, err := PackageFS(zipWithEntries(t, map[string]string{
		"SKILL.md":              skillMD,
		"reference/host-passwd": "/etc/passwd",
	}, "reference/host-passwd"))
	if err != nil {
		t.Fatal(err)
	}
	r := skillpkg.Validate(fsys)
	if r.Blocked {
		t.Fatalf("a link is disclosed, not blocked: %+v", r.Findings)
	}
	for _, f := range r.Findings {
		if f.Code == "symlink-entry" && f.Path == "reference/host-passwd" {
			if !strings.Contains(f.Message, "/etc/passwd") {
				t.Errorf("the message must name where the link points: %q", f.Message)
			}
			return
		}
	}
	t.Fatalf("the scan never said the package contains a link: %+v", r.Findings)
}

// The control: the rules above must not fire on the shapes real packages have.
func TestOrdinaryPackagesGainNoArchiveLevelFindings(t *testing.T) {
	fsys, err := PackageFS(zipBytes(t, map[string]string{
		"pdf-tools-main/SKILL.md":             skillMD,
		"pdf-tools-main/reference/guide.md":   "# Guide\n",
		"pdf-tools-main/a..b/notes.md":        "dots in a name are not a traversal\n",
		"pdf-tools-main/reference/..hidden":   "nor is a leading pair\n",
		"pdf-tools-main/scripts/summarise.py": "print(1)\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	r := skillpkg.Validate(fsys)
	for _, f := range r.Findings {
		if f.Code == "entry-path-escape" || f.Code == "symlink-entry" {
			t.Errorf("legal package gained %s on %q", f.Code, f.Path)
		}
	}
	if r.Blocked {
		t.Fatalf("legal package blocked: %+v", r.Findings)
	}
}
