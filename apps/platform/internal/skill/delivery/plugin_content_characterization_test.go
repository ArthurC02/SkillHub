package packaging

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"sort"
	"testing"
)

const memberSkillMD = "---\nname: tidy-csv\ndescription: Tidies CSV files.\nlicense: MIT\n---\n\nTidy it.\n"

func zipEntries(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = body
	}
	return out
}

func TestAPluginManifestCarriesTheSchemaNameVersionAndDescriptionOfTheSpec(t *testing.T) {
	p := pluginOfOneMemberDescribed(t, nil, "Tools for CSV desks.")

	raw, ok := zipEntries(t, p.Zip)["plugin.json"]
	if !ok {
		t.Fatal("the plugin zip has no plugin.json at its root")
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("plugin.json is not JSON: %v", err)
	}
	want := map[string]any{
		"$schema":     "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
		"name":        "csv-kit",
		"version":     "1.0.0",
		"description": "Tools for CSV desks.",
	}
	if len(got) != len(want) {
		t.Errorf("plugin.json = %v, want exactly the keys of %v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("plugin.json[%q] = %v, want %v", key, got[key], value)
		}
	}
}

func TestAPluginZipHoldsOnlyItsManifestAndTheMembersFilesUnderSkills(t *testing.T) {
	p := pluginOfOneMember(t, map[string]string{
		"references/guide.md": "Read me.\n",
		ManifestFile:          `{"hooks":{"on":"start"}}`,
	})

	var got []string
	for name := range zipEntries(t, p.Zip) {
		got = append(got, name)
	}
	sort.Strings(got)
	want := []string{"plugin.json", "skills/tidy-csv/SKILL.md", "skills/tidy-csv/references/guide.md"}
	if len(got) != len(want) {
		t.Fatalf("plugin zip files = %v, want exactly %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("plugin zip files = %v, want exactly %v", got, want)
		}
	}
}

func TestAPluginMemberSkillMDIsCopiedByteForByte(t *testing.T) {
	p := pluginOfOneMember(t, nil)

	got, ok := zipEntries(t, p.Zip)["skills/tidy-csv/SKILL.md"]

	if !ok || string(got) != memberSkillMD {
		t.Errorf("the member SKILL.md = %q (present %v), want the source bytes %q", got, ok, memberSkillMD)
	}
}
