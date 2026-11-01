package skillpkg

import (
	"strings"
	"testing"
)

const layoutFrontmatter = "---\nname: s\ndescription: d\n---\n"

func fillerLines(n int) string {
	return strings.Repeat("line\n", n)
}

func layoutFindings(r Report) map[string][]Finding {
	out := map[string][]Finding{}
	for _, f := range r.Findings {
		switch f.Code {
		case CodeSkillBodyTooLong, CodeNestedReference, CodeReferenceWithoutContents:
			out[f.Code] = append(out[f.Code], f)
		}
	}
	return out
}

func TestTheSkillBodyMayRunToFiveHundredLinesAndIsFlaggedPastThat(t *testing.T) {
	for name, c := range map[string]struct {
		bodyLines int
		want      bool
	}{
		"exactly 500 lines": {500, false},
		"501 lines":         {501, true},
	} {
		t.Run(name, func(t *testing.T) {
			r := Validate(pkg(layoutFrontmatter+fillerLines(c.bodyLines), nil))
			got := layoutFindings(r)[CodeSkillBodyTooLong]
			if (len(got) == 1) != c.want {
				t.Fatalf("findings = %+v, want flagged = %v", got, c.want)
			}
			if c.want && (got[0].Severity != SeverityWarning || got[0].Path != "SKILL.md" || !strings.Contains(got[0].Message, "501")) {
				t.Fatalf("finding = %+v, want a warning on SKILL.md naming 501 lines", got[0])
			}
			if r.Blocked {
				t.Fatal("a long body blocked an imported package; on import it is only a warning")
			}
		})
	}
}

func TestALongReferenceWithoutAContentsListIsFlagged(t *testing.T) {
	skill := layoutFrontmatter + "Read [rules](references/rules.md) for the tiers.\n"
	for name, c := range map[string]struct {
		reference string
		want      bool
	}{
		"100 lines and no contents":                     {"# Rules\n" + fillerLines(99), false},
		"101 lines and no contents":                     {"# Rules\n" + fillerLines(100), true},
		"101 lines under a Contents heading":            {"# Rules\n## Contents\n- Tiers\n" + fillerLines(98), false},
		"101 lines under a Chinese contents heading":    {"# 規則\n## 目錄\n- 級距\n" + fillerLines(98), false},
		"101 lines opening with a list of anchors":      {"# Rules\n- [Tiers](#tiers)\n" + fillerLines(99), false},
		"a contents heading on line 20":                 {fillerLines(19) + "## Contents\n" + fillerLines(81), false},
		"a contents heading on line 21 is too far down": {fillerLines(20) + "## Contents\n" + fillerLines(80), true},
		"a heading that only mentions contents":         {"# Rules\n## Contents of the tier table\n" + fillerLines(99), true},
	} {
		t.Run(name, func(t *testing.T) {
			r := Validate(pkg(skill, map[string]string{"references/rules.md": c.reference}))
			got := layoutFindings(r)[CodeReferenceWithoutContents]
			if (len(got) == 1) != c.want {
				t.Fatalf("findings = %+v, want flagged = %v", got, c.want)
			}
			if c.want && (got[0].Path != "references/rules.md" || got[0].Severity != SeverityWarning) {
				t.Fatalf("finding = %+v, want a warning on references/rules.md", got[0])
			}
		})
	}
}

func TestALongFileTheBodyDoesNotLinkIsNotAskedForContents(t *testing.T) {
	r := Validate(pkg(layoutFrontmatter+"No links.\n", map[string]string{"references/notes.md": fillerLines(300)}))
	if got := layoutFindings(r)[CodeReferenceWithoutContents]; len(got) != 0 {
		t.Fatalf("an unlinked file was checked as a reference: %+v", got)
	}
}

func TestAReferenceReachableOnlyThroughAnotherReferenceIsFlagged(t *testing.T) {
	for name, c := range map[string]struct {
		skill, advanced string
		extra           map[string]string
		wantNested      []string
	}{
		"a relative link from one reference to the next": {
			skill:      "See [advanced](references/advanced.md).\n",
			advanced:   "See [details](details.md).\n",
			extra:      map[string]string{"references/details.md": "facts\n"},
			wantNested: []string{"references/details.md"},
		},
		"a bare package path from one reference to the next": {
			skill:      "See references/advanced.md.\n",
			advanced:   "Then read references/details.md.\n",
			extra:      map[string]string{"references/details.md": "facts\n"},
			wantNested: []string{"references/details.md"},
		},
		"the body links both files itself": {
			skill:    "See [advanced](references/advanced.md) and [details](references/details.md).\n",
			advanced: "See [details](details.md).\n",
			extra:    map[string]string{"references/details.md": "facts\n"},
		},
		"a reference that links back to SKILL.md and to itself": {
			skill:    "See [advanced](references/advanced.md).\n",
			advanced: "Back to [SKILL.md](../SKILL.md), [top](advanced.md).\n",
		},
		"a reference that names a script": {
			skill:    "See [advanced](references/advanced.md).\n",
			advanced: "Run scripts/tier.py.\n",
			extra:    map[string]string{"scripts/tier.py": "print(1)\n"},
		},
		"a reference that links a file the package does not ship": {
			skill:    "See [advanced](references/advanced.md).\n",
			advanced: "See [gone](gone.md).\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			files := map[string]string{"references/advanced.md": c.advanced}
			for p, content := range c.extra {
				files[p] = content
			}
			r := Validate(pkg(layoutFrontmatter+c.skill, files))
			var got []string
			for _, f := range layoutFindings(r)[CodeNestedReference] {
				got = append(got, f.Path)
				if f.Severity != SeverityWarning || !strings.Contains(f.Message, "references/advanced.md") {
					t.Errorf("finding = %+v, want a warning naming the file that links it", f)
				}
			}
			if strings.Join(got, ",") != strings.Join(c.wantNested, ",") {
				t.Fatalf("nested = %v, want %v", got, c.wantNested)
			}
			if r.Blocked {
				t.Fatal("a nested reference blocked an imported package; on import it is only a warning")
			}
		})
	}
}

func TestANestedReferenceLinkedFromTwoReferencesIsFlaggedOnce(t *testing.T) {
	r := Validate(pkg(layoutFrontmatter+"See [a](references/a.md) and [b](references/b.md).\n", map[string]string{
		"references/a.md": "See [c](c.md).\n",
		"references/b.md": "See [c](c.md).\n",
		"references/c.md": "facts\n",
	}))
	if got := layoutFindings(r)[CodeNestedReference]; len(got) != 1 || got[0].Path != "references/c.md" {
		t.Fatalf("nested = %+v, want one finding on references/c.md", got)
	}
}

func findingsWithCode(r Report, code string) []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Code == code {
			out = append(out, f)
		}
	}
	return out
}

func TestAScriptOrReferenceNothingMentionsIsFlagged(t *testing.T) {
	for name, c := range map[string]struct {
		body  string
		files map[string]string
		want  []string
	}{
		"a script named by its path": {
			body: "Run `python scripts/tier.py`.\n", files: map[string]string{"scripts/tier.py": "x = 1\n"},
		},
		"a script named by its file name only": {
			body: "Run **tier.py** with the totals.\n", files: map[string]string{"scripts/tier.py": "x = 1\n"},
		},
		"a script run as a module": {
			body: "Run `python -m scripts.tier`.\n", files: map[string]string{"scripts/tier.py": "x = 1\n"},
		},
		"a helper another script imports": {
			body: "Run scripts/tier.py.\n",
			files: map[string]string{
				"scripts/tier.py":    "from scripts.helpers import rate\n",
				"scripts/helpers.py": "def rate(): pass\n",
			},
		},
		"a package marker": {
			body:  "Run scripts/tier.py.\n",
			files: map[string]string{"scripts/tier.py": "x = 1\n", "scripts/__init__.py": ""},
		},
		"a reference named only inside a linked reference": {
			body: "See [guide](references/guide.md).\n",
			files: map[string]string{
				"references/guide.md": "Open tiers.md for the table.\n",
				"references/tiers.md": "table\n",
			},
		},
		"a data file beside the scripts": {
			body:  "Run scripts/tier.py.\n",
			files: map[string]string{"scripts/tier.py": "x = 1\n", "scripts/input_schema.json": "{}", "scripts/rates.csv": "a\n"},
		},
		"a script outside scripts and references": {
			body: "Nothing here.\n", files: map[string]string{"assets/build.py": "x = 1\n"},
		},
		"a script nothing mentions": {
			body:  "Run scripts/tier.py.\n",
			files: map[string]string{"scripts/tier.py": "x = 1\n", "scripts/cleanup.sh": "rm -f x\n"},
			want:  []string{"scripts/cleanup.sh"},
		},
		"a reference nothing mentions": {
			body:  "No links.\n",
			files: map[string]string{"references/tiers.md": "table\n"},
			want:  []string{"references/tiers.md"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := Validate(pkg(layoutFrontmatter+c.body, c.files))
			var got []string
			for _, f := range findingsWithCode(r, CodeUnmentionedFile) {
				got = append(got, f.Path)
				if f.Severity != SeverityWarning {
					t.Errorf("finding = %+v, want a warning", f)
				}
			}
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Fatalf("unmentioned = %v, want %v", got, c.want)
			}
		})
	}
}

func TestACreationRuleNeverPromotesAnUnmentionedFile(t *testing.T) {
	r := Validate(pkg(layoutFrontmatter+"No links.\n", map[string]string{"references/tiers.md": "table\n"}))
	r.RequireAuthoringRules()
	got := findingsWithCode(r, CodeUnmentionedFile)
	if len(got) != 1 || got[0].Severity != SeverityWarning || r.Blocked {
		t.Fatalf("findings = %+v blocked = %v, want the unmentioned file to stay a warning", got, r.Blocked)
	}
}

func TestANameHoldingAReservedWordIsFlagged(t *testing.T) {
	for name, want := range map[string]bool{
		"claude-invoice":     true,
		"pdf-for-anthropic":  true,
		"invoiceclaudette":   true,
		"claud-invoice":      false,
		"anthropology-notes": false,
	} {
		t.Run(name, func(t *testing.T) {
			r := Validate(pkg("---\nname: "+name+"\ndescription: d\n---\nbody\n", nil))
			got := findingsWithCode(r, CodeNameReservedWord)
			if (len(got) == 1) != want {
				t.Fatalf("findings = %+v, want flagged = %v", got, want)
			}
			if want && (got[0].Severity != SeverityWarning || r.Blocked) {
				t.Fatalf("finding = %+v blocked = %v, want an unblocking warning on import", got[0], r.Blocked)
			}
		})
	}
}

func TestADescriptionHoldingAnXMLTagIsFlagged(t *testing.T) {
	for name, c := range map[string]struct {
		description, tag string
	}{
		"a paired tag":             {"Extracts <b>tables</b> from PDFs.", "<b>"},
		"a self-closing tag":       {"Fills forms.<br/> Use for PDFs.", "<br/>"},
		"a tag with an attribute":  {`Reads <file path="x"> inputs.`, `<file path=\"x\">`},
		"a comparison in prose":    {"Use when a < b and c > d.", ""},
		"an arrow between steps":   {"Parse -> validate -> save.", ""},
		"a number in angle quotes": {"Caps at <5> retries.", ""},
	} {
		t.Run(name, func(t *testing.T) {
			r := Validate(pkg("---\nname: s\ndescription: '"+c.description+"'\n---\nbody\n", nil))
			got := findingsWithCode(r, CodeDescriptionXMLTag)
			if c.tag == "" {
				if len(got) != 0 {
					t.Fatalf("prose was read as a tag: %+v", got)
				}
				return
			}
			if len(got) != 1 || !strings.Contains(got[0].Message, c.tag) || got[0].Severity != SeverityWarning {
				t.Fatalf("findings = %+v, want one warning naming %s", got, c.tag)
			}
		})
	}
}

func TestABackslashPathIntoAShippedFolderIsFlaggedWithItsForwardSlashForm(t *testing.T) {
	extra := map[string]string{"scripts/run.py": "print(1)\n"}
	for name, c := range map[string]struct {
		body string
		want []string
	}{
		"a backslash path into a shipped folder": {`Run scripts\run.py now.` + "\n", []string{`"scripts/run.py"`}},
		"the same path written twice":            {"Run `scripts\\run.py`, then scripts\\run.py again.\n", []string{`"scripts/run.py"`}},
		"a folder the package does not ship":     {`See docs\guide.md.` + "\n", nil},
		"a regular expression in prose":          {`Match \d+\.\d+ in the text.` + "\n", nil},
		"a forward-slash path":                   {"Run scripts/run.py now.\n", nil},
	} {
		t.Run(name, func(t *testing.T) {
			r := Validate(pkg(layoutFrontmatter+c.body, extra))
			got := findingsWithCode(r, CodeBackslashPath)
			if len(got) != len(c.want) {
				t.Fatalf("findings = %+v, want %d", got, len(c.want))
			}
			for i, f := range got {
				if !strings.Contains(f.Message, c.want[i]) || f.Severity != SeverityWarning {
					t.Fatalf("finding = %+v, want a warning suggesting %s", f, c.want[i])
				}
			}
		})
	}
}
