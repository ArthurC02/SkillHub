package packaging

import (
	"strings"
	"testing"
)

func installSection(guide, heading string) (string, int) {
	start := strings.Index(guide, "## "+heading+"\n")
	if start < 0 {
		return "", -1
	}
	body := guide[start+len(heading)+4:]
	if end := strings.Index(body, "\n## "); end >= 0 {
		body = body[:end]
	}
	return body, start
}

func TestTheInstallGuidePutsEveryProfileValueInItsOwnSectionInOrder(t *testing.T) {
	p := Profile{DisplayName: "Agent", Version: "1", SupportStatus: "unverified"}
	p.Install.Locations = []InstallLocation{{Scope: "user", Path: "~/.agent/skills/<name>", Description: "per user"}}
	p.EnvVars = []EnvVar{
		{Name: "API_KEY", Required: true, Description: "The key.", Example: "abc"},
		{Name: "MODE", Description: "The mode."},
	}
	p.Snippet = "run <name>"
	p.VerificationPrompt = "use <name>"
	p.VerificationSteps = []string{"look", "see"}
	p.KnownLimitations = []string{"slow"}
	p.Notes = []string{"note one", "note two"}

	guide := renderInstall(p, "demo", []string{"requests"}, mcpNeeds{})

	if strings.Contains(guide, "<name>") {
		t.Errorf("the guide still says <name> where it should say demo:\n%s", guide)
	}
	previous := -1
	for _, tc := range []struct {
		heading string
		holds   []string
	}{
		{"Where it goes", []string{"- `~/.agent/skills/demo` (user) — per user"}},
		{"Dependencies", []string{"- requests"}},
		{"Environment variables", []string{"- `API_KEY` (required) — The key. Example: `abc`", "- `MODE` (optional) — The mode.\n"}},
		{"Minimal working example", []string{"```\nrun demo\n```"}},
		{"Check that it worked", []string{"> use demo", "1. look\n1. see"}},
		{"Known limitations", []string{"- slow", "> note one\n> note two\n"}},
	} {
		body, at := installSection(guide, tc.heading)
		if at < 0 {
			t.Errorf("no %q section:\n%s", tc.heading, guide)
			continue
		}
		if at < previous {
			t.Errorf("%q comes before the section listed ahead of it", tc.heading)
		}
		previous = at
		for _, want := range tc.holds {
			if !strings.Contains(body, want) {
				t.Errorf("%q section lacks %q:\n%s", tc.heading, want, body)
			}
		}
	}
}

func TestAnInstallGuideLeavesOutTheSectionsAProfileHasNothingFor(t *testing.T) {
	guide := renderInstall(Profile{DisplayName: "Any", Version: "1"}, "demo", nil, mcpNeeds{})

	for _, heading := range []string{"Dependencies", "Environment variables", "Minimal working example", "Known limitations"} {
		if _, at := installSection(guide, heading); at >= 0 {
			t.Errorf("an empty profile still writes a %q section:\n%s", heading, guide)
		}
	}
	for _, heading := range []string{"Where it goes", "Check that it worked"} {
		if _, at := installSection(guide, heading); at < 0 {
			t.Errorf("no %q section:\n%s", heading, guide)
		}
	}
	if where, _ := installSection(guide, "Where it goes"); strings.TrimSpace(where) == "" || strings.Contains(where, "- `") || strings.Contains(where, "Unzip") {
		t.Errorf("a profile with no install location says nothing, lists one, or says where to unzip:\n%s", where)
	}
}

func TestAnInstallGuideForAZipWithItsOwnFolderSaysToUnzipInTheParent(t *testing.T) {
	top := "<name>"
	for _, tc := range []struct {
		name     string
		path     string
		top      *string
		holds    string
		notHolds string
	}{
		{"a wrapped zip whose path ends in its folder", "~/.claude/skills/<name>/", &top,
			"so unzip it inside `~/.claude/skills/`: `SKILL.md` then ends up at `~/.claude/skills/demo/SKILL.md`. " +
				"Unzipping into `~/.claude/skills/demo/` itself nests it as `~/.claude/skills/demo/demo/`",
			"place its contents at that path"},
		{"a wrapped zip whose path does not end in its folder", "~/.agent/skills/", &top,
			"Unzip the package and place its contents at that path.", "unzip it inside"},
		{"a zip whose root is the Skill root", "~/.agent/skills/<name>/", nil,
			"Unzip the package and place its contents at that path.", "unzip it inside"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := Profile{DisplayName: "Agent", Version: "1"}
			p.Install.Locations = []InstallLocation{{Scope: "user", Path: tc.path, Description: "per user"}}
			p.Install.TopLevelDir = tc.top
			where, _ := installSection(renderInstall(p, "demo", nil, mcpNeeds{}), "Where it goes")
			if !strings.Contains(where, tc.holds) || strings.Contains(where, tc.notHolds) {
				t.Errorf("the guide should say %q and not %q:\n%s", tc.holds, tc.notHolds, where)
			}
		})
	}
}
