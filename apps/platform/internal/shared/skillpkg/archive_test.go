package skillpkg

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"io/fs"
	"strings"
	"testing"
)

// Moved here with PackageFS from internal/ingest on 2026-08-20 (DDD-006).

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

const archiveSkillMD = "---\nname: pdf-tools\ndescription: Work with PDFs.\nlicense: MIT\n---\n# PDF\n"

func TestPackageFSNotAZip(t *testing.T) {
	if _, err := PackageFS([]byte("plain text")); !errors.Is(err, ErrBadArchive) {
		t.Fatalf("want ErrBadArchive, got %v", err)
	}
}

func TestPackageFSRejectsCorruptEntryBytes(t *testing.T) {
	const payload = "unique-corruption-target"
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range map[string]string{"SKILL.md": archiveSkillMD, "broken.txt": payload} {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(content))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	pos := bytes.Index(data, []byte(payload))
	if pos < 0 {
		t.Fatal("fixture payload not stored verbatim")
	}
	data[pos] ^= 0xff
	if _, err := PackageFS(data); !errors.Is(err, ErrBadArchive) {
		t.Fatalf("corrupt entry passed archive admission: %v", err)
	}
}

func TestPackageFSRejectsZip64ExtraEvenForASmallEntry(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	h := &zip.FileHeader{Name: "SKILL.md", Method: zip.Store, Extra: []byte{0x01, 0x00, 0x00, 0x00}}
	w, err := zw.CreateHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte(archiveSkillMD))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := PackageFS(buf.Bytes()); !errors.Is(err, ErrBadArchive) {
		t.Fatalf("small zip64 entry passed admission: %v", err)
	}
}

func TestPackageFSRootSkillMD(t *testing.T) {
	fsys, err := PackageFS(zipBytes(t, map[string]string{"SKILL.md": archiveSkillMD}))
	if err != nil {
		t.Fatal(err)
	}
	if r := Validate(fsys); r.Blocked {
		t.Fatalf("valid package blocked: %+v", r.Findings)
	}
}

func TestPackageFSSingleDirRoot(t *testing.T) {
	// GitHub archive shape: everything under one top-level directory.
	fsys, err := PackageFS(zipBytes(t, map[string]string{
		"pdf-tools-main/SKILL.md":  archiveSkillMD,
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

	data := zipBytes(t, map[string]string{"SKILL.md": archiveSkillMD, "big.txt": strings.Repeat("x", 4096)})
	if _, err := PackageFS(data); !errors.Is(err, ErrBadArchive) {
		t.Fatalf("want ErrBadArchive for bomb, got %v", err)
	}
}

func TestPackageFSRejectsDuplicateEntryNames(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, content := range []string{archiveSkillMD, "malicious replacement"} {
		w, err := zw.Create("SKILL.md")
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

	if _, err := PackageFS(buf.Bytes()); !errors.Is(err, ErrBadArchive) {
		t.Fatalf("duplicate SKILL.md: err = %v, want ErrBadArchive", err)
	}
}

func TestPackageFSRejectsCanonicallyDuplicateEntryNames(t *testing.T) {
	for _, files := range []map[string]string{
		{"SKILL.md": archiveSkillMD, "./SKILL.md": "replacement"},
		{"SKILL.md": archiveSkillMD, "dir/x": "one", `dir\x`: "two"},
		{"SKILL.md": archiveSkillMD, "skill.md": "replacement"},
		{"SKILL.md": archiveSkillMD, "dir/x": "one", "dir/x. ": "two"},
	} {
		if _, err := PackageFS(zipBytes(t, files)); !errors.Is(err, ErrBadArchive) {
			t.Fatalf("canonical duplicate: err = %v, want ErrBadArchive", err)
		}
	}
}

func TestPackageFSRejectsEntriesTheRuntimeCannotExtract(t *testing.T) {
	for _, tc := range []struct {
		name   string
		header zip.FileHeader
	}{
		{name: "encrypted", header: zip.FileHeader{Name: "secret.bin", Method: zip.Store, Flags: 1}},
		{name: "unsupported compression", header: zip.FileHeader{Name: "odd.bin", Method: 99}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			zw := zip.NewWriter(&buf)
			w, err := zw.Create("SKILL.md")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := w.Write([]byte(archiveSkillMD)); err != nil {
				t.Fatal(err)
			}
			if _, err := zw.CreateRaw(&tc.header); err != nil {
				t.Fatal(err)
			}
			if err := zw.Close(); err != nil {
				t.Fatal(err)
			}

			if _, err := PackageFS(buf.Bytes()); !errors.Is(err, ErrBadArchive) {
				t.Fatalf("err = %v, want ErrBadArchive", err)
			}
		})
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
		{"backslash", `scripts\run.sh`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fsys, err := PackageFS(zipWithEntries(t, map[string]string{
				"SKILL.md": archiveSkillMD, tc.entry: "#!/bin/sh\nrm -rf /\n",
			}))
			if err != nil {
				t.Fatal(err)
			}
			r := Validate(fsys)
			if !r.Blocked {
				t.Fatalf("an entry aimed outside the package must block: %+v", r.Findings)
			}
			var found *Finding
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
func TestASymlinkEntryInAZipIsBlockedBeforeRuntime(t *testing.T) {
	fsys, err := PackageFS(zipWithEntries(t, map[string]string{
		"SKILL.md":              archiveSkillMD,
		"reference/host-passwd": "/etc/passwd",
	}, "reference/host-passwd"))
	if err != nil {
		t.Fatal(err)
	}
	r := Validate(fsys)
	if !r.Blocked {
		t.Fatalf("a link accepted here cannot be extracted by the runtime: %+v", r.Findings)
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
		"pdf-tools-main/SKILL.md":             archiveSkillMD,
		"pdf-tools-main/reference/guide.md":   "# Guide\n",
		"pdf-tools-main/a..b/notes.md":        "dots in a name are not a traversal\n",
		"pdf-tools-main/reference/..hidden":   "nor is a leading pair\n",
		"pdf-tools-main/scripts/summarise.py": "print(1)\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	r := Validate(fsys)
	for _, f := range r.Findings {
		if f.Code == "entry-path-escape" || f.Code == "symlink-entry" {
			t.Errorf("legal package gained %s on %q", f.Code, f.Path)
		}
	}
	if r.Blocked {
		t.Fatalf("legal package blocked: %+v", r.Findings)
	}
}

func TestArchiveEnvelopeRejectsZip64AndExecutablePrefixes(t *testing.T) {
	ordinary := zipBytes(t, map[string]string{"SKILL.md": archiveSkillMD})
	locator := make([]byte, 20)
	binary.LittleEndian.PutUint32(locator, 0x07064b50)
	zip64 := append(append(append([]byte{}, ordinary[:len(ordinary)-22]...), locator...), ordinary[len(ordinary)-22:]...)
	if _, err := PackageFS(zip64); !errors.Is(err, ErrBadArchive) {
		t.Fatalf("archive-level zip64 locator: err=%v, want ErrBadArchive", err)
	}
	prefixed := append([]byte("MZ executable stub"), ordinary...)
	if _, err := PackageFS(prefixed); !errors.Is(err, ErrBadArchive) {
		t.Fatalf("self-extracting prefix: err=%v, want ErrBadArchive", err)
	}
	prefix := []byte("MZ adjusted stub")
	adjusted := append(append([]byte{}, prefix...), ordinary...)
	oldEOCD := len(ordinary) - 22
	oldCentral := binary.LittleEndian.Uint32(ordinary[oldEOCD+16 : oldEOCD+20])
	newCentral := len(prefix) + int(oldCentral)
	oldLocal := binary.LittleEndian.Uint32(adjusted[newCentral+42 : newCentral+46])
	binary.LittleEndian.PutUint32(adjusted[newCentral+42:newCentral+46], oldLocal+uint32(len(prefix)))
	newEOCD := len(prefix) + oldEOCD
	binary.LittleEndian.PutUint32(adjusted[newEOCD+16:newEOCD+20], oldCentral+uint32(len(prefix)))
	if _, err := PackageFS(adjusted); !errors.Is(err, ErrBadArchive) {
		t.Fatalf("self-extracting prefix with adjusted offsets: err=%v, want ErrBadArchive", err)
	}
}

func TestArchiveRejectsATruncatedExtraField(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	h := &zip.FileHeader{Name: "SKILL.md", Method: zip.Store, Extra: []byte{0x34, 0x12, 0x05, 0x00}}
	w, err := zw.CreateHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(archiveSkillMD)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := PackageFS(buf.Bytes()); !errors.Is(err, ErrBadArchive) {
		t.Fatalf("truncated extra field: err=%v, want ErrBadArchive", err)
	}
}

func TestArchiveEntryTypeMustAgreeWithItsRawName(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode fs.FileMode
	}{
		{"link/", fs.ModeSymlink | 0o777},
		{"directory-without-slash", fs.ModeDir | 0o755},
	} {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		h := &zip.FileHeader{Name: tc.name, Method: zip.Store}
		h.SetMode(tc.mode)
		if _, err := zw.CreateHeader(h); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := PackageFS(buf.Bytes()); !errors.Is(err, ErrBadArchive) {
			t.Errorf("%q mode %v: err=%v, want ErrBadArchive", tc.name, tc.mode, err)
		}
	}
}

func TestArchiveRejectsWindowsNonportableNames(t *testing.T) {
	for _, name := range []string{"NUL", "con.txt", "dir/COM1.log", "file:stream", "bad?.txt", "control\x01.txt"} {
		t.Run(name, func(t *testing.T) {
			if _, err := PackageFS(zipBytes(t, map[string]string{name: "x"})); !errors.Is(err, ErrBadArchive) {
				t.Fatalf("name %q: err=%v, want ErrBadArchive", name, err)
			}
		})
	}
}

func TestArchiveRejectsFileAncestorConflictsAndOversizedComponents(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"ancestor first":      {"a": "file", "a/b": "child"},
		"descendant first":    {"a/b": "child", "a": "file"},
		"oversized component": {strings.Repeat("x", 256): "long"},
		"case fold shrinks":   {strings.Repeat("K", 100) + ".txt": "long"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := PackageFS(zipBytes(t, files)); !errors.Is(err, ErrBadArchive) {
				t.Fatalf("files %#v: err=%v, want ErrBadArchive", files, err)
			}
		})
	}
}

// PDM-005 §5.1b's other three ceilings, none of which had a value before
// 2026-08-25. Each is checked in the direction that matters: the ceiling refuses,
// and one entry below it does not — a test that only proved the refusal would
// stay green if the bound were set to zero.
//
// The ceilings are lowered for the test rather than built up to. A fixture that
// has to declare 10 MB to reach maxEntryBytes is a fixture that makes the suite
// slow enough to stop being run, which is its own kind of missing test.
func TestTheArchiveCeilingsPDM005NamesAreEnforced(t *testing.T) {
	t.Run("entry count", func(t *testing.T) {
		defer swapInt(&maxArchiveEntries, 3)()
		files := map[string]string{"SKILL.md": archiveSkillMD, "a": "1", "b": "2", "c": "3"}
		if _, err := PackageFS(zipWithEntries(t, files)); !errors.Is(err, ErrBadArchive) {
			t.Errorf("4 entries under a ceiling of 3: err = %v, want ErrBadArchive", err)
		}
		delete(files, "c")
		if _, err := PackageFS(zipWithEntries(t, files)); err != nil {
			t.Errorf("3 entries under a ceiling of 3 is at the bound, not over it: %v", err)
		}
	})

	t.Run("one entry's size", func(t *testing.T) {
		// Above archiveSkillMD's own size and below the oversized entry, so the
		// fixture's own SKILL.md is not what trips the ceiling.
		defer swapUint64(&maxEntryBytes, 100)()
		// Well under the total cap, so only the per-entry ceiling can refuse it.
		big := strings.Repeat("x", 200)
		if _, err := PackageFS(zipWithEntries(t, map[string]string{
			"SKILL.md": archiveSkillMD, "big.txt": big,
		})); !errors.Is(err, ErrBadArchive) {
			t.Errorf("a 200 byte entry under a 100 byte ceiling: err = %v, want ErrBadArchive", err)
		}
		if _, err := PackageFS(zipWithEntries(t, map[string]string{
			"SKILL.md": archiveSkillMD, "small.txt": "x",
		})); err != nil {
			t.Errorf("a one byte entry was refused: %v", err)
		}
	})

	t.Run("directory depth", func(t *testing.T) {
		defer swapInt(&maxEntryDepth, 2)()
		if _, err := PackageFS(zipWithEntries(t, map[string]string{
			"SKILL.md": archiveSkillMD, "a/b/c/deep.txt": "x",
		})); !errors.Is(err, ErrBadArchive) {
			t.Errorf("three directories under a depth of 2: err = %v, want ErrBadArchive", err)
		}
		if _, err := PackageFS(zipWithEntries(t, map[string]string{
			"SKILL.md": archiveSkillMD, "a/b/ok.txt": "x",
		})); err != nil {
			t.Errorf("two directories is at the bound, not over it: %v", err)
		}
	})
}

func swapInt(p *int, v int) func() {
	old := *p
	*p = v
	return func() { *p = old }
}

func swapUint64(p *uint64, v uint64) func() {
	old := *p
	*p = v
	return func() { *p = old }
}

// The two ceilings PDM-005 §5.1b states by number, asserted as numbers.
//
// Every other test in this file reaches these through the symbol, so they stay
// green at any value — which is exactly how the constant sat at 32 MiB for
// months while §5.1b, `02:SEC-003` and PDM-005 all said 10 MB, and no test could
// tell. The product owner ratified §5.1b's values on 2026-08-27 (`05` R-13), and
// this is the assertion that makes drifting off them fail rather than pass.
//
// It is deliberately a literal comparison and not a reference to some shared
// constant: a guard that reads the same variable it guards guards nothing. If
// PDM-005 is ever amended, the amendment changes this line, and the diff shows
// somebody decided rather than somebody adjusted.
func TestTheImportCeilingsAreTheRatifiedOnes(t *testing.T) {
	// §5.1b: 壓縮檔 10 MB. The codebase reads MB as MiB throughout (HumanMB
	// divides by 1<<20), so this is the same convention the refusal message uses.
	if MaxZipBytes != 10<<20 {
		t.Errorf("MaxZipBytes = %d (%s), want 10 MB: PDM-005 §5.1b and 02:SEC-003 both state it, "+
			"and 02:SEC-003 is an acceptance criterion — a different value makes 03:INGEST-014 "+
			"unticklable no matter how well the fetcher is written", MaxZipBytes, HumanMB(MaxZipBytes))
	}
	// §5.1b: 解壓後 100 MB.
	if maxUnpackedBytes != uint64(100<<20) {
		t.Errorf("maxUnpackedBytes = %d, want 100 MB (PDM-005 §5.1b)", maxUnpackedBytes)
	}
	// The two import paths share one set of limits, and 02:SEC-003 asks for that
	// in so many words. Sharing the SYMBOL is what makes it true; this test is
	// here because sharing the symbol was already true while the value was wrong.
	if HumanMB(MaxZipBytes) != "10.0 MB" {
		t.Errorf("a refusal would print %q; the number a creator reads has to be §5.1b's",
			HumanMB(MaxZipBytes))
	}
}
