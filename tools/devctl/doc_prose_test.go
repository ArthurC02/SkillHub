package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProse(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for relative, contents := range files {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestDocProseNamesEveryShapeItLooksFor(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, line, shape string
	}{
		{"a control character", "而那正是\x01修好的那個形狀。", "a control character"},
		{"two spaces where an identifier was", "而那正是  修好的那個形狀。", "two spaces between CJK characters"},
		{"a space before the full stop", "屬部署期硬化項，見 。", "a space before closing punctuation"},
		{"two full stops in a row", "含必補回歸測試）。，該提案為允收準則。", "doubled punctuation"},
		{"an empty bracket opening", "修法見（）那一節。", "an opening bracket followed by punctuation"},
		{"a half-width comma between characters", "每一次呼叫都記一筆,不分誰買單。", "half-width punctuation in a Chinese sentence"},
		{"a half-width semicolon between characters", "這是平台的事實;使用者搜了什麼不是。", "half-width punctuation in a Chinese sentence"},
		{"a half-width comma before a Latin word", "既有 Session 失效,OAuth 不得誤認。", "half-width punctuation in a Chinese sentence"},
		{"a half-width comma after a Latin word", "存成 basis points,理由是整數運算不失真。", "half-width punctuation in a Chinese sentence"},
		{"a half-width comma after a closing quote", "次數配額問「還能跑幾次」,Credit 閘問餘額夠不夠。", "half-width punctuation in a Chinese sentence"},
		{"a half-width comma after a closing bracket", "改派換掉的是 attempt（不是 Run）,Run 的身分不變。", "half-width punctuation in a Chinese sentence"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := writeProse(t, map[string]string{"docs/plans/03-work-items.md": "前一行沒問題。\n" + tc.line + "\n"})
			problems := docProseProblems(root)
			if len(problems) != 1 {
				t.Fatalf("want exactly one problem, got %d: %v", len(problems), problems)
			}
			for _, want := range []string{"docs/plans/03-work-items.md:2", tc.shape} {
				if !strings.Contains(problems[0], want) {
					t.Errorf("problem does not name %q: %s", want, problems[0])
				}
			}
		})
	}
}

func TestDocProseAcceptsTheSameShapesOneStepInsideTheBoundary(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, line string }{
		{"one space between CJK characters is ordinary spacing", "而那正是 修好的那個形狀。"},
		{"one full stop is a sentence", "含必補回歸測試）。該提案為允收準則。"},
		{"a bracket that opens on a word", "拒絕規則（單一錯誤形狀 422）。"},
		{"a full stop that follows a character", "屬部署期硬化項。"},
		{"the full-width comma this repository writes", "每一次呼叫都記一筆，不分誰買單。"},
		{"a thousands separator inside a Chinese sentence", "entries 上限是 2,000 個。"},
		{"a comma between Latin words", "The order is script, validation, agent."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := writeProse(t, map[string]string{"AGENTS.md": tc.line + "\n"})
			if problems := docProseProblems(root); len(problems) != 0 {
				t.Fatalf("intact prose was rejected: %v", problems)
			}
		})
	}
}

func TestDocProseLeavesAloneWhatIsNotProse(t *testing.T) {
	t.Parallel()
	const hole = "而那正是  修好的那個形狀。\n"
	for _, tc := range []struct{ name, path, body string }{
		{"a fenced code block", "AGENTS.md", "說明：\n\n```text\n" + hole + "```\n"},
		{"a fenced block inside a blockquote", "AGENTS.md", "說明：\n\n> ```text\n> " + hole + "> ```\n"},
		{"a box-drawing diagram", "README.zh-TW.md", "│ Python LLM 能力服務       獨立節點上的 sandboxd │\n"},
		{"a milestone record", "docs/plans/mvp/m4/README.md", hole},
		{"a corpus fixture", "tools/goldenset/corpus/writing/humanize.md", hole},
		{"an inline code span", "AGENTS.md", "版本比大小用 `(release 日期, patch)` tuple。\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := writeProse(t, map[string]string{tc.path: tc.body})
			if problems := docProseProblems(root); len(problems) != 0 {
				t.Fatalf("%s was scanned as prose: %v", tc.name, problems)
			}
		})
	}
}

func TestDocProseIgnoresACarriageReturn(t *testing.T) {
	t.Parallel()
	root := writeProse(t, map[string]string{"AGENTS.md": "這一行以 CRLF 結尾。\r\n"})
	if problems := docProseProblems(root); len(problems) != 0 {
		t.Fatalf("a CRLF checkout was reported as damaged prose, which is the machine's state and not the repository's: %v", problems)
	}
}

func TestDocProseReportsEveryShapeOnOneLine(t *testing.T) {
	t.Parallel()
	root := writeProse(t, map[string]string{"docs/AGENTS.md": "見  那一節，。\n"})
	problems := docProseProblems(root)
	if len(problems) != 2 {
		t.Fatalf("want a problem per shape, got %d: %v", len(problems), problems)
	}
}

func TestDocProseCarriesTheOffendingTextIntoTheMessage(t *testing.T) {
	t.Parallel()
	root := writeProse(t, map[string]string{
		"docs/AGENTS.md": strings.Repeat("前置敘述", 20) + "而那正是  修好的那個形狀。" + strings.Repeat("後續敘述", 20) + "\n",
	})
	problems := docProseProblems(root)
	if len(problems) != 1 {
		t.Fatalf("want one problem, got %d: %v", len(problems), problems)
	}
	if !strings.Contains(problems[0], "修好的那個形狀") {
		t.Fatalf("the message does not quote the damaged text: %s", problems[0])
	}
	if strings.Contains(problems[0], strings.Repeat("前置敘述", 10)) {
		t.Fatalf("the message quotes the whole line instead of an excerpt: %s", problems[0])
	}
}
