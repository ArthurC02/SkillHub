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
