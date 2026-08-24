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
