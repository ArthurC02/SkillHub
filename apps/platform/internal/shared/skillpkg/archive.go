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

const MaxZipBytes = 10 << 20

func HumanMB(n int64) string {
	return strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64) + " MB"
}

var maxUnpackedBytes = uint64(100 << 20)

var (
	maxArchiveEntries = 2000 // one-number: maxSkillPackageEntries

	maxEntryBytes = uint64(10 << 20)

	maxEntryDepth = 10
)

type ArchiveLimits struct {
	ZipBytes      int64
	UnpackedBytes int64
	Entries       int
	EntryBytes    int64
	EntryDepth    int
}

func Limits() ArchiveLimits {
	return ArchiveLimits{
		ZipBytes:      MaxZipBytes,
		UnpackedBytes: int64(maxUnpackedBytes),
		Entries:       maxArchiveEntries,
		EntryBytes:    int64(maxEntryBytes),
		EntryDepth:    maxEntryDepth,
	}
}

var ErrBadArchive = errors.New("bad archive")

type ArchiveRefusal string

const (
	ArchiveNotZip       ArchiveRefusal = "not_zip"
	ArchiveUnsafeName   ArchiveRefusal = "unsafe_name"
	ArchiveBeyondLimits ArchiveRefusal = "beyond_limits"
	ArchiveEncrypted    ArchiveRefusal = "encrypted"
	ArchiveUnsupported  ArchiveRefusal = "unsupported_feature"
	ArchiveCorrupt      ArchiveRefusal = "corrupt"
)

type ArchiveError struct {
	Refusal ArchiveRefusal
	Detail  string
}

func (e *ArchiveError) Error() string { return ErrBadArchive.Error() + ": " + e.Detail }

func (e *ArchiveError) Unwrap() error { return ErrBadArchive }

func (e *ArchiveError) Message() string {
	switch e.Refusal {
	case ArchiveNotZip:
		return "這個檔案不是 zip 套件。"
	case ArchiveUnsafeName:
		return "套件裡有平台不會解開的檔名:重複、指向套件外面,或是同一個名字同時當檔案又當資料夾。"
	case ArchiveBeyondLimits:
		return fmt.Sprintf("這個套件超過匯入上限:最多 %d 個項目、單一檔案 %s、解開後總共 %s、資料夾最多 %d 層。",
			maxArchiveEntries, HumanMB(int64(maxEntryBytes)), HumanMB(int64(maxUnpackedBytes)), maxEntryDepth)
	case ArchiveEncrypted:
		return "套件裡有加密的項目,平台不會解開需要密碼的內容。"
	case ArchiveUnsupported:
		return "這個 zip 用了平台不支援的功能(例如 zip64 或非標準的壓縮方式),請用一般的 zip 重新打包。"
	default:
		return "這個 zip 讀不完整,可能在產生或傳輸的過程中壞掉了,請重新打包或重新下載之後再試一次。"
	}
}

func badArchive(refusal ArchiveRefusal, detail string, args ...any) error {
	return &ArchiveError{Refusal: refusal, Detail: fmt.Sprintf(detail, args...)}
}

func PackageFS(data []byte) (fs.FS, error) {
	if err := validateZipEnvelope(data); err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, badArchive(ArchiveNotZip, "not a zip archive")
	}
	if len(zr.File) > maxArchiveEntries {
		return nil, badArchive(ArchiveBeyondLimits, "archive holds %d entries, more than the %d allowed", len(zr.File), maxArchiveEntries)
	}
	var unpacked uint64
	var findings []Finding
	seen := make(map[string]bool, len(zr.File))
	requiredDirs := make(map[string]struct{}, len(zr.File))
	for _, f := range zr.File {
		if f.Name == "" || strings.ContainsRune(f.Name, 0) || !utf8.ValidString(f.Name) {
			return nil, badArchive(ArchiveUnsafeName, "archive has an empty or invalid UTF-8 entry name")
		}
		nameIsDir := strings.HasSuffix(f.Name, "/")
		mode := f.Mode()
		if mode.IsDir() != nameIsDir || (nameIsDir && mode&fs.ModeSymlink != 0) {
			return nil, badArchive(ArchiveUnsafeName, "archive entry type disagrees with its name for %q", f.Name)
		}
		hasZip64, extraErr := hasZip64Extra(f.Extra)
		if extraErr != nil {
			return nil, badArchive(ArchiveCorrupt, "malformed extra field for %q: %v", f.Name, extraErr)
		}
		if hasZip64 {
			return nil, badArchive(ArchiveUnsupported, "unsupported zip64 entry %q", f.Name)
		}
		if f.Flags&1 != 0 {
			return nil, badArchive(ArchiveEncrypted, "encrypted archive entry %q", f.Name)
		}
		if f.Method != zip.Store && f.Method != zip.Deflate {
			return nil, badArchive(ArchiveUnsupported, "unsupported compression method %d for %q", f.Method, f.Name)
		}
		finding, escapes := ArchiveEntryFinding(f.Name)
		name := canonicalArchiveName(f.Name)
		if !escapes && !isCanonicalArchiveName(f.Name) {
			return nil, badArchive(ArchiveUnsafeName, "archive entry has a non-canonical portable name %q", f.Name)
		}
		for _, part := range strings.Split(strings.TrimSuffix(f.Name, "/"), "/") {
			if len(part) > 255 {
				return nil, badArchive(ArchiveBeyondLimits, "archive entry component exceeds 255 bytes in %q", f.Name)
			}
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, badArchive(ArchiveUnsafeName, "duplicate archive entry %q", f.Name)
		}
		portable := strings.TrimSuffix(name, "/")
		parts := strings.Split(portable, "/")
		for i := 1; i < len(parts); i++ {
			ancestor := strings.Join(parts[:i], "/")
			if isDir, exists := seen[ancestor]; exists && !isDir {
				return nil, badArchive(ArchiveUnsafeName, "archive file %q is an ancestor of %q", ancestor, f.Name)
			}
			requiredDirs[ancestor] = struct{}{}
		}
		if !nameIsDir {
			if _, neededAsDir := requiredDirs[portable]; neededAsDir {
				return nil, badArchive(ArchiveUnsafeName, "archive file %q conflicts with a descendant entry", f.Name)
			}
		}
		seen[portable] = nameIsDir
		if f.UncompressedSize64 > maxEntryBytes {
			return nil, badArchive(ArchiveBeyondLimits, "%s declares %d bytes, more than the %d allowed for one file",
				f.Name, f.UncompressedSize64, maxEntryBytes)
		}
		if depth := strings.Count(strings.Trim(f.Name, "/"), "/"); depth > maxEntryDepth {
			return nil, badArchive(ArchiveBeyondLimits, "%s nests %d directories deep, more than the %d allowed",
				f.Name, depth, maxEntryDepth)
		}
		unpacked += f.UncompressedSize64
		if unpacked > maxUnpackedBytes {
			return nil, badArchive(ArchiveBeyondLimits, "uncompressed content exceeds %d bytes", maxUnpackedBytes)
		}

		if !nameIsDir && LooksLikeArchive(f.Name) {
			findings = append(findings, Finding{Severity: SeverityInfo, Code: CodeNestedArchive, Path: f.Name,
				Message: "這個套件裡有一個壓縮檔，平台沒有打開它——上面的解壓上限管的是平台自己解開的內容，不涵蓋它。解壓縮這個套件的人要自己決定要不要打開。"})
		}

		if escapes {
			findings = append(findings, finding)
		}
		if !f.FileInfo().IsDir() {
			r, err := f.Open()
			if err != nil {
				return nil, badArchive(ArchiveCorrupt, "cannot open entry %q: %v", f.Name, err)
			}
			_, readErr := io.Copy(io.Discard, r)
			closeErr := r.Close()
			if readErr != nil || closeErr != nil {
				return nil, badArchive(ArchiveCorrupt, "corrupt entry %q: %v", f.Name, errors.Join(readErr, closeErr))
			}
		}
	}
	var tree fs.FS = zr
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
		return badArchive(ArchiveNotZip, "not a zip archive")
	}
	// The end-of-central-directory record sits at the very end of the file but
	// may be preceded by a comment of up to 65535 bytes, so scan backward for
	// its signature instead of assuming a fixed offset.
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
		return badArchive(ArchiveCorrupt, "end of central directory not found")
	}
	if eocd >= 20 && binary.LittleEndian.Uint32(data[eocd-20:eocd-16]) == zip64LocatorSignature {
		return badArchive(ArchiveUnsupported, "unsupported zip64 archive")
	}
	entries := binary.LittleEndian.Uint16(data[eocd+10 : eocd+12])
	cdSize := binary.LittleEndian.Uint32(data[eocd+12 : eocd+16])
	cdOffset := binary.LittleEndian.Uint32(data[eocd+16 : eocd+20])
	if entries == 0xffff || cdSize == 0xffffffff || cdOffset == 0xffffffff {
		return badArchive(ArchiveUnsupported, "unsupported zip64 archive")
	}
	if entries > 0 && (len(data) < 4 || binary.LittleEndian.Uint32(data[:4]) != 0x04034b50) {
		return badArchive(ArchiveCorrupt, "prefixed zip archive")
	}
	if uint64(cdOffset)+uint64(cdSize) != uint64(eocd) {
		return badArchive(ArchiveCorrupt, "prefixed or malformed zip archive")
	}
	return nil
}

// hasZip64Extra walks the extra field as a sequence of (id, size, payload)
// records, since a zip entry may carry several unrelated extra blocks
// back to back.
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

// canonicalArchiveName folds the name variations that extract to the same
// path on a case-insensitive or Windows-hosted filesystem: separators, case,
// and trailing dots or spaces.
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

type packageFS struct {
	fs.FS
	findings []Finding
}

func (p packageFS) ArchiveFindings() []Finding { return p.findings }

func PackageRoot(zr *zip.Reader) string {
	if _, err := fs.Stat(zr, "SKILL.md"); err == nil {
		return ""
	}
	if dirs, err := fs.ReadDir(zr, "."); err == nil && len(dirs) == 1 && dirs[0].IsDir() {
		return dirs[0].Name() + "/"
	}
	return ""
}

// A stored package may be a whole Agent Plugin or a repository holding several
// Skills, so a Skill Version records which directory inside it is its own root.
func SkillFS(data []byte, sourcePath string) (fs.FS, error) {
	fsys, err := PackageFS(data)
	if err != nil || sourcePath == "" {
		return fsys, err
	}
	return fs.Sub(fsys, sourcePath)
}

type StoredSkill struct {
	ObjectKey  string
	SourcePath string
}

func (s StoredSkill) Open(data []byte) (fs.FS, error) { return SkillFS(data, s.SourcePath) }
