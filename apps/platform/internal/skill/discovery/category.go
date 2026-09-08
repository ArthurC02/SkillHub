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

// categorySourceClause is the one sentence 0061 added a column to be able to
// say: who put a real shelf's value there. `owner` is PUT /skills/{id}/category
// (05 R-19); anything else — including the pre-0061 rows this backfilled to
// `curated` and any value this platform version does not recognise — is worded
// as the curation judgement it always was, because that is the only other
// writer that has ever existed (0061's own comment explains why 'model' is not
// a third option yet). Either way the sentence names a source, never a
// verdict: NFR-001 forbids reading provenance as a safety claim.
func categorySourceClause(source *string) string {
	if source != nil && *source == "owner" {
		return "由擁有者標示"
	}
	return "由平台策展時分類"
}

var categoryLabels = map[Category]string{
	CategoryDocuments: "文件",
	CategoryWriting:   "寫作",
	CategoryData:      "資料",
}

var categoryWhatFor = map[Category]string{
	CategoryDocuments: "建立或格式化文件",
	CategoryWriting:   "起草或修潤文章",
	CategoryData:      "整理、篩選、去重、合併、拆分或取代資料",
}

// Display returns the label/note copy for c given who assigned it. source is
// ignored for every value except the three real shelves — CategoryUnassigned
// has no assigner (0061's pairing CHECK keeps category_source NULL exactly
// when category is) and an unrecognised value has nothing on record either.
//
// An unrecognised Category keeps its raw value as the label, for the reason
// Tier.Display and axis() both spell out: a blank shelf reads as 「this has
// nothing to say about it」, and the one thing worse than a word the reader has
// to look up is no word at all.
func (c Category) Display(source *string) CategoryDisplay {
	if c == CategoryUnassigned {
		return CategoryDisplay{
			Label: "尚未定值",
			Note:  "平台還沒決定使用者自己匯入的 Skill 怎麼取得分類(05 R-19),所以這一格是尚未定值,不是沒有用途。",
		}
	}
	if what, ok := categoryWhatFor[c]; ok {
		return CategoryDisplay{
			Label: categoryLabels[c],
			Note:  categorySourceClause(source) + "的分類:" + what + "。分類只說它做什麼,不說它安不安全。",
		}
	}
	return CategoryDisplay{
		Label: string(c),
		Note:  "這個平台版本沒有這個分類的說明,值照原樣顯示,不猜測它的意思。",
	}
}

// categoryLabel renders the stored columns. NULL — and an empty string, which
// is what a COALESCE or a hand-written row could produce — is
// CategoryUnassigned: 尚未定值, never blank and never a guessed shelf (0053's
// own comment, 05 R-19). source is 0061's category_source, read alongside it so
// the note can say who assigned a real shelf (R-19 item 4).
func categoryLabel(stored, source *string) labelled {
	value := CategoryUnassigned
	if stored != nil && *stored != "" {
		value = Category(*stored)
	}
	d := value.Display(source)
	return labelled{Value: string(value), Label: d.Label, Note: d.Note}
}
