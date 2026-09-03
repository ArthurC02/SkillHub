package catalog

// Category is what a Skill is *for* (PDM-001, 02:DISC-002 類別維度). It is a
// shelf, not a verdict: unlike Tier one file over, it says nothing about how
// much review the content has had, and NFR-001 forbids either axis standing in
// for the other. A reader picking 「資料」 has narrowed what the skill does and
// learned nothing about whether it is safe.
//
// The three values are the curation judgement recorded in
// tools/content/seed-skills.json, persisted onto skills.category by 0053. The
// boundary rule that file states is the one the notes below repeat verbatim:
// create/format → documents, drafting/editing prose → writing,
// tidy/filter/dedupe/merge/split/replace → data.
type Category string

const (
	// CategoryDocuments (文件): builds or reshapes a document.
	CategoryDocuments Category = "documents"
	// CategoryWriting (寫作): drafts or edits prose.
	CategoryWriting Category = "writing"
	// CategoryData (資料): reshapes a set of records rather than a document.
	CategoryData Category = "data"
	// CategoryUnassigned (尚未定值) is not a fourth shelf and is not a column
	// value — 0053 stores NULL. It is 設計 §2.9's named absence: the platform has
	// not decided how a user-imported Skill gets a category (05 R-19 is still
	// open, and (b) model-classified-at-index-time is the recorded upgrade path).
	//
	// A guessed shelf would be worse than this word, because a filter that
	// silently sorted every imported Skill onto some default shelf would make
	// 「這一格是策展判斷」 false without anybody being told (02:DISC-004).
	CategoryUnassigned Category = "unassigned"
)

// CategoryDisplay is the shelf label and the sentence that says where the label
// came from and what it does not claim (設計 §2.11(c): every badge states its
// own ceiling in the same block).
type CategoryDisplay struct {
	Label string
	Note  string
}

var categoryDisplays = map[Category]CategoryDisplay{
	CategoryDocuments: {
		Label: "文件",
		Note:  "策展時依用途歸入的分類:建立或格式化文件。分類只說它做什麼,不說它安不安全。",
	},
	CategoryWriting: {
		Label: "寫作",
		Note:  "策展時依用途歸入的分類:起草或修潤文章。分類只說它做什麼,不說它安不安全。",
	},
	CategoryData: {
		Label: "資料",
		Note:  "策展時依用途歸入的分類:整理、篩選、去重、合併、拆分或取代資料。分類只說它做什麼,不說它安不安全。",
	},
	CategoryUnassigned: {
		Label: "尚未定值",
		Note:  "平台還沒決定使用者自己匯入的 Skill 怎麼取得分類(05 R-19),所以這一格是尚未定值,不是沒有用途。",
	},
}

// Display returns the label/note copy for c. An unrecognised value keeps its raw
// value as the label, for the reason Tier.Display and axis() both spell out: a
// blank shelf reads as 「this has nothing to say about it」, and the one thing
// worse than a word the reader has to look up is no word at all.
func (c Category) Display() CategoryDisplay {
	if d, ok := categoryDisplays[c]; ok {
		return d
	}
	return CategoryDisplay{
		Label: string(c),
		Note:  "這個平台版本沒有這個分類的說明,值照原樣顯示,不猜測它的意思。",
	}
}

// categoryLabel renders the stored column. NULL — and an empty string, which is
// what a COALESCE or a hand-written row could produce — is CategoryUnassigned:
// 尚未定值, never blank and never a guessed shelf (0053's own comment, 05 R-19).
func categoryLabel(stored *string) labelled {
	value := CategoryUnassigned
	if stored != nil && *stored != "" {
		value = Category(*stored)
	}
	d := value.Display()
	return labelled{Value: string(value), Label: d.Label, Note: d.Note}
}
