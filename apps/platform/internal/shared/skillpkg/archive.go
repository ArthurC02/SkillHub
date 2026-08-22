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
// ponytail: flat cap until PDM-005 fixes real limits.
const MaxZipBytes = 32 << 20 // 32 MiB

// maxUnpackedBytes caps total declared uncompressed size (zip bombs,
// ADR-007 壓縮炸彈). Var only so tests can lower it.
var maxUnpackedBytes = uint64(256 << 20) // 256 MiB

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
	var unpacked uint64
	var findings []Finding
	for _, f := range zr.File {
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
