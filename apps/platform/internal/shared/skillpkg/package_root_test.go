package skillpkg

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

type packageRootCase struct {
	Name    string   `json:"name"`
	Entries []string `json:"entries"`
	Root    string   `json:"root"`
}

func packageRootCases(t *testing.T) []packageRootCase {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "contracts", "packaging", "package-root-cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Cases []packageRootCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Cases) == 0 {
		t.Fatal("package-root-cases.json holds no case")
	}
	return doc.Cases
}

func TestPackageRootAgreesWithTheRuntimeOnEveryCase(t *testing.T) {
	for _, tc := range packageRootCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			files := map[string]string{}
			for _, name := range tc.Entries {
				files[name] = ""
			}
			zr, err := zip.NewReader(bytes.NewReader(zipBytes(t, files)), int64(len(zipBytes(t, files))))
			if err != nil {
				t.Fatal(err)
			}
			if got := PackageRoot(zr); got != tc.Root {
				t.Fatalf("PackageRoot(%v) = %q, want %q: the runtime installs from that root, so a Skill "+
					"directory recorded here would not be found there", tc.Entries, got, tc.Root)
			}
		})
	}
}

func TestAPluginInsideARepositoryDirectoryOpensEachSkillFromItsRecordedPath(t *testing.T) {
	const tidy = "---\nname: tidy-notes\ndescription: Tidy notes.\n---\nBody.\n"
	data := zipBytes(t, map[string]string{
		"desk-tools-main/plugin.json":                conformingManifest("desk-tools"),
		"desk-tools-main/skills/tidy-notes/SKILL.md": tidy,
		"desk-tools-main/skills/split-csv/SKILL.md":  "---\nname: split-csv\ndescription: Split.\n---\n",
	})
	fsys, err := PackageFS(data)
	if err != nil {
		t.Fatal(err)
	}
	d := Discover(fsys)
	if d.Shape != ShapePlugin || len(d.Skills) != 2 || d.Skills[1] != "skills/tidy-notes" {
		t.Fatalf("discovery = %s with %v, want a plugin with skills/split-csv and skills/tidy-notes", d.Shape, d.Skills)
	}
	skill, err := SkillFS(data, d.Skills[1])
	if err != nil {
		t.Fatal(err)
	}
	if got, err := fs.ReadFile(skill, "SKILL.md"); err != nil || string(got) != tidy {
		t.Fatalf("SKILL.md at the recorded path = %q, %v; want the plugin's tidy-notes", got, err)
	}
}
