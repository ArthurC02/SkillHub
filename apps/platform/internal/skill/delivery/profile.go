package packaging

// The read side of contracts/packaging/packaging-profile.schema.json: the
// versioned configuration half of a packaging target (ADR-012). The Adapter half
// is the code in this package; there is no plugin mechanism and none is wanted,
// because the MVP ships three built-in targets and an extension point for a
// second party who does not exist is scaffolding for later (m4/README §2.2).
//
// Data driven rather than compiled in. `support_status` changes when a target is
// measured and the install paths are reviewed text, so a copy of them in Go would
// be a second truth that nobody re-reviews — the same reason GET
// /packaging/targets is an endpoint and not a frontend constant. A deployment
// with no profile directory therefore has no targets, and says so (503); it does
// not fall back to a hard-coded set, because a hard-coded fallback is exactly the
// second truth this avoids.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

// TargetIDs is the value domain of public.yaml's PackagingTargetId, in the order
// the targets endpoint serves them: the standard package first, because PDM-008
// requires the product to say "one standard package plus two verified install
// profiles" and the standard one is the claim that carries.
var TargetIDs = []string{"standard", "claude-code", "claude-agent-sdk"}

// Reserved frontmatter keys a Profile may not write. ADR-012 forbids an Adapter
// silently changing a Skill's task intent or removing a safety constraint; the
// machine-checkable form is "may not name a field the spec already defines". The
// JSON Schema refuses these too — this is the floor under a hand-edited file
// that never went through the validator.
var reservedFrontmatterKeys = map[string]bool{
	"name": true, "description": true, "license": true,
	"allowed-tools": true, "allowed_tools": true,
}

// InstallLocation is one place an unpacked Skill goes (PACK-006).
type InstallLocation struct {
	Scope       string `json:"scope"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

// EnvVar documents a variable the target needs. `Example` is a placeholder and
// never a credential: it is rendered verbatim into INSTALL.md, which ships inside
// a package handed to users (iron rule 11).
type EnvVar struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
	Example     string `json:"example,omitempty"`
}

// Profile is one packaging target's configuration.
type Profile struct {
	SchemaVersion string `json:"schema_version"`
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Version       string `json:"version"`
	DisplayName   string `json:"display_name"`
	SupportStatus string `json:"support_status"`
	Install       struct {
		Locations []InstallLocation `json:"locations"`
		// TopLevelDir is the single directory inside the zip, or nil for a zip
		// whose root IS the Skill root. Nil for the standard package: that shape
		// is the exact inverse of the import path, which is what makes "unzip and
		// re-validate" well defined (PACK-009).
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

// Profiles is the loaded set, keyed by target id.
type Profiles map[string]Profile

// Ordered returns the profiles in TargetIDs order, skipping ones this deployment
// does not have.
func (p Profiles) Ordered() []Profile {
	out := make([]Profile, 0, len(p))
	for _, id := range TargetIDs {
		if prof, ok := p[id]; ok {
			out = append(out, prof)
		}
	}
	return out
}

// LoadProfiles reads every *.json in dir as a packaging profile. A missing
// directory is not an error the caller has to distinguish: it comes back as an
// empty set, and every packaging route then answers "not configured" rather than
// inventing a target.
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

// check refuses a configuration that would make the packager violate a rule the
// packager itself cannot see. It is not a re-implementation of the JSON Schema —
// only the three rules whose violation would be silent in the produced bytes.
func (p Profile) check() error {
	if !isTargetID(p.ID) {
		return fmt.Errorf("id %q is not one of the packaging targets", p.ID)
	}
	if p.Version == "" {
		return fmt.Errorf("version is required; it is recorded in every package this target produces")
	}
	// PDM-008: the standard package is the evidence that Skill Hub is not bound
	// to one Agent, and that evidence is "SKILL.md is byte for byte the source
	// version's". A frontmatter addition here would quietly retire the claim.
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

// topLevelDir is the zip prefix this profile writes, with <name> substituted, or
// "" for a zip whose root is the package root.
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

// renderInstall assembles INSTALL.md from the profile's reviewed fields and the
// package's own validation findings (02:PACK-002). Assembled, never generated: a
// model writing install paths produces instructions nobody verified, which is why
// llm-internal.yaml is untouched by M4 (contract-deltas §5).
func renderInstall(p Profile, skillName string, deps []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Installing %s for %s\n\n", skillName, p.DisplayName)

	// PACK-008 first, before any instruction: an unverified target must not be
	// read as a promise that the package works.
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

	// What "conforms" means, named.
	//
	// This section replaced a sentence that told every downloader the package was
	// "valid against the Agent Skills specification" at a time when this
	// repository contained no such document — no URL, no copy, no revision, no
	// ADR. Its only real content was "it passed our own parser", which is not
	// what a reader takes from it, and it shipped inside the artifact rather than
	// on a screen somebody could argue with (04 一致性 review, ADR-044).
	//
	// The specification is now pinned (contracts/spec/SOURCE.json). The claim
	// names its revision and lists what was checked, because a claim the reader
	// cannot re-check is the same defect written longer.
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

	// 03:PACK-012. Said here and not only in a comment, because it is a fact the
	// reader can walk into: the packager adds files the source did not have (this
	// file, the manifest, any test cases), so a package can come out larger than
	// the ceiling Skill Hub's own import accepts, and the round trip they would
	// reasonably assume works then does not.
	//
	// This document rather than the download preview: it is the only surface that
	// travels WITH the bytes, so it is still there when someone tries the
	// re-import weeks later, and it needs no contract change to carry.
	// Unconditional rather than "only when the package is actually over", because
	// the size is not known until after this file has been written into the zip —
	// and a sentence naming the number lets the reader check the file they are
	// holding, which a conditional one they never see does not.
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
		// Not "dependencies this package declares": the list also carries what the
		// scripts import WITHOUT declaring, and that entry is the one a reader most
		// needs. A heading that excluded it would make the section wrong about the
		// only line that is not obvious from the package itself.
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

	// PACK-007: at least one of these two is present, and the schema enforces it.
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
	// One substitution over the finished document rather than one per field. The
	// placeholder turns up in paths, descriptions, prompts, steps, limitations and
	// notes alike, and a per-field list is a list somebody adds a field to and
	// forgets — at which point a user is told to create a directory literally
	// called <name>.
	return substituteName(b.String(), skillName)
}
