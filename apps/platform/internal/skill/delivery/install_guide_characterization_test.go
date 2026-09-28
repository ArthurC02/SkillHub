package packaging

import (
	"strings"
	"testing"
)

func TestTheInstallGuideWritesEveryProfileSectionInOrder(t *testing.T) {
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

	got := renderInstall(p, "demo", []string{"requests"})

	want := "## Where it goes\n\n" +
		"- `~/.agent/skills/demo` (user) — per user\n" +
		"\nUnzip the package and place its contents at that path.\n\n" +
		"## Dependencies\n\n- requests\n" +
		"\nSkill Hub does not install these for you, and it does not execute anything " +
		"in this package. Read them before you run it.\n\n" +
		"## Environment variables\n\n" +
		"- `API_KEY` (required) — The key. Example: `abc`\n" +
		"- `MODE` (optional) — The mode.\n" +
		"\nSet these in your own environment. No package Skill Hub produces contains a key.\n\n" +
		"## Minimal working example\n\n```\nrun demo\n```\n\n" +
		"## Check that it worked\n\n" +
		"Run this prompt against your Agent:\n\n> use demo\n\n" +
		"1. look\n1. see\n\n" +
		"## Known limitations\n\n- slow\n\n" +
		"> note one\n> note two\n"
	if !strings.HasSuffix(got, want) || strings.Count(got, "## Where it goes") != 1 {
		t.Fatalf("install guide =\n%s\nwant it to end with\n%s", got, want)
	}
}

func TestAnInstallGuideWithoutLocationsSaysItNamesNone(t *testing.T) {
	got := renderInstall(Profile{DisplayName: "Any", Version: "1"}, "demo", nil)

	want := "## Where it goes\n\n" +
		"This is the standard Agent Skills package. It names no install location, " +
		"because Skill Hub does not claim to know where your Agent keeps its Skills. " +
		"Consult your Agent's documentation for the Agent Skills directory.\n\n" +
		"## Check that it worked\n\n"
	if !strings.HasSuffix(got, want) {
		t.Fatalf("install guide =\n%s\nwant it to end with\n%s", got, want)
	}
}
