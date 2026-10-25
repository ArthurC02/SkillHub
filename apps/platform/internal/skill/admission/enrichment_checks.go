package ingest

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

type EnrichmentFinding struct {
	Rule  string `json:"rule"`
	Field string `json:"field"`
	Token string `json:"token,omitempty"`
}

const (
	ruleRuntimeNotInLimitations = "runtime_not_in_limitations"
	ruleUnsupportedAppraisal    = "unsupported_appraisal"
	ruleNonEnglishInEnExample   = "non_english_in_en_example"
)

type runtimeSignal struct {
	runtime  string
	evidence []*regexp.Regexp
	names    []string
}

var runtimeSignals = []runtimeSignal{
	{
		runtime: "python",
		evidence: compileAll(`(?im)^\s*(?:import|from)\s+\w`, `(?i)#!/usr/bin/env python`,
			`(?i)\bpython3?\b`, `(?i)\.py\b`),
		names: []string{"python", "py3"},
	},
	{
		runtime: "node",
		evidence: compileAll(`(?i)#!/usr/bin/env node`, `(?i)\bnpm\s+(?:i|install|run)\b`,
			`(?i)\bnode\s+\S+\.m?js\b`, `(?i)\.mjs\b`),
		names: []string{"node", "node.js", "nodejs", "npm"},
	},
	{
		runtime:  "pandoc",
		evidence: compileAll(`(?i)\bpandoc\b`),
		names:    []string{"pandoc"},
	},
}

type appraisal struct {
	token     string
	spellings []string
}

var appraisals = []appraisal{
	{"clear", []string{"clear", "清晰", "清楚"}},
	{"concise", []string{"concise", "簡潔", "精簡"}},
	{"polished", []string{"polished", "精緻", "細緻"}},
	{"accurate", []string{"accurate", "accuracy", "準確", "精確"}},
	{"professional", []string{"professional", "專業"}},
	{"well-structured", []string{"well-structured", "well structured", "結構良好", "結構清晰"}},
}

type appraisalSpelling struct{ spelling, token string }

// Longest spelling first: a matched span is consumed before shorter spellings
// look, so 結構清晰 is one appraisal rather than two overlapping ones.
var appraisalSpellings = longestFirst(appraisals)

var nonEnglishScript = regexp.MustCompile(`[\x{3040}-\x{30FF}\x{3400}-\x{4DBF}\x{4E00}-\x{9FFF}\x{F900}-\x{FAFF}]`)

type enrichmentDocument struct {
	skillMD        string
	fileTree       []string
	summary        string
	limitations    []string
	taskExamplesEn []string
	tags           []string
}

func checkEnrichment(doc enrichmentDocument) []EnrichmentFinding {
	source := doc.skillMD + "\n" + strings.Join(doc.fileTree, "\n")
	fields := [][2]string{{"summary", doc.summary}}
	for i, text := range doc.limitations {
		fields = append(fields, [2]string{fmt.Sprintf("limitations[%d]", i), text})
	}
	for i, text := range doc.tags {
		fields = append(fields, [2]string{fmt.Sprintf("tags[%d]", i), text})
	}
	findings := []EnrichmentFinding{}
	findings = append(findings, runtimeFindings(source, doc.limitations)...)
	findings = append(findings, appraisalFindings(source, fields)...)
	return append(findings, nonEnglishExampleFindings(doc.taskExamplesEn)...)
}

func runtimeFindings(source string, limitations []string) []EnrichmentFinding {
	stated := strings.ToLower(strings.Join(limitations, "\n"))
	var out []EnrichmentFinding
	for _, signal := range runtimeSignals {
		needed := slices.ContainsFunc(signal.evidence, func(re *regexp.Regexp) bool { return re.MatchString(source) })
		mentioned := slices.ContainsFunc(signal.names, func(name string) bool { return strings.Contains(stated, name) })
		if needed && !mentioned {
			out = append(out, EnrichmentFinding{Rule: ruleRuntimeNotInLimitations, Field: "limitations", Token: signal.runtime})
		}
	}
	return out
}

func appraisalFindings(source string, fields [][2]string) []EnrichmentFinding {
	lowSource := strings.ToLower(source)
	seen := map[[2]string]bool{}
	var out []EnrichmentFinding
	for _, field := range fields {
		low := strings.ToLower(field[1])
		for _, s := range appraisalSpellings {
			if !strings.Contains(low, s.spelling) {
				continue
			}
			low = strings.ReplaceAll(low, s.spelling, " ")
			key := [2]string{field[0], s.token}
			if sourceClaims(lowSource, s.token) || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, EnrichmentFinding{Rule: ruleUnsupportedAppraisal, Field: field[0], Token: s.token})
		}
	}
	return out
}

func sourceClaims(lowSource, token string) bool {
	for _, a := range appraisals {
		if a.token == token {
			return slices.ContainsFunc(a.spellings, func(s string) bool { return strings.Contains(lowSource, s) })
		}
	}
	return false
}

func nonEnglishExampleFindings(examples []string) []EnrichmentFinding {
	var out []EnrichmentFinding
	for i, text := range examples {
		if nonEnglishScript.MatchString(text) {
			out = append(out, EnrichmentFinding{Rule: ruleNonEnglishInEnExample, Field: fmt.Sprintf("task_examples[%d].en", i)})
		}
	}
	return out
}

func compileAll(patterns ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		out = append(out, regexp.MustCompile(p))
	}
	return out
}

func longestFirst(all []appraisal) []appraisalSpelling {
	var out []appraisalSpelling
	for _, a := range all {
		for _, s := range a.spellings {
			out = append(out, appraisalSpelling{spelling: strings.ToLower(s), token: a.token})
		}
	}
	slices.SortStableFunc(out, func(a, b appraisalSpelling) int {
		return utf8.RuneCountInString(b.spelling) - utf8.RuneCountInString(a.spelling)
	})
	return out
}

func documentOf(p preparedPackage, resp *SkillEnrichment) enrichmentDocument {
	doc := enrichmentDocument{
		skillMD: p.skillMD, fileTree: p.fileTree, summary: resp.Summary, limitations: resp.Limitations,
	}
	for _, example := range resp.TaskExamples {
		doc.taskExamplesEn = append(doc.taskExamplesEn, example.En)
	}
	for _, bucket := range [][]string{resp.Tags.Inputs, resp.Tags.Outputs, resp.Tags.Tools, resp.Tags.Dependencies} {
		doc.tags = append(doc.tags, bucket...)
	}
	return doc
}

func marshalFindings(findings []EnrichmentFinding) []byte {
	raw, err := json.Marshal(findings)
	if err != nil {
		return nil
	}
	return raw
}
