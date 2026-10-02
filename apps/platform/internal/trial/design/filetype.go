package testlab

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

var ErrUnsupportedType = errors.New("testlab: the file is not one of the types a dataset accepts")

const sniffLen = 512

var deniedMagic = [][]byte{
	{0x7f, 'E', 'L', 'F'},        // ELF
	{0xfe, 0xed, 0xfa, 0xce},     // Mach-O 32-bit BE
	{0xce, 0xfa, 0xed, 0xfe},     // Mach-O 32-bit LE
	{0xfe, 0xed, 0xfa, 0xcf},     // Mach-O 64-bit BE
	{0xcf, 0xfa, 0xed, 0xfe},     // Mach-O 64-bit LE
	{0xca, 0xfe, 0xba, 0xbe},     // Mach-O universal binary
	{'#', '!'},                   // script shebang
	{0x1f, 0x8b},                 // gzip
	{0xfd, '7', 'z', 'X', 'Z'},   // xz
	{'7', 'z', 0xbc, 0xaf, 0x27}, // 7z
	{'R', 'a', 'r', '!'},         // rar
}

var deniedMagicUnlessText = [][]byte{
	{'M', 'Z'},      // PE / DOS executable
	{'B', 'Z', 'h'}, // bzip2
}

var allowedSniffed = map[string]bool{
	"application/pdf": true,
	"application/zip": true,
	"image/png":       true,
	"image/jpeg":      true,
	"image/webp":      true,
}

func detectContentType(content DatasetContent) (string, error) {
	head := make([]byte, min(content.Size(), sniffLen))
	if _, err := content.ReadAt(head, 0); err != nil && !errors.Is(err, io.EOF) {
		return "", ErrUnsupportedType
	}
	for _, magic := range deniedMagic {
		if bytes.HasPrefix(head, magic) {
			return "", ErrUnsupportedType
		}
	}

	sniffed := http.DetectContentType(head)
	base, _, _ := strings.Cut(sniffed, ";")
	base = strings.TrimSpace(base)

	if strings.HasPrefix(base, "text/") {
		return base, nil
	}
	for _, magic := range deniedMagicUnlessText {
		if bytes.HasPrefix(head, magic) {
			return "", ErrUnsupportedType
		}
	}
	if !allowedSniffed[base] {
		return "", ErrUnsupportedType
	}
	if base == "application/zip" {
		if err := inspectZip(content); err != nil {
			return "", err
		}
	}
	return base, nil
}

var archiveExts = map[string]bool{
	".zip": true, ".gz": true, ".tgz": true, ".tar": true, ".bz2": true,
	".xz": true, ".7z": true, ".rar": true, ".zst": true,
}

func inspectZip(content DatasetContent) error {
	zr, err := zip.NewReader(content, content.Size())
	if err != nil {
		return ErrUnsupportedType
	}

	ooxml := false
	var files int
	var unpacked uint64
	for _, f := range zr.File {
		name := f.Name
		if name == "[Content_Types].xml" {
			ooxml = true
		}

		if escapesArchiveRoot(name) {
			return ErrUnsupportedType
		}
		if f.Mode()&fs.ModeSymlink != 0 {
			return ErrUnsupportedType
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if archiveExts[strings.ToLower(path.Ext(name))] {
			return ErrUnsupportedType
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

func escapesArchiveRoot(name string) bool {
	return path.IsAbs(name) || strings.HasPrefix(name, "/") || strings.Contains(name, `\`) ||
		name == ".." || strings.HasPrefix(name, "../") || strings.Contains(name, "/../") ||
		strings.HasSuffix(name, "/..")
}
