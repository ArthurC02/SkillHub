package packaging

import (
	"encoding/json"
	"strings"
	"testing"
)

func pluginManifestOf(t *testing.T, f pluginFixture) map[string]any {
	t.Helper()
	p, err := planTestPlugin(f)
	if err != nil || !p.Allowed {
		t.Fatalf("plan=%+v err=%v", p, err)
	}
	var got map[string]any
	if err := json.Unmarshal(zipEntries(t, p.Zip)["plugin.json"], &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestAPluginNamesItsPublisherAsAuthor(t *testing.T) {
	got := pluginManifestOf(t, pluginFixture{spec: PluginSpec{Author: "acme"}})
	author, _ := got["author"].(map[string]any)
	if len(author) != 1 || author["name"] != "acme" {
		t.Errorf("plugin.json author = %v, want exactly {name: acme}", got["author"])
	}

	if _, present := pluginManifestOf(t, pluginFixture{})["author"]; present {
		t.Error("a plugin with no publisher still names an author")
	}
}

func TestAPluginNamesALicenseOnlyWhenItsMembersShareOne(t *testing.T) {
	mit, apache := "MIT", "Apache-2.0"
	cases := []struct {
		name    string
		members []exportedMember
		want    string
	}{
		{"one licensed member", []exportedMember{{license: &mit}}, "MIT"},
		{"two members, same license", []exportedMember{{license: &mit}, {license: &mit}}, "MIT"},
		{"two members, different licenses", []exportedMember{{license: &mit}, {license: &apache}}, ""},
		{"first member unlicensed", []exportedMember{{}, {license: &mit}}, ""},
		{"second member unlicensed", []exportedMember{{license: &mit}, {}}, ""},
		{"no members", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sharedLicense(tc.members); got != tc.want {
				t.Errorf("sharedLicense = %q, want %q", got, tc.want)
			}
		})
	}

	if got := pluginManifestOf(t, pluginFixture{license: &mit})["license"]; got != "MIT" {
		t.Errorf("plugin.json license = %v, want MIT", got)
	}
	if _, present := pluginManifestOf(t, pluginFixture{})["license"]; present {
		t.Error("a plugin whose member has no known license names one")
	}
}

func TestAPluginReadmeListsItsSkillsAndHowItIsLicensed(t *testing.T) {
	mit := "MIT"
	p, err := planTestPlugin(pluginFixture{
		skillMD: "---\nname: tidy-csv\ndescription: |\n  Tidies CSV files.\n  Use for exports.\nlicense: MIT\n---\n\nTidy it.\n",
		license: &mit, spec: PluginSpec{Description: "Tools for CSV desks.", Author: "acme"},
	})
	if err != nil || !p.Allowed {
		t.Fatalf("plan=%+v err=%v", p, err)
	}
	readme := string(zipEntries(t, p.Zip)["README.md"])
	for _, want := range []string{
		"# csv-kit 1.0.0\n\nTools for CSV desks.\n",
		"Published by acme.",
		"- `skills/tidy-csv/` — Tidies CSV files. Use for exports.\n",
		"Every Skill in this plugin is licensed MIT",
	} {
		if !strings.Contains(readme, want) {
			t.Errorf("README lacks %q:\n%s", want, readme)
		}
	}
	if strings.Contains(readme, "## MCP servers") {
		t.Errorf("a plugin calling no MCP tool has an MCP section:\n%s", readme)
	}

	unlicensed, err := planTestPlugin(pluginFixture{})
	if err != nil {
		t.Fatal(err)
	}
	if readme := string(zipEntries(t, unlicensed.Zip)["README.md"]); !strings.Contains(readme, "do not share one known license") {
		t.Errorf("README of an unlicensed plugin does not say so:\n%s", readme)
	}
}
