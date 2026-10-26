package creation

import (
	_ "embed"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

const (
	checkScriptPath = "scripts/check_output.py"
	inputScriptPath = "scripts/check_input.py"
	inputSchemaPath = "scripts/input_schema.json"
)

//go:embed shipped/check_output.py
var checkScriptSource string

//go:embed shipped/check_input.py
var inputScriptSource string

const inputCheckSection = "## Input check (first step)\n\n" +
	"Before any other step, write every value the message gives into `input.json` as " +
	"`{\"records\": [{<field name>: <the value exactly as written>}, ...]}`, one record " +
	"per item, with the field names of `scripts/input_schema.json`, then run\n" +
	"`python scripts/check_input.py input.json`\n" +
	"from the directory holding this SKILL.md, adding `--today YYYY-MM-DD` when the message " +
	"states today's date. Every later step and script takes its values only from " +
	"`checked.json`. Copy every line it prints that starts with 假設：, 無法採用： or 未提供： " +
	"into the answer word for word; compute nothing from a value it refused, and still give " +
	"every result the other values settle.\n"

const scriptsSection = "## Scripts\n\n" +
	"Paths below are relative to the directory holding this SKILL.md: run each script from " +
	"there (or by its full path) with the inputs taken from the message, and present what " +
	"it prints; never work its result out yourself. `python <script> --help` lists its " +
	"arguments.\n"

const referencesSection = "## References\n\n" +
	"Read the file below, relative to the directory holding this SKILL.md, when its topic " +
	"comes up.\n"

const outputCheckSection = "## Output check\n\n" +
	"Before answering, write the answer to `answer.txt`, run\n" +
	"`python scripts/%s answer.txt`\n" +
	"from the directory holding this SKILL.md, and revise the file until it prints OK, " +
	"first by shorter wording and the short forms of dates and times (11/14 14:30); then " +
	"answer with the file's content and nothing else. When everything cannot fit, keep the " +
	"limit and add one line after the answer saying what was left out; never answer with a " +
	"note that it cannot fit instead of the text.\n"

var (
	shippedScriptPath    = regexp.MustCompile(`^scripts/[^/]+\.py$`)
	shippedReferencePath = regexp.MustCompile(`^references/[^/]+\.md$`)
	clauseBreak          = regexp.MustCompile(`[。；;，,\n]`)
	perUnit              = regexp.MustCompile(`(?i)每|各|\b(?:each|per|every)\b`)
)

type outputCap struct {
	flag    string
	pattern *regexp.Regexp
}

const (
	capSpace = `[\s\x{1c}-\x{1f}\x{85}\p{Z}]*`
	capCount = `([0-9０-９]+)`
	capBound = `(?:不超過|不得超過|最多|至多|no more than|at most|within)`
)

func capPattern(zhUnit, enUnit string) *regexp.Regexp {
	return regexp.MustCompile(capBound + capSpace + capCount + capSpace + zhUnit +
		`|` + capCount + capSpace + zhUnit + `(?:以內|內)` +
		`|(?:no more than|at most|within)` + capSpace + capCount + capSpace + enUnit)
}

var outputCaps = []outputCap{
	{"--max-sentences", regexp.MustCompile(capBound + capSpace + capCount + capSpace + `句` +
		`|` + capCount + capSpace + `句(?:話)?(?:以內|內)` +
		`|(?:no more than|at most|within)` + capSpace + capCount + capSpace + `sentences?`)},
	{"--max-chars", capPattern(`(?:個字|字)`, `(?:characters|chars)`)},
	{"--max-items", capPattern(`(?:項|條|點)`, `(?:items|bullets|points)`)},
}

func assembleDraft(d GeneratedSkill, brief string, criteria []string) GeneratedSkill {
	d = supplyInputCheck(d)
	d = bindOutputCheck(d, brief, criteria)
	d = supplyCheckScript(d)
	return bindShippedFiles(d)
}

func withPlatformFile(files []GeneratedFile, path, content string) []GeneratedFile {
	kept := slices.DeleteFunc(slices.Clone(files), func(f GeneratedFile) bool { return f.Path == path })
	return append(kept, GeneratedFile{Path: path, Content: content})
}

func supplyCheckScript(d GeneratedSkill) GeneratedSkill {
	if !strings.Contains(d.Body, checkScriptPath) {
		return d
	}
	d.Files = withPlatformFile(d.Files, checkScriptPath, checkScriptSource)
	return d
}

func supplyInputCheck(d GeneratedSkill) GeneratedSkill {
	if !slices.ContainsFunc(d.Files, func(f GeneratedFile) bool { return f.Path == inputSchemaPath }) {
		return d
	}
	d.Files = withPlatformFile(d.Files, inputScriptPath, inputScriptSource)
	if strings.Contains(d.Body, inputScriptPath) {
		return d
	}
	return appendSection(d, inputCheckSection)
}

func appendSection(d GeneratedSkill, section string) GeneratedSkill {
	d.Body = strings.TrimRightFunc(d.Body, unicode.IsSpace) + "\n\n" + section
	return d
}

func bindShippedFiles(d GeneratedSkill) GeneratedSkill {
	var scripts, references []string
	for _, f := range d.Files {
		switch {
		case strings.Contains(d.Body, f.Path):
		case shippedScriptPath.MatchString(f.Path):
			scripts = append(scripts, f.Path)
		case shippedReferencePath.MatchString(f.Path):
			references = append(references, f.Path)
		}
	}
	if len(scripts) > 0 {
		section := scriptsSection
		for _, p := range scripts {
			section += "\n- `python " + p + "`"
		}
		d = appendSection(d, section+"\n")
	}
	if len(references) > 0 {
		section := referencesSection
		for _, p := range references {
			section += fmt.Sprintf("\n- [%s](%s)", p, p)
		}
		d = appendSection(d, section+"\n")
	}
	return d
}

func bindOutputCheck(d GeneratedSkill, brief string, criteria []string) GeneratedSkill {
	if strings.Contains(d.Body, checkScriptPath) {
		return d
	}
	flags := outputCapFlags(strings.Join(append([]string{brief}, criteria...), "\n"))
	if len(flags) == 0 {
		return d
	}
	command := strings.TrimPrefix(checkScriptPath, "scripts/") + " " + strings.Join(flags, " ")
	return appendSection(d, fmt.Sprintf(outputCheckSection, command))
}

func outputCapFlags(text string) []string {
	var flags []string
	for _, c := range outputCaps {
		lowest := -1
		for _, m := range c.pattern.FindAllStringSubmatchIndex(text, -1) {
			if capsOneUnit(text, m[0], m[1]) {
				continue
			}
			for g := 2; g < len(m); g += 2 {
				if m[g] < 0 {
					continue
				}
				if n := capNumber(text[m[g]:m[g+1]]); lowest < 0 || n < lowest {
					lowest = n
				}
			}
		}
		if lowest >= 0 {
			flags = append(flags, fmt.Sprintf("%s %d", c.flag, lowest))
		}
	}
	return flags
}

func capsOneUnit(text string, start, end int) bool {
	clauseStart := 0
	for _, b := range clauseBreak.FindAllStringIndex(text[:start], -1) {
		clauseStart = b[1]
	}
	clauseEnd := len(text)
	if next := clauseBreak.FindStringIndex(text[end:]); next != nil {
		clauseEnd = end + next[0]
	}
	return perUnit.MatchString(text[clauseStart:clauseEnd])
}

func capNumber(digits string) int {
	n := 0
	for _, r := range digits {
		if r >= '０' {
			r -= '０' - '0'
		}
		n = n*10 + int(r-'0')
	}
	return n
}
