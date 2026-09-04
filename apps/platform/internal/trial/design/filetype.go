package testlab

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// ErrUnsupportedType: the content is not one of the PDM-005 §5.1 allowed kinds.
// The message is deliberately the same for every rejected kind — 02:TEST-002
// requires an error that does not leak system information, so the caller never
// learns which magic bytes were detected or which rule fired.
var ErrUnsupportedType = errors.New("不支援這種檔案類型")

// sniffLen is what http.DetectContentType actually reads.
const sniffLen = 512

// deniedMagic is the PDM-005 "明確拒絕" list: native executables. These are
// checked before the allow-list rather than left to fall through it, because
// "not in the allow-list" depends on a sniffer's judgement and this does not.
// A PE/ELF renamed to .csv is rejected here, which is the whole point of judging
// by magic bytes instead of by file extension.
var deniedMagic = [][]byte{
	{0x7f, 'E', 'L', 'F'},        // ELF
	{'M', 'Z'},                   // PE / DOS executable
	{0xfe, 0xed, 0xfa, 0xce},     // Mach-O 32-bit BE
	{0xce, 0xfa, 0xed, 0xfe},     // Mach-O 32-bit LE
	{0xfe, 0xed, 0xfa, 0xcf},     // Mach-O 64-bit BE
	{0xcf, 0xfa, 0xed, 0xfe},     // Mach-O 64-bit LE
	{0xca, 0xfe, 0xba, 0xbe},     // Mach-O universal binary
	{'#', '!'},                   // script shebang
	{0x1f, 0x8b},                 // gzip: an archive we do not unpack (nested-archive rule)
	{'B', 'Z', 'h'},              // bzip2
	{0xfd, '7', 'z', 'X', 'Z'},   // xz
	{'7', 'z', 0xbc, 0xaf, 0x27}, // 7z
	{'R', 'a', 'r', '!'},         // rar
}

// allowedSniffed maps what http.DetectContentType reports onto the PDM-005
// families. Textual formats (.txt .md .csv .tsv .json .jsonl .xml .yaml .yml)
// all sniff as some text/* type and share one entry; there is no magic number
// that tells CSV from TSV, and no reason to care — they are data either way.
// OOXML documents (.docx .xlsx .pptx) sniff as application/zip and are checked
// by inspectZip below.
var allowedSniffed = map[string]bool{
	"application/pdf": true,
	"application/zip": true,
	"image/png":       true,
	"image/jpeg":      true,
	"image/webp":      true,
}

// detectContentType returns the stored content type for data, judged by content
// only (PDM-005: "以 magic bytes 判定，不信任副檔名"). The file name is never
// consulted — not even as a tie-breaker — so the type recorded on the dataset
// row is what the bytes say, not what the uploader claimed.
func detectContentType(data []byte) (string, error) {
	head := data
	if len(head) > sniffLen {
		head = head[:sniffLen]
	}
	for _, magic := range deniedMagic {
		if bytes.HasPrefix(head, magic) {
			return "", ErrUnsupportedType
		}
	}

	sniffed := http.DetectContentType(head)
	base, _, _ := strings.Cut(sniffed, ";")
	base = strings.TrimSpace(base)

	// Any text/* is accepted as the text family: JSON, CSV and YAML all report
	// text/plain, XML reports text/xml, and a Markdown file that opens with an
	// HTML comment reports text/html. All three are data, none is executed
	// (iron rule 1), so splitting them here would only reject valid uploads.
	if strings.HasPrefix(base, "text/") {
		return base, nil
	}
	if !allowedSniffed[base] {
		return "", ErrUnsupportedType
	}
	if base == "application/zip" {
		if err := inspectZip(data); err != nil {
			return "", err
		}
	}
	return base, nil
}

// archiveExts are archive formats inside an archive: PDM-005 allows one layer
// of zip and rejects nesting, which is the standard way to smuggle content past
// an unpack budget.
var archiveExts = map[string]bool{
	".zip": true, ".gz": true, ".tgz": true, ".tar": true, ".bz2": true,
	".xz": true, ".7z": true, ".rar": true, ".zst": true,
}

// inspectZip enforces the PDM-005 archive rules on a zip dataset: single layer,
// no symlinks, no path traversal, and — for a plain archive — the per-test-case
// file count and size budget applied to the unpacked content, since that is what
// the sandbox will mount.
//
// The FILE COUNT limit is skipped for OOXML containers (.docx/.xlsx/.pptx). Those
// are single documents that happen to be zips; a normal .xlsx carries well over
// 20 internal parts, and counting them as 20 user files would reject every
// spreadsheet ever uploaded.
//
// The unpacked-BYTES limit is not skipped, and used to be. An entry named
// [Content_Types].xml turned off both budgets, so the whole PDM-005 §5.1 unpack
// allowance for a zip dataset came off one filename - and the only remaining
// upper bound was MaxFileBytes on the compressed side, which deflate turns into
// several GB of declared content. A real .xlsx is nowhere near 100 MB unpacked,
// so keeping this limit costs the case it was skipped for nothing.
//
// Declared sizes from the central directory are enough here: the file is never
// unpacked on this side (it is unpacked inside the sandbox, ADR-005), so this is
// a budget check rather than a zip-bomb defence.
func inspectZip(data []byte) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ErrUnsupportedType // a zip sniff that will not open is not a zip
	}

	ooxml := false
	var files int
	var unpacked uint64
	for _, f := range zr.File {
		name := f.Name
		if name == "[Content_Types].xml" {
			ooxml = true
		}
		// Path traversal and absolute paths (PDM-005 "路徑含 .. 的封存項目").
		if path.IsAbs(name) || strings.HasPrefix(name, "/") || strings.Contains(name, `\`) ||
			name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "/../") ||
			strings.HasSuffix(name, "/..") {
			return ErrUnsupportedType
		}
		if f.Mode()&fs.ModeSymlink != 0 {
			return ErrUnsupportedType
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if archiveExts[strings.ToLower(path.Ext(name))] {
			return ErrUnsupportedType // nested archive
		}
		files++
		unpacked += f.UncompressedSize64
	}
	if files > MaxFilesPerTestCase && !ooxml {
		return fmt.Errorf("%w: 壓縮檔裡超過 %d 個檔案", ErrLimitExceeded, MaxFilesPerTestCase)
	}
	if unpacked > uint64(MaxTestCaseBytes) {
		return fmt.Errorf("%w: 壓縮檔解開後超過 %s", ErrLimitExceeded, humanMB(MaxTestCaseBytes))
	}
	return nil
}
