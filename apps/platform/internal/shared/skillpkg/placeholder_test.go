package skillpkg

import (
	"reflect"
	"testing"
)

// Every "must NOT match" case below is a false positive the A round actually
// produced and the report recorded by name (m5/report-generate-spike.md §3.2);
// every "must match" case is the shape the rule exists for. This is what makes
// the census usable for GEN-009 ③ — the old rules measured 0 true positives
// against 2 false ones, which systematically overestimates the shell rate.

func TestATopicIsNotAPlaceholder(t *testing.T) {
	// OUT-3: a Skill whose SUBJECT is scanning for leftover TODOs.
	body := "# 提交前檢查\n\n掃描 staged code，找出遺留的 TODO 與 FIXME 註解並逐一列出。\n"
	if got := PlaceholderShapes(body); len(got) != 0 {
		t.Errorf("a skill about TODOs was flagged as containing them: %v", got)
	}
}

func TestALineLeadingTodoIsAPlaceholder(t *testing.T) {
	body := "# 步驟\n\nTODO: 補上實際的步驟\n"
	if got := PlaceholderShapes(body); !reflect.DeepEqual(got, []string{"TODO"}) {
		t.Errorf("got %v, want [TODO]", got)
	}
}

func TestAFilenameTemplateAndAnHTMLTagAreNotSlots(t *testing.T) {
	// DOC-4: `<original-name>`/`<extension>` explaining an output filename; and
	// any lowercase HTML tag, which the old `<[a-z_ -]{3,30}>` matched.
	body := "# 輸出\n\n輸出檔名為 `<original-name>.<extension>`。頁面上用 <summary> 折疊每一段。\n"
	if got := PlaceholderShapes(body); len(got) != 0 {
		t.Errorf("templates and tags were flagged as slots: %v", got)
	}
}

func TestAnActualAngleSlotStillMatches(t *testing.T) {
	body := "# 設定\n\n把 <your api key here> 換成你的金鑰。\n"
	if got := PlaceholderShapes(body); !reflect.DeepEqual(got, []string{"<angle>"}) {
		t.Errorf("got %v, want [<angle>]", got)
	}
}

func TestCodeBlocksDoNotCount(t *testing.T) {
	// The report: `ellipsis` matched standard elision inside code blocks, and
	// 6/19 packages carried extra files with code.
	body := "# 範例\n\n照下面的形狀寫：\n\n```python\ndef handle(x):\n    ...\n# TODO: caller fills this in\n```\n\n以上。\n"
	if got := PlaceholderShapes(body); len(got) != 0 {
		t.Errorf("code-block content was counted as placeholders: %v", got)
	}
}

func TestAProseEllipsisLineStillMatches(t *testing.T) {
	body := "# 步驟\n\n先做第一步。\n\n...\n\n然後結束。\n"
	if got := PlaceholderShapes(body); !reflect.DeepEqual(got, []string{"ellipsis"}) {
		t.Errorf("got %v, want [ellipsis]", got)
	}
}

func TestAnEmptySectionIsAShapeOfItsOwn(t *testing.T) {
	// The B round's 38-character shell: syntactically perfect headings with
	// nothing under them. No word-list catches it, which is why the old census
	// called that batch clean.
	body := "# 用法\n\n## 輸入\n\n## 輸出\n\n有一段真的內容在這裡。\n"
	if got := PlaceholderShapes(body); !reflect.DeepEqual(got, []string{"empty-section"}) {
		t.Errorf("got %v, want [empty-section]", got)
	}
}

func TestAFullDocumentReportsNothing(t *testing.T) {
	body := "# 掃描發票\n\n## 步驟\n\n1. 讀取每一份檔案。\n2. 抽出品項與金額。\n\n## 輸出\n\n一份 CSV，每列一筆品項。\n"
	if got := PlaceholderShapes(body); len(got) != 0 {
		t.Errorf("a complete document was flagged: %v", got)
	}
}

// The B round of review found the first version of the code stripper handled
// exactly one of four shapes, and that stripping to nothing turned a section
// whose whole body is a code block into an "empty" one — a false positive
// introduced by the commit whose entire purpose was removing false positives.
// One case per shape, plus the three rules that had no positive test at all.

func TestASectionWhoseBodyIsCodeIsNotEmpty(t *testing.T) {
	body := "# 用法\n\n## 執行\n\n```bash\nskillhub run\n```\n\n## 說明\n\n這一段有字。\n"
	if got := PlaceholderShapes(body); len(got) != 0 {
		t.Errorf("a section whose body is a code block was called empty: %v", got)
	}
}

func TestEveryCodeShapeIsStripped(t *testing.T) {
	for name, body := range map[string]string{
		"tilde fence":     "# A\n\n~~~python\ndef f():\n    ...\n# TODO: fill in\n~~~\n\n收工。\n",
		"indented fence":  "# A\n\n1. 執行：\n\n   ```bash\n   foo\n   ...\n   ```\n\n2. 完成。\n",
		"unclosed fence":  "# A\n\n說明如下。\n\n```python\ndef f():\n    ...\n# TODO: fill me\n",
		"four-space code": "# A\n\n說明如下。\n\n    def f():\n        ...\n    # TODO: later\n\n收工。\n",
	} {
		if got := PlaceholderShapes(body); len(got) != 0 {
			t.Errorf("%s: code content counted as placeholders: %v", name, got)
		}
	}
}

func TestTheRulesWithNoPositiveCaseUntilNow(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		want string
	}{
		"FIXME":     {"# 步驟\n\nFIXME 這裡要補\n", "FIXME"},
		"xxx":       {"# 步驟\n\nxxx 待填\n", "xxx"},
		"[bracket]": {"# 設定\n\n把 [your key] 填進去。\n", "[bracket]"},
	} {
		got := PlaceholderShapes(tc.body)
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("%s: got %v, want [%s] — the rule has no teeth", name, got, tc.want)
		}
	}
}
