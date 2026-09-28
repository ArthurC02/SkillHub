package skillpkg

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"math"
	"strings"
	"testing"
)

func zipWithComment(t *testing.T, comment string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(archiveSkillMD)); err != nil {
		t.Fatal(err)
	}
	if err := zw.SetComment(comment); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAnArchiveCommentDoesNotHideTheEndOfCentralDirectory(t *testing.T) {
	for _, tc := range []struct {
		name    string
		comment string
	}{
		{"short comment", "packed by a release script"},
		{"longest comment the format allows", strings.Repeat("c", math.MaxUint16)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fsys, err := PackageFS(zipWithComment(t, tc.comment))
			if err != nil {
				t.Fatalf("PackageFS refused a zip with a %d-byte comment: %v", len(tc.comment), err)
			}
			got, err := fs.ReadFile(fsys, "SKILL.md")
			if err != nil || string(got) != archiveSkillMD {
				t.Fatalf("SKILL.md = %q, %v; want the packed content", got, err)
			}
		})
	}
}
