package main

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

const adrDir = "docs/adr"

const adrIndex = adrDir + "/README.md"

var (
	adrNumberPattern = regexp.MustCompile(`ADR-\d{3}`)
	adrFilePattern   = regexp.MustCompile(`^ADR-(\d{3})-[a-z0-9-]+\.md$`)
	adrIndexLink     = regexp.MustCompile(`\]\(\./(ADR-\d{3}-[a-z0-9-]+\.md)\)`)
	adrAnchorLink    = regexp.MustCompile(`\]\(([^)\s#]*README\.md)#([^)\s]+)\)`)
)

var adrCitationExemptions = []struct {
	reason string
	covers func(relative string) bool
}{
	{"a third-party Skill corpus, kept as its authors wrote it", func(r string) bool { return strings.HasPrefix(r, "tools/goldenset/corpus") }},
	{"a model-written Skill body kept as evidence", func(r string) bool { return strings.HasSuffix(r, ".SKILL.md") }},
	{"recorded output of a milestone run (transcripts, probes, logs)", func(r string) bool {
		return strings.HasPrefix(r, "docs/plans/mvp/") && !strings.HasSuffix(r, ".md")
	}},
	{"recorded measurement results", func(r string) bool {
		return (strings.HasPrefix(r, "tools/eval-regression/") && strings.Contains(r, "results") && strings.HasSuffix(r, ".jsonl")) ||
			(strings.HasPrefix(r, "tools/goldenset/results") && strings.HasSuffix(r, ".txt"))
	}},
}

func adrCitationProblems(root string) []string {
	listed, err := exec.Command("git", "-C", root, "ls-files", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		return []string{fmt.Sprintf("adr-citations: git ls-files: %v", err)}
	}
	var files []string
	for _, relative := range strings.Split(strings.TrimSpace(string(listed)), "\n") {
		if relative == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); err == nil {
			files = append(files, relative)
		}
	}

	adrs := map[string]string{}
	var problems []string
	for _, relative := range files {
		if path.Dir(relative) != adrDir || relative == adrIndex {
			if strings.HasPrefix(relative, adrDir+"/") && relative != adrIndex {
				problems = append(problems, fmt.Sprintf(
					"adr-citations: %s is in %s but is neither an ADR nor the index; only ADR-NNN-<slug>.md and README.md live there",
					relative, adrDir))
			}
			continue
		}
		m := adrFilePattern.FindStringSubmatch(path.Base(relative))
		if m == nil {
			problems = append(problems, fmt.Sprintf(
				"adr-citations: %s is in %s but is not named ADR-NNN-<slug>.md", relative, adrDir))
			continue
		}
		adrs["ADR-"+m[1]] = relative
	}
	if len(adrs) == 0 {
		return append(problems, fmt.Sprintf("adr-citations: %s holds no ADR, so every rule below would pass on nothing", adrDir))
	}

	index, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(adrIndex)))
	if err != nil {
		return append(problems, fmt.Sprintf("adr-citations: %v", err))
	}
	problems = append(problems, adrIndexProblems(root, string(index), adrs)...)
	anchors := markdownAnchors(string(index))

	for _, relative := range files {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			continue
		}
		text := string(body)
		insideADRs := strings.HasPrefix(relative, adrDir+"/")
		exempt := adrCitationExempt(relative)
		for i, line := range strings.Split(text, "\n") {
			for _, number := range adrNumberPattern.FindAllString(line, -1) {
				switch {
				case insideADRs && adrs[number] == "":
					problems = append(problems, fmt.Sprintf(
						"adr-citations: %s:%d cites %s, and no such ADR exists in %s", relative, i+1, number, adrDir))
				case !insideADRs && !exempt:
					problems = append(problems, fmt.Sprintf(
						"adr-citations: %s:%d names %s. Only the ADRs and their index carry ADR numbers: write the rule "+
							"itself, and where the reason matters link the topic in %s (e.g. README.md#<topic>)",
						relative, i+1, number, adrIndex))
				}
			}
			if !strings.HasSuffix(relative, ".md") {
				continue
			}
			for _, m := range adrAnchorLink.FindAllStringSubmatch(line, -1) {
				target := path.Clean(path.Join(path.Dir(relative), m[1]))
				anchor, err := url.PathUnescape(m[2])
				if err != nil {
					anchor = m[2]
				}
				if target == adrIndex && !anchors[anchor] {
					problems = append(problems, fmt.Sprintf(
						"adr-citations: %s:%d links %s#%s, and the index has no heading with that anchor; "+
							"link one of its topic headings", relative, i+1, adrIndex, anchor))
				}
			}
		}
	}
	sort.Strings(problems)
	return problems
}

func adrIndexProblems(root, index string, adrs map[string]string) []string {
	var problems []string
	linked := map[string]bool{}
	heading := ""
	for i, line := range strings.Split(index, "\n") {
		if strings.HasPrefix(line, "#") {
			heading = strings.TrimSpace(strings.TrimLeft(line, "#"))
			continue
		}
		for _, m := range adrIndexLink.FindAllStringSubmatch(line, -1) {
			relative := adrDir + "/" + m[1]
			linked[relative] = true
			if adrs[m[1][:7]] != relative {
				problems = append(problems, fmt.Sprintf("adr-citations: %s:%d links %s, which does not exist", adrIndex, i+1, m[1]))
				continue
			}
			if title := adrTitle(root, relative); title != heading {
				problems = append(problems, fmt.Sprintf(
					"adr-citations: %s:%d lists %s under 「%s」, but the ADR is titled 「%s」; the index heading is the "+
						"anchor other documents link, so it must be the ADR's title", adrIndex, i+1, m[1], heading, title))
			}
		}
	}
	for _, relative := range adrs {
		if !linked[relative] {
			problems = append(problems, fmt.Sprintf("adr-citations: %s is not listed in %s", relative, adrIndex))
		}
	}
	return problems
}

func adrTitle(root, relative string) string {
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		return ""
	}
	first, _, _ := strings.Cut(string(body), "\n")
	_, title, found := strings.Cut(strings.TrimRight(first, "\r"), "：")
	if !found {
		return ""
	}
	return strings.TrimSpace(title)
}

func markdownAnchors(text string) map[string]bool {
	anchors := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, "#") {
			continue
		}
		var slug strings.Builder
		for _, r := range strings.ToLower(strings.TrimSpace(strings.TrimLeft(line, "#"))) {
			switch {
			case unicode.IsLetter(r) || unicode.IsNumber(r) || r == '-' || r == '_':
				slug.WriteRune(r)
			case r == ' ':
				slug.WriteRune('-')
			}
		}
		anchors[slug.String()] = true
	}
	return anchors
}

func adrCitationExempt(relative string) bool {
	for _, exemption := range adrCitationExemptions {
		if exemption.covers(relative) {
			return true
		}
	}
	return false
}
