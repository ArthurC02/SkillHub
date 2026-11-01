package skillpkg

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
)

const (
	CodeSkillBodyTooLong         = "skill-body-too-long"
	CodeNestedReference          = "nested-reference"
	CodeReferenceWithoutContents = "reference-without-contents"
)

var layoutCodes = []string{CodeSkillBodyTooLong, CodeNestedReference, CodeReferenceWithoutContents}

const (
	maxSkillBodyLines         = 500
	maxReferenceLinesUnlisted = 100
	contentsSearchLines       = 20
)

var contentsHeading = regexp.MustCompile(`(?i)^#{1,6}\s*(contents|table of contents|toc|目錄|目录|目次)\s*$`)

var anchorListItem = regexp.MustCompile(`^\s*(?:[-*+]|\d+\.)\s+\[[^\]]+\]\(#[^)]+\)`)

func (r *Report) RequireLayout() {
	for i, f := range r.Findings {
		if slices.Contains(layoutCodes, f.Code) {
			r.Findings[i].Severity = SeverityError
			r.Blocked = true
		}
	}
}

func (r *Report) checkLayout(fsys fs.FS, body string) {
	if n := lineCount(body); n > maxSkillBodyLines {
		r.add(SeverityWarning, CodeSkillBodyTooLong, skillMDPath, fmt.Sprintf(
			"SKILL.md 本文有 %d 行，超過 %d 行；本文應只是導覽，把規則表、範本、範例與背景移到 references/ 下的檔案，從本文直接連結。",
			n, maxSkillBodyLines))
	}
	refs, _ := referencedPaths(fsys, body)
	direct := map[string]bool{skillMDPath: true}
	for _, ref := range refs {
		direct[ref] = true
	}
	nested := map[string]bool{}
	for _, ref := range refs {
		if !isMarkdown(ref) {
			continue
		}
		data, err := fs.ReadFile(fsys, ref)
		if err != nil {
			continue
		}
		content := string(data)
		r.checkContentsList(ref, content)
		for _, target := range markdownLinksFrom(fsys, ref, content) {
			if direct[target] || nested[target] {
				continue
			}
			nested[target] = true
			r.add(SeverityWarning, CodeNestedReference, target, fmt.Sprintf(
				"%s 只能從 %s 連到，SKILL.md 沒有直接連結它；執行時可能只讀到它的開頭。請從 SKILL.md 直接連結，並寫一句何時要讀。",
				target, ref))
		}
	}
}

func (r *Report) checkContentsList(ref, content string) {
	n := lineCount(content)
	if n <= maxReferenceLinesUnlisted || hasContentsList(content) {
		return
	}
	r.add(SeverityWarning, CodeReferenceWithoutContents, ref, fmt.Sprintf(
		"%s 有 %d 行，開頭卻沒有目錄；執行時可能只預覽前 %d 行。請在檔案最上方加一段「## Contents」（或「## 目錄」），逐行列出下方各段標題。",
		ref, n, maxReferenceLinesUnlisted))
}

func hasContentsList(content string) bool {
	lines := strings.Split(content, "\n")
	for _, line := range lines[:min(len(lines), contentsSearchLines)] {
		line = strings.TrimSpace(line)
		if contentsHeading.MatchString(line) || anchorListItem.MatchString(line) {
			return true
		}
	}
	return false
}

func markdownLinksFrom(fsys fs.FS, from, content string) []string {
	refs, _ := referencedPaths(fsys, content)
	var out []string
	for _, ref := range refs {
		if target, ok := existingMarkdown(fsys, from, ref); ok {
			out = append(out, target)
		}
	}
	return out
}

func existingMarkdown(fsys fs.FS, from, ref string) (string, bool) {
	if !isMarkdown(ref) {
		return "", false
	}
	for _, candidate := range []string{path.Join(path.Dir(from), ref), ref} {
		if fs.ValidPath(candidate) {
			if info, err := fs.Stat(fsys, candidate); err == nil && !info.IsDir() {
				return candidate, true
			}
		}
	}
	return "", false
}

func isMarkdown(p string) bool {
	return strings.EqualFold(path.Ext(p), ".md")
}

func lineCount(s string) int {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}
