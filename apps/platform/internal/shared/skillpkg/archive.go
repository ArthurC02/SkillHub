package skillpkg

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"
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

// MaxZipBytes caps the uploaded archive. PDM-005 §5.1b's number, ratified by the
// product owner on 2026-08-27 (`05` R-13, `04` 乙-23).
//
// It was 32 MiB for months, and the comment standing here defended the gap:
// lowering it 「rejects packages that import successfully today」. That reasoning
// was sound and the premise was never measured. When it finally was: 45 package
// objects, largest 0.156 MB, p95 0.066 MB, and ZERO between 10 MB and 32 MiB.
// Nothing was rejected by this change. **A ceiling is a one-way door** — raising
// it breaks nobody, lowering it breaks anyone already over — and the cost of
// walking through the reversible side was zero today and stops being zero the
// first time a beta user imports a Skill that ships a font or a notebook.
//
// The ratification also ended a three-way disagreement rather than just a
// two-way one: `02:SEC-003`'s acceptance sentence says 10 MB and adds that both
// import paths share one set of limits. Sharing was implemented (fetch and
// upload read this same constant); the shared VALUE was not the stated one, so
// `03:INGEST-014` was structurally unticklable however well the fetcher was
// written.
//
// The other five ceilings §5.1b names had no value at all until 2026-08-25 —
// see the block below. `03` §1 said PDM-005's numbers were 值已全數強制; for §5.1b
// that was true of two of seven.
const MaxZipBytes = 10 << 20 // 10 MB, PDM-005 §5.1b

// HumanMB renders a byte ceiling the way a refusal has to say it (03:INGEST-016).
//
// One decimal and not `n>>20`, because the two numbers a refusal prints are a
// ceiling and an actual size, and truncation makes the interesting case unsayable:
// 32.4 MB against a 32 MiB cap would print "over the 32 MB limit (this was 32 MB)".
func HumanMB(n int64) string {
	return strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64) + " MB"
}

// maxUnpackedBytes caps total declared uncompressed size (zip bombs,
// ADR-007 壓縮炸彈). §5.1b's second number, ratified with the first. Var only so
// tests can lower it.
var maxUnpackedBytes = uint64(100 << 20) // 100 MB, PDM-005 §5.1b

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
	maxArchiveEntries = 2000 // one-number: maxSkillPackageEntries
	// 單一檔案大小 ≤ 10 MB. Declared size, checked before anything is read:
	// the total cap alone lets one entry claim all of it.
	maxEntryBytes = uint64(10 << 20)
	// 目錄巢狀深度 ≤ 10.
	maxEntryDepth = 10
)

// §5.1b's two remaining clauses are deliberately not here.
//
// **Symlinks are refused by validation.** The archive reader preserves their
// mode and target for a precise finding; validation blocks them because the
// runtime extractor accepts regular files only. Admission and execution must
// agree on which stored package bytes are runnable.
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
	if err := validateZipEnvelope(data); err != nil {
		return nil, err
	}
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
	seen := make(map[string]bool, len(zr.File)) // portable name -> directory
	requiredDirs := make(map[string]struct{}, len(zr.File))
	for _, f := range zr.File {
		if f.Name == "" || strings.ContainsRune(f.Name, 0) || !utf8.ValidString(f.Name) {
			return nil, fmt.Errorf("%w: archive has an empty or invalid UTF-8 entry name", ErrBadArchive)
		}
		nameIsDir := strings.HasSuffix(f.Name, "/")
		mode := f.Mode()
		if mode.IsDir() != nameIsDir || (nameIsDir && mode&fs.ModeSymlink != 0) {
			return nil, fmt.Errorf("%w: archive entry type disagrees with its name for %q", ErrBadArchive, f.Name)
		}
		hasZip64, extraErr := hasZip64Extra(f.Extra)
		if extraErr != nil {
			return nil, fmt.Errorf("%w: malformed extra field for %q: %v", ErrBadArchive, f.Name, extraErr)
		}
		if hasZip64 {
			return nil, fmt.Errorf("%w: unsupported zip64 entry %q", ErrBadArchive, f.Name)
		}
		if f.Flags&1 != 0 {
			return nil, fmt.Errorf("%w: encrypted archive entry %q", ErrBadArchive, f.Name)
		}
		if f.Method != zip.Store && f.Method != zip.Deflate {
			return nil, fmt.Errorf("%w: unsupported compression method %d for %q", ErrBadArchive, f.Method, f.Name)
		}
		finding, escapes := ArchiveEntryFinding(f.Name)
		name := canonicalArchiveName(f.Name)
		if !escapes && !isCanonicalArchiveName(f.Name) {
			return nil, fmt.Errorf("%w: archive entry has a non-canonical portable name %q", ErrBadArchive, f.Name)
		}
		for _, part := range strings.Split(strings.TrimSuffix(f.Name, "/"), "/") {
			if len(part) > 255 {
				return nil, fmt.Errorf("%w: archive entry component exceeds 255 bytes in %q", ErrBadArchive, f.Name)
			}
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("%w: duplicate archive entry %q", ErrBadArchive, f.Name)
		}
		portable := strings.TrimSuffix(name, "/")
		parts := strings.Split(portable, "/")
		for i := 1; i < len(parts); i++ {
			ancestor := strings.Join(parts[:i], "/")
			if isDir, exists := seen[ancestor]; exists && !isDir {
				return nil, fmt.Errorf("%w: archive file %q is an ancestor of %q", ErrBadArchive, ancestor, f.Name)
			}
			requiredDirs[ancestor] = struct{}{}
		}
		if !nameIsDir {
			if _, neededAsDir := requiredDirs[portable]; neededAsDir {
				return nil, fmt.Errorf("%w: archive file %q conflicts with a descendant entry", ErrBadArchive, f.Name)
			}
		}
		seen[portable] = nameIsDir
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
		if escapes {
			findings = append(findings, finding)
		}
		if !f.FileInfo().IsDir() {
			r, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("%w: cannot open entry %q: %v", ErrBadArchive, f.Name, err)
			}
			_, readErr := io.Copy(io.Discard, r)
			closeErr := r.Close()
			if readErr != nil || closeErr != nil {
				return nil, fmt.Errorf("%w: corrupt entry %q: %v", ErrBadArchive, f.Name, errors.Join(readErr, closeErr))
			}
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

func validateZipEnvelope(data []byte) error {
	const (
		eocdSignature         = 0x06054b50
		zip64LocatorSignature = 0x07064b50
	)
	if len(data) < 22 {
		return fmt.Errorf("%w: not a zip archive", ErrBadArchive)
	}
	min := len(data) - 22 - 65535
	if min < 0 {
		min = 0
	}
	eocd := -1
	for i := len(data) - 22; i >= min; i-- {
		if binary.LittleEndian.Uint32(data[i:i+4]) == eocdSignature &&
			i+22+int(binary.LittleEndian.Uint16(data[i+20:i+22])) == len(data) {
			eocd = i
			break
		}
	}
	if eocd < 0 {
		return fmt.Errorf("%w: end of central directory not found", ErrBadArchive)
	}
	if eocd >= 20 && binary.LittleEndian.Uint32(data[eocd-20:eocd-16]) == zip64LocatorSignature {
		return fmt.Errorf("%w: unsupported zip64 archive", ErrBadArchive)
	}
	entries := binary.LittleEndian.Uint16(data[eocd+10 : eocd+12])
	cdSize := binary.LittleEndian.Uint32(data[eocd+12 : eocd+16])
	cdOffset := binary.LittleEndian.Uint32(data[eocd+16 : eocd+20])
	if entries == 0xffff || cdSize == 0xffffffff || cdOffset == 0xffffffff {
		return fmt.Errorf("%w: unsupported zip64 archive", ErrBadArchive)
	}
	if entries > 0 && (len(data) < 4 || binary.LittleEndian.Uint32(data[:4]) != 0x04034b50) {
		return fmt.Errorf("%w: prefixed zip archive", ErrBadArchive)
	}
	if uint64(cdOffset)+uint64(cdSize) != uint64(eocd) {
		return fmt.Errorf("%w: prefixed or malformed zip archive", ErrBadArchive)
	}
	return nil
}

func hasZip64Extra(extra []byte) (bool, error) {
	for len(extra) > 0 {
		if len(extra) < 4 {
			return false, errors.New("truncated extra-field header")
		}
		id := uint16(extra[0]) | uint16(extra[1])<<8
		size := int(extra[2]) | int(extra[3])<<8
		if id == 0x0001 {
			return true, nil
		}
		if size > len(extra)-4 {
			return false, errors.New("extra-field payload exceeds its container")
		}
		extra = extra[4+size:]
	}
	return false, nil
}

// canonicalArchiveName follows the aliases shared by supported extractors:
// slash direction and dot segments on every platform, plus case and trailing
// dots/spaces on Windows. It is used only for collision detection; the original
// name remains the one exposed by the read-only ZIP filesystem.
func canonicalArchiveName(name string) string {
	parts := strings.Split(path.Clean(strings.ReplaceAll(name, `\`, "/")), "/")
	for i := range parts {
		parts[i] = strings.ToLower(strings.TrimRight(parts[i], " ."))
	}
	return strings.Join(parts, "/")
}

func isCanonicalArchiveName(name string) bool {
	trimmed := strings.TrimSuffix(name, "/")
	if trimmed == "" || path.Clean(trimmed) != trimmed {
		return false
	}
	for _, part := range strings.Split(trimmed, "/") {
		if part == "" || part == "." || strings.TrimRight(part, " .") != part ||
			strings.ContainsAny(part, `<>:"|?*`) || hasASCIIControl(part) || isWindowsReservedName(part) {
			return false
		}
	}
	return true
}

func hasASCIIControl(s string) bool {
	for _, r := range s {
		if r < 0x20 {
			return true
		}
	}
	return false
}

func isWindowsReservedName(part string) bool {
	base := strings.ToLower(strings.SplitN(part, ".", 2)[0])
	if base == "con" || base == "prn" || base == "aux" || base == "nul" {
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) &&
		base[3] >= '1' && base[3] <= '9'
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
