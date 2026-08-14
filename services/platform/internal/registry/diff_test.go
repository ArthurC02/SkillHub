package registry

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestDiffFS(t *testing.T) {
	from := fstest.MapFS{
		"SKILL.md": &fstest.MapFile{Data: []byte("---\nname: x\n---\nold line\n")},
		"gone.txt": &fstest.MapFile{Data: []byte("bye\n")},
		"same.txt": &fstest.MapFile{Data: []byte("unchanged\n")},
		"blob.bin": &fstest.MapFile{Data: []byte{0x00, 0x01}},
	}
	to := fstest.MapFS{
		"SKILL.md":  &fstest.MapFile{Data: []byte("---\nname: x\n---\nnew line\n")},
		"added.txt": &fstest.MapFile{Data: []byte("hi\n")},
		"same.txt":  &fstest.MapFile{Data: []byte("unchanged\n")},
		"blob.bin":  &fstest.MapFile{Data: []byte{0x00, 0x02}},
	}

	diffs, err := diffFS(from, to)
	if err != nil {
		t.Fatal(err)
	}

	byPath := map[string]FileDiff{}
	for _, d := range diffs {
		byPath[d.Path] = d
	}
	if len(diffs) != 4 {
		t.Fatalf("want 4 diffs (unchanged excluded), got %d: %+v", len(diffs), diffs)
	}
	if byPath["gone.txt"].Status != "removed" || byPath["added.txt"].Status != "added" {
		t.Fatalf("added/removed wrong: %+v", byPath)
	}
	md := byPath["SKILL.md"]
	if md.Status != "modified" || !strings.Contains(md.Diff, "-old line") || !strings.Contains(md.Diff, "+new line") {
		t.Fatalf("SKILL.md diff wrong: %+v", md)
	}
	bin := byPath["blob.bin"]
	if bin.Status != "modified" || bin.Diff != "" {
		t.Fatalf("binary must report modified without text diff: %+v", bin)
	}
}
