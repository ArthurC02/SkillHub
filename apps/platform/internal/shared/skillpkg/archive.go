package skillpkg

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

// Zip archive reading, moved here from internal/ingest on 2026-08-20 (DDD-006).
//
// PackageFS, PackageRoot and MaxZipBytes are stateless pure functions over
// bytes — no pool, no store, no policy, no context — and catalog, run,
// packaging and eval all need the same answer import got. That makes them
// Shared Kernel material (ADR-032 §1), not a reason for four bounded contexts
// to import the Trust & Supply Chain context. What stays in ingest is the part
// that is not stateless: SaveVersion *is* the import validation pipeline
// (prepare → scan → enrich → tx), and M4's PACK-002 ruling requires 採納建議 to
// reuse it rather than grow a second version-creation path (a second truth).

// MaxZipBytes caps the uploaded archive.
//
// PDM-005 §5.1b says 10 MB and this says 32 MiB, and the difference is left
// standing on purpose: lowering it rejects packages that import successfully
// today, which is a product call and not a defect to be quietly fixed. The
// same applies to maxUnpackedBytes below (§5.1b says 100 MB). Both are recorded
// in 04 rather than changed here.
//
// What was a defect is that the other five ceilings §5.1b names had no value at
// all — see the block below. `03` §1 said PDM-005's numbers were 值已全數強制;
// for §5.1b that was true of two of seven.
const MaxZipBytes = 32 << 20 // 32 MiB

// maxUnpackedBytes caps total declared uncompressed size (zip bombs,
// ADR-007 壓縮炸彈). Var only so tests can lower it.
var maxUnpackedBytes = uint64(256 << 20) // 256 MiB

// The rest of PDM-005 §5.1b. Each of these was unbounded until 2026-08-25, and
// unbounded is the wrong default for every one of them: this archive is opened
// in the control plane (TM-IMP-02), not in a sandbox, so the blast radius is the
// import node itself.
//
// The values are §5.1b's, because it is the only place they are stated. Var, not
// const, for the same reason maxUnpackedBytes is: a test that has to build a
// 10 MB fixture to reach a ceiling is a test nobody runs.
var (
	// 檔案總數 ≤ 2,000 — large enough for a Skill that ships assets, small
	// enough that a million-tiny-file archive cannot exhaust inodes.
	maxArchiveEntries = 2000
	// 單一檔案大小 ≤ 10 MB. Declared size, checked before anything is read:
	// the total cap alone lets one entry claim all of it.
	maxEntryBytes = uint64(10 << 20)
	// 目錄巢狀深度 ≤ 10.
	maxEntryDepth = 10
)

// §5.1b's two remaining clauses are deliberately not here.
//
// **Symlinks are disclosed, not refused.** §5.1b says refuse, and its reason is
// TOCTOU: "解析後檢查有 TOCTOU 空間，直接拒絕沒有". That premise does not hold
// for this implementation — nothing here ever resolves a link. The archive is
// read as a read-only fs.FS and a link entry's content is the target path as a
// string, so there is no window between checking and following because there is
// no following. 04 丙-15 D-3 decided disclosure, with a test that requires the
// finding to name where the link points, and that decision is better informed
// about this code than the blanket rule is.
//
// **Nested archives are not refused either**, and that one is a real gap rather
// than a resolved disagreement: §5.1b forbids an archive inside an archive as a
// standard way around the unpacking cap, and refusing by file extension would
// also reject a Skill that legitimately ships a zip as sample data. Whether that
// trade is worth making is a product call, recorded rather than taken.

// ErrBadArchive marks input that is not an acceptable zip at all (as opposed
// to a well-formed package that fails validation).
var ErrBadArchive = errors.New("bad archive")

// PackageFS opens the zip as a read-only fs.FS, rejecting bombs and locating
// the package root: SKILL.md at top level, or inside a single top-level
// directory (the shape GitHub archive downloads have).
//
// Exported because the read side has to resolve the package root exactly the
// way import did: catalog's detail and file views re-open the stored archive,
// and a second root-finding rule would show a different package than the one
// that was validated. Opening is still pure analysis — nothing inside is ever
// executed (iron rule 1).
func PackageFS(data []byte) (fs.FS, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: not a zip archive", ErrBadArchive)
	}
	if len(zr.File) > maxArchiveEntries {
		return nil, fmt.Errorf("%w: archive holds %d entries, more than the %d allowed",
			ErrBadArchive, len(zr.File), maxArchiveEntries)
	}
	var unpacked uint64
	var findings []Finding
	for _, f := range zr.File {
		if f.UncompressedSize64 > maxEntryBytes {
			return nil, fmt.Errorf("%w: %s declares %d bytes, more than the %d allowed for one file",
				ErrBadArchive, f.Name, f.UncompressedSize64, maxEntryBytes)
		}
		if depth := strings.Count(strings.Trim(f.Name, "/"), "/"); depth > maxEntryDepth {
			return nil, fmt.Errorf("%w: %s nests %d directories deep, more than the %d allowed",
				ErrBadArchive, f.Name, depth, maxEntryDepth)
		}
		unpacked += f.UncompressedSize64
		if unpacked > maxUnpackedBytes {
			return nil, fmt.Errorf("%w: uncompressed content exceeds %d bytes", ErrBadArchive, maxUnpackedBytes)
		}
		// The raw name, read here because this is the last place it exists: the
		// fs view below rewrites `../../evil.sh` to `evil.sh` (04 丙-15 D-1/D-2).
		if finding, escapes := ArchiveEntryFinding(f.Name); escapes {
			findings = append(findings, finding)
		}
	}
	var tree fs.FS = zr // no root to strip: let Validate report skill-md-missing
	if root := PackageRoot(zr); root != "" {
		if sub, err := fs.Sub(zr, strings.TrimSuffix(root, "/")); err == nil {
			tree = sub
		}
	}
	return packageFS{FS: tree, findings: findings}, nil
}

// packageFS is the tree plus what the archive declared before the tree
// normalised it. The two travel together because Validate takes an fs.FS and
// would otherwise never learn about an entry the fs view renamed; every caller
// that opens a package through PackageFS gets the archive-level findings without
// having to ask for them (ArchiveSource).
type packageFS struct {
	fs.FS
	findings []Finding
}

func (p packageFS) ArchiveFindings() []Finding { return p.findings }

// PackageRoot is the prefix inside the archive that PackageFS strips: empty when
// SKILL.md is at the top level, "dir/" when the package sits in a single
// top-level directory (the shape GitHub archive downloads have).
//
// Exported and shared with PackageFS because a writer needs the same answer a
// reader got: internal/eval rebuilds an archive with one file replaced, and a
// second root-finding rule there would write the new file next to the package
// instead of into it.
func PackageRoot(zr *zip.Reader) string {
	if _, err := fs.Stat(zr, "SKILL.md"); err == nil {
		return ""
	}
	if dirs, err := fs.ReadDir(zr, "."); err == nil && len(dirs) == 1 && dirs[0].IsDir() {
		return dirs[0].Name() + "/"
	}
	return ""
}
