package skillpkg

import (
	"strings"
	"testing"
	"testing/fstest"
)

func tree(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for p, content := range files {
		m[p] = &fstest.MapFile{Data: []byte(content)}
	}
	return m
}

func conformingManifest(name string) string {
	return `{"$schema":"` + PluginSchemaID + `","name":"` + name + `"}`
}

func discoveryCodes(d Discovery) map[string]Severity {
	out := map[string]Severity{}
	for _, f := range append(append([]Finding{}, d.Findings...), d.Excluded...) {
		out[f.Code] = f.Severity
	}
	return out
}

func TestDiscoverReadsAPluginOnlyWhenItDeclaresTheSharedSpecification(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		manifest  string
		wantShape SourceShape
		wantCode  string
		wantSev   Severity
	}{{
		name:      "the shared specification is what makes it a plugin",
		manifest:  conformingManifest("my-plugin"),
		wantShape: ShapePlugin,
	}, {
		name:      "another vendor's schema is read by nobody and blocks nobody",
		manifest:  `{"$schema":"https://example.invalid/schemas/v1/plugin.json","name":"my-plugin"}`,
		wantShape: ShapeTree,
		wantCode:  CodeForeignManifest,
		wantSev:   SeverityInfo,
	}, {
		name:      "no schema at all cannot be attributed to any specification",
		manifest:  `{"name":"my-plugin"}`,
		wantShape: ShapeTree,
		wantCode:  CodeForeignManifest,
		wantSev:   SeverityInfo,
	}, {
		name:      "a manifest that is not JSON is not a manifest",
		manifest:  `{ this is not json`,
		wantShape: ShapeTree,
		wantCode:  CodeForeignManifest,
		wantSev:   SeverityInfo,
	}, {
		name:      "claiming the shared specification and failing it is an error",
		manifest:  `{"$schema":"` + PluginSchemaID + `","name":"Not-Kebab"}`,
		wantShape: ShapeTree,
		wantCode:  CodePluginManifest,
		wantSev:   SeverityError,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := Discover(tree(map[string]string{
				"plugin.json":               tc.manifest,
				"skills/alpha/SKILL.md":     "---\nname: alpha\n---\n",
				"skills/alpha/scripts/x.py": "print()\n",
			}))

			if d.Shape != tc.wantShape {
				t.Errorf("shape = %q, want %q", d.Shape, tc.wantShape)
			}
			if tc.wantCode == "" {
				if len(d.Findings) != 0 {
					t.Errorf("a clean conforming manifest reported %+v", d.Findings)
				}
				return
			}
			got := discoveryCodes(d)
			if got[tc.wantCode] != tc.wantSev {
				t.Errorf("findings = %v, want %s at %s", got, tc.wantCode, tc.wantSev)
			}
			if (tc.wantSev == SeverityError) != d.Blocked {
				t.Errorf("Blocked = %v for a %s finding", d.Blocked, tc.wantSev)
			}
		})
	}
}

func TestDiscoverAcceptsExactlyThePluginNamesTheSpecificationAllows(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		value string
		want  bool
	}{
		{"one character is the lower bound", "a", true},
		{"empty is below it", "", false},
		{"sixty-four characters is the upper bound", strings.Repeat("a", 64), true},
		{"sixty-five is over it", strings.Repeat("a", 65), false},
		{"a dot is allowed, unlike the skill name rule", "com.example.tools", true},
		{"a hyphen inside is allowed", "my-plugin", true},
		{"consecutive hyphens are not", "my--plugin", false},
		{"consecutive dots are not", "my..plugin", false},
		{"a leading hyphen is not", "-plugin", false},
		{"a trailing dot is not", "plugin.", false},
		{"uppercase is not", "My-Plugin", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := validPluginName(tc.value); got != tc.want {
				t.Errorf("validPluginName(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestDiscoverTakesPluginSkillsFromTheFixedLocationWithoutRecursing(t *testing.T) {
	t.Parallel()
	d := Discover(tree(map[string]string{
		"plugin.json":                 conformingManifest("my-plugin"),
		"skills/alpha/SKILL.md":       "---\nname: alpha\n---\n",
		"skills/beta/SKILL.md":        "---\nname: beta\n---\n",
		"skills/beta/deeper/SKILL.md": "---\nname: deeper\n---\n",
		"skills/SKILL.md":             "---\nname: loose\n---\n",
		"docs/guide/SKILL.md":         "---\nname: elsewhere\n---\n",
	}))

	want := []string{"skills/alpha", "skills/beta"}
	if strings.Join(d.Skills, ",") != strings.Join(want, ",") {
		t.Fatalf("skills = %v, want exactly %v: the specification fixes the location at the immediate "+
			"children of skills/ and forbids searching deeper", d.Skills, want)
	}
}

func TestDiscoverBlocksAPluginWhoseFixedLocationHoldsNoSkill(t *testing.T) {
	t.Parallel()
	d := Discover(tree(map[string]string{
		"plugin.json":         conformingManifest("my-plugin"),
		"docs/guide/SKILL.md": "---\nname: elsewhere\n---\n",
	}))

	if !d.Blocked || discoveryCodes(d)[CodeNoSkillFound] != SeverityError {
		t.Fatalf("a plugin with an empty skills/ was accepted: %+v", d)
	}
	if !strings.Contains(d.Findings[len(d.Findings)-1].Message, "skills/") {
		t.Errorf("the refusal does not name the location it searched: %q", d.Findings[len(d.Findings)-1].Message)
	}
}

func TestDiscoverDisclosesTheComponentsItWillNeverImport(t *testing.T) {
	t.Parallel()
	d := Discover(tree(map[string]string{
		"plugin.json":                            conformingManifest("my-plugin"),
		"mcp.json":                               `{"mcpServers":{}}`,
		"com.example.client/hooks/on-start.json": "{}",
		"org.other.agent/commands/go.md":         "# go",
		"skills/alpha/SKILL.md":                  "---\nname: alpha\n---\n",
		"scripts/build.sh":                       "echo hi",
	}))

	var paths []string
	for _, f := range d.Excluded {
		if f.Code != CodePluginComponent {
			t.Errorf("excluded finding carries code %q", f.Code)
		}
		paths = append(paths, f.Path)
	}
	for _, want := range []string{"mcp.json", "com.example.client/", "org.other.agent/"} {
		if !contains(paths, want) {
			t.Errorf("%s was not disclosed as a component: %v", want, paths)
		}
	}
	if contains(paths, "scripts/") || contains(paths, "skills/") {
		t.Errorf("a plain directory was disclosed as a client namespace: %v", paths)
	}
}

func TestDiscoverFindsSkillsAnyClientLeftBehindWithoutNamingThatClient(t *testing.T) {
	t.Parallel()
	d := Discover(tree(map[string]string{
		".vendor-plugin/plugin.json":      `{"name":"legacy"}`,
		".vendor/skills/alpha/SKILL.md":   "---\nname: alpha\n---\n",
		"plugins/bravo/skills/b/SKILL.md": "---\nname: b\n---\n",
		"README.md":                       "# repo",
	}))

	if d.Shape != ShapeTree {
		t.Fatalf("shape = %q, want %q", d.Shape, ShapeTree)
	}
	want := []string{".vendor/skills/alpha", "plugins/bravo/skills/b"}
	if strings.Join(d.Skills, ",") != strings.Join(want, ",") {
		t.Fatalf("skills = %v, want %v: a skill is found by its SKILL.md, never by which client put it there",
			d.Skills, want)
	}
}

func TestDiscoverTreatsASourceRootedOnSkillMDAsOneSkill(t *testing.T) {
	t.Parallel()
	d := Discover(tree(map[string]string{
		"SKILL.md":       "---\nname: solo\n---\n",
		"scripts/run.py": "print()\n",
	}))

	if d.Shape != ShapeSkill || strings.Join(d.Skills, ",") != "." {
		t.Fatalf("shape = %q skills = %v, want a single skill at the source root", d.Shape, d.Skills)
	}
}

func TestDiscoverRefusesAnEmptySourceBySayingWhereItLooked(t *testing.T) {
	t.Parallel()
	d := Discover(tree(map[string]string{"README.md": "# nothing here"}))

	if !d.Blocked {
		t.Fatal("a source with no SKILL.md was accepted")
	}
	message := d.Findings[0].Message
	for _, place := range []string{"根目錄", "plugin.json", "skills/", "目錄樹"} {
		if !strings.Contains(message, place) {
			t.Errorf("the refusal never names %q, so it cannot tell the reader what shape is expected: %q",
				place, message)
		}
	}
}

func TestDiscoverRecordsUnknownManifestFieldsWithoutRefusingThem(t *testing.T) {
	t.Parallel()
	d := Discover(tree(map[string]string{
		"plugin.json": `{"$schema":"` + PluginSchemaID + `","name":"my-plugin",` +
			`"version":"1.2.0","repository":"https://example.invalid/r","sparkles":true,"aardvark":1}`,
		"skills/alpha/SKILL.md": "---\nname: alpha\n---\n",
	}))

	if d.Blocked {
		t.Fatal("an unknown manifest field blocked the import")
	}
	if d.Plugin == nil || d.Plugin.Version != "1.2.0" || d.Plugin.Repository != "https://example.invalid/r" {
		t.Fatalf("plugin facts = %+v, want the optional fields the manifest declared", d.Plugin)
	}
	for _, f := range d.Findings {
		if f.Code != CodePluginUnknownField {
			continue
		}
		if strings.Join(f.Details, ",") != "aardvark,sparkles" {
			t.Errorf("unknown fields = %v, want both, sorted", f.Details)
		}
		return
	}
	t.Error("the unknown fields were ignored silently; the specification requires reporting them")
}
