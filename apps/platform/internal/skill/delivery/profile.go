package packaging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

var TargetIDs = []string{"standard", "claude-code", "claude-agent-sdk"}

var reservedFrontmatterKeys = map[string]bool{
	"name": true, "description": true, "license": true,
	"allowed-tools": true, "allowed_tools": true,
}

type InstallLocation struct {
	Scope       string `json:"scope"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

type EnvVar struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
	Example     string `json:"example,omitempty"`
}

type Profile struct {
	SchemaVersion string `json:"schema_version"`
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Version       string `json:"version"`
	DisplayName   string `json:"display_name"`
	SupportStatus string `json:"support_status"`
	Install       struct {
		Locations []InstallLocation `json:"locations"`

		TopLevelDir *string `json:"top_level_dir"`
	} `json:"install"`
	FrontmatterAdditions map[string]any `json:"frontmatter_additions"`
	EnvVars              []EnvVar       `json:"env_vars"`
	VerificationPrompt   string         `json:"verification_prompt,omitempty"`
	VerificationSteps    []string       `json:"verification_steps,omitempty"`
	Snippet              string         `json:"snippet,omitempty"`
	KnownLimitations     []string       `json:"known_limitations"`
	Notes                []string       `json:"notes,omitempty"`
}

type Profiles map[string]Profile

func (p Profiles) Ordered() []Profile {
	out := make([]Profile, 0, len(p))
	for _, id := range TargetIDs {
		if prof, ok := p[id]; ok {
			out = append(out, prof)
		}
	}
	return out
}

func LoadProfiles(dir string) (Profiles, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return Profiles{}, nil
		}
		return nil, fmt.Errorf("packaging profiles: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	out := Profiles{}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("packaging profile %s: %w", name, err)
		}
		var p Profile
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("packaging profile %s: %w", name, err)
		}
		if err := p.check(); err != nil {
			return nil, fmt.Errorf("packaging profile %s: %w", name, err)
		}
		if _, dup := out[p.ID]; dup {
			return nil, fmt.Errorf("packaging profile %s: %s is already configured", name, p.ID)
		}
		out[p.ID] = p
	}
	return out, nil
}

func (p Profile) check() error {
	if !isTargetID(p.ID) {
		return fmt.Errorf("id %q is not one of the packaging targets", p.ID)
	}
	if p.Version == "" {
		return fmt.Errorf("version is required; it is recorded in every package this target produces")
	}

	if p.ID == "standard" && len(p.FrontmatterAdditions) > 0 {
		return fmt.Errorf("the standard package may not add frontmatter: its SKILL.md is copied byte for byte")
	}
	for key := range p.FrontmatterAdditions {
		if reservedFrontmatterKeys[key] {
			return fmt.Errorf("frontmatter_additions may not name %q: a Profile adds fields, it does not rewrite them", key)
		}
	}
	return nil
}

func isTargetID(id string) bool {
	for _, t := range TargetIDs {
		if t == id {
			return true
		}
	}
	return false
}

func (p Profile) topLevelDir(skillName string) string {
	if p.Install.TopLevelDir == nil {
		return ""
	}
	dir := strings.TrimSuffix(substituteName(*p.Install.TopLevelDir, skillName), "/")
	if dir == "" {
		return ""
	}
	return dir + "/"
}

func substituteName(s, skillName string) string {
	return strings.ReplaceAll(s, "<name>", skillName)
}

func renderInstall(p Profile, skillName string, deps []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Installing %s for %s\n\n", skillName, p.DisplayName)

	if p.SupportStatus == "verified" {
		fmt.Fprintf(&b, "**Support status: verified.** Skill Hub has installed and run a package "+
			"through this target. That is not a promise about this Skill — see the compatibility "+
			"block in `skillhub-manifest.json` for what was actually measured for this version.\n\n")
	} else {
		fmt.Fprintf(&b, "**Support status: unverified.** Skill Hub has not installed or run a package "+
			"through this target, and nothing here promises it will work.\n\n")
	}
	fmt.Fprintf(&b, "- Target: %s (profile version %s)\n- Packaged by: Skill Hub packager %s\n\n",
		p.DisplayName, p.Version, PackagerVersion)

	fmt.Fprintf(&b, "## What was checked\n\n"+
		"Validated against the Agent Skills specification, revision `%s` "+
		"(https://agentskills.io/specification). Checked: `SKILL.md` present at the package root; "+
		"frontmatter parses as YAML; `name` present, at most 64 characters, lowercase alphanumerics "+
		"and single hyphens, not starting or ending with one; `description` present, non-blank and at "+
		"most 1024 characters; `compatibility`, where present, at most 500 characters; every file "+
		"`SKILL.md` points at is in this package.\n\n"+
		"**Not checked and not claimed:** that your Agent loads this Skill, that its scripts run, or "+
		"that it does what it says. Those are the other two layers of ADR-012, and the "+
		"`compatibility` block of `skillhub-manifest.json` records what was actually measured for "+
		"this version - which for most versions is nothing.\n\n"+
		"The specification defines exactly six frontmatter fields (`name`, `description`, `license`, "+
		"`compatibility`, `metadata`, `allowed-tools`), and anything else is a **hard error** here, "+
		"as it is in the specification's own reference validator and in some clients' upload paths. "+
		"No package you download from Skill Hub carries one.\n\n",
		skillpkg.SpecRevision)

	fmt.Fprintf(&b, "**Re-importing this into Skill Hub:** Skill Hub accepts packages up to %s on "+
		"import. The packager adds files your source did not have, so a package it produces can be "+
		"larger than that — if this one is, Skill Hub will not take it back. That is Skill Hub's own "+
		"ceiling, not the specification's, and it changes nothing about installing the package.\n\n",
		skillpkg.HumanMB(skillpkg.MaxZipBytes))

	b.WriteString("## Where it goes\n\n")
	if len(p.Install.Locations) == 0 {
		b.WriteString("This is the standard Agent Skills package. It names no install location, " +
			"because Skill Hub does not claim to know where your Agent keeps its Skills. " +
			"Consult your Agent's documentation for the Agent Skills directory.\n\n")
	}
	for _, loc := range p.Install.Locations {
		fmt.Fprintf(&b, "- `%s` (%s) — %s\n", loc.Path, loc.Scope, loc.Description)
	}
	if len(p.Install.Locations) > 0 {
		b.WriteString("\nUnzip the package and place its contents at that path.\n\n")
	}

	if len(deps) > 0 {

		b.WriteString("## Dependencies\n\n")
		for _, d := range deps {
			fmt.Fprintf(&b, "- %s\n", d)
		}
		b.WriteString("\nSkill Hub does not install these for you, and it does not execute anything " +
			"in this package. Read them before you run it.\n\n")
	}

	if len(p.EnvVars) > 0 {
		b.WriteString("## Environment variables\n\n")
		for _, v := range p.EnvVars {
			req := "optional"
			if v.Required {
				req = "required"
			}
			fmt.Fprintf(&b, "- `%s` (%s) — %s", v.Name, req, v.Description)
			if v.Example != "" {
				fmt.Fprintf(&b, " Example: `%s`", v.Example)
			}
			b.WriteString("\n")
		}
		b.WriteString("\nSet these in your own environment. No package Skill Hub produces contains a key.\n\n")
	}

	if p.Snippet != "" {
		b.WriteString("## Minimal working example\n\n```\n" + p.Snippet + "\n```\n\n")
	}

	b.WriteString("## Check that it worked\n\n")
	if p.VerificationPrompt != "" {
		fmt.Fprintf(&b, "Run this prompt against your Agent:\n\n> %s\n\n", p.VerificationPrompt)
	}
	for _, step := range p.VerificationSteps {
		fmt.Fprintf(&b, "1. %s\n", step)
	}
	if len(p.VerificationSteps) > 0 {
		b.WriteString("\n")
	}

	if len(p.KnownLimitations) > 0 {
		b.WriteString("## Known limitations\n\n")
		for _, l := range p.KnownLimitations {
			fmt.Fprintf(&b, "- %s\n", l)
		}
		b.WriteString("\n")
	}
	for _, n := range p.Notes {
		fmt.Fprintf(&b, "> %s\n", n)
	}

	return substituteName(b.String(), skillName)
}
