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
	CodeBackslashPath            = "backslash-path"
	CodeNameReservedWord         = "name-reserved-word"
	CodeDescriptionXMLTag        = "description-xml-tag"
	CodeUnmentionedFile          = "unmentioned-file"
)

var authoringCodes = []string{
	CodeSkillBodyTooLong, CodeNestedReference, CodeReferenceWithoutContents,
	CodeBackslashPath, CodeNameReservedWord, CodeDescriptionXMLTag,
}

const (
	maxSkillBodyLines         = 500
	maxReferenceLinesUnlisted = 100
	contentsSearchLines       = 20
)

var contentsHeading = regexp.MustCompile(`(?i)^#{1,6}\s*(contents|table of contents|toc|目錄|目录|目次)\s*$`)

var backslashRef = regexp.MustCompile(`(?:^|[\s"'` + "`" + `(<\[])((?:[A-Za-z0-9._-]+\\)+[A-Za-z0-9._-]+\.[A-Za-z0-9]{1,8})`)

var anchorListItem = regexp.MustCompile(`^\s*(?:[-*+]|\d+\.)\s+\[[^\]]+\]\(#[^)]+\)`)

func (r *Report) RequireAuthoringRules() {
	for i, f := range r.Findings {
		if slices.Contains(authoringCodes, f.Code) {
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
	r.checkBackslashPaths(fsys, body)
	refs, _ := referencedPaths(fsys, body)
	direct := map[string]bool{skillMDPath: true}
	for _, ref := range refs {
		direct[ref] = true
	}
	nested := map[string]bool{}
	instructions := []string{body}
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
		instructions = append(instructions, content)
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
	r.checkUnmentionedFiles(fsys, strings.Join(instructions, "\n"))
}

var instructionFolders = []string{"scripts", "references"}

func (r *Report) checkUnmentionedFiles(fsys fs.FS, instructions string) {
	var candidates []string
	scripts := map[string]string{}
	EachReadableFile(fsys, func(p string, _ fs.DirEntry) {
		if !IsInstructionFile(p) {
			return
		}
		candidates = append(candidates, p)
		if strings.HasPrefix(p, "scripts/") {
			if data, err := readCapped(fsys, p, int64(maxEntryBytes)); err == nil {
				scripts[p] = string(data)
			}
		}
	})
	for _, p := range candidates {
		if mentionedIn(instructions, p) || importedByAnotherScript(scripts, p) {
			continue
		}
		r.add(SeverityInfo, CodeUnmentionedFile, p, fmt.Sprintf(
			"SKILL.md 與它連結的檔案都沒有提到 %s；Agent 不會知道要執行或閱讀它。請在本文說明何時執行（附指令）或何時閱讀。", p))
	}
}

func IsInstructionFile(p string) bool {
	folder, _, _ := strings.Cut(p, "/")
	return slices.Contains(instructionFolders, folder) && path.Base(p) != "__init__.py" &&
		(scriptExts[strings.ToLower(path.Ext(p))] || isMarkdown(p))
}

func mentionedIn(text, p string) bool {
	module := strings.TrimSuffix(strings.ReplaceAll(p, "/", "."), path.Ext(p))
	return strings.Contains(text, path.Base(p)) || strings.Contains(text, module)
}

func importedByAnotherScript(scripts map[string]string, p string) bool {
	if path.Ext(p) != pythonExt {
		return false
	}
	stem := strings.TrimSuffix(path.Base(p), pythonExt)
	imports := regexp.MustCompile(`\b(?:import|from)\s+[\w.]*\b` + regexp.QuoteMeta(stem) + `\b`)
	for other, content := range scripts {
		if other != p && imports.MatchString(content) {
			return true
		}
	}
	return false
}

func (r *Report) checkBackslashPaths(fsys fs.FS, body string) {
	seen := map[string]bool{}
	for _, m := range backslashRef.FindAllStringSubmatch(body, -1) {
		written := m[1]
		if seen[written] {
			continue
		}
		dir, _, _ := strings.Cut(written, `\`)
		if info, err := fs.Stat(fsys, dir); err != nil || !info.IsDir() {
			continue
		}
		seen[written] = true
		r.add(SeverityWarning, CodeBackslashPath, skillMDPath, fmt.Sprintf(
			"SKILL.md 用反斜線寫了路徑 %q；在 Linux 與 macOS 上找不到這個檔案，請改寫成 %q。",
			written, strings.ReplaceAll(written, `\`, "/")))
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
