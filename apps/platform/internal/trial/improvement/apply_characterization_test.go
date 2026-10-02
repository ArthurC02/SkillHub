package eval

import (
	"archive/zip"
	"bytes"
	"io"
	"reflect"
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

	patched, err := patchArchive(buf.Bytes(), "", map[string]string{"SKILL.md": "---\nname: demo\ndescription: A demo.\nlicense: MIT\n---\n\nNew body.\n"})
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

func TestPatchingASkillInsideAPluginPackageRewritesOnlyThatSkillAsItsOwnPackage(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		".claude-plugin/plugin.json": `{"name":"kit"}`,
		"skills/alpha/SKILL.md":      "---\nname: alpha\ndescription: Alpha.\nlicense: MIT\n---\n\nOld alpha.\n",
		"skills/alpha/notes.txt":     "alpha notes\n",
		"skills/beta/SKILL.md":       "---\nname: beta\ndescription: Beta.\nlicense: MIT\n---\n\nBeta stays.\n",
	} {
		w, err := zw.Create(name)
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
	newAlpha := "---\nname: alpha\ndescription: Alpha.\nlicense: MIT\n---\n\nNew alpha.\n"

	patched, err := patchArchive(buf.Bytes(), "skills/alpha", map[string]string{"SKILL.md": newAlpha})
	if err != nil {
		t.Fatal(err)
	}

	zr, err := zip.NewReader(bytes.NewReader(patched), int64(len(patched)))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(rc)
		rc.Close()
		got[f.Name] = string(body)
	}
	want := map[string]string{"SKILL.md": newAlpha, "notes.txt": "alpha notes\n"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("patched package = %v, want only alpha's own files with the change at its root", got)
	}
}
