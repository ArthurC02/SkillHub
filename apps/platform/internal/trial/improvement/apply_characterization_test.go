package eval

import (
	"archive/zip"
	"bytes"
	"testing"
	"time"
)

func TestPatchingKeepsEachExistingEntrysModificationTime(t *testing.T) {
	stamped := time.Date(2024, 1, 2, 3, 4, 6, 0, time.UTC)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"SKILL.md":  "---\nname: demo\ndescription: A demo.\nlicense: MIT\n---\n\nOld body.\n",
		"notes.txt": "untouched\n",
	} {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: stamped})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	patched, err := patchArchive(buf.Bytes(), map[string]string{"SKILL.md": "---\nname: demo\ndescription: A demo.\nlicense: MIT\n---\n\nNew body.\n"})
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(patched), int64(len(patched)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Modified.Unix() != stamped.Unix() {
			t.Errorf("%s modified = %v, want the original %v", f.Name, f.Modified, stamped)
		}
	}
	if len(zr.File) != 2 {
		t.Errorf("patched archive has %d entries, want 2", len(zr.File))
	}
}
