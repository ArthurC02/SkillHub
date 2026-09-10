package catalog

type Category string

const (
	CategoryDocuments Category = "documents"

	CategoryWriting Category = "writing"

	CategoryData Category = "data"

	CategoryUnassigned Category = "unassigned"
)

type CategoryDisplay struct {
	Label string
	Note  string
}

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

func categoryLabel(stored, source *string) labelled {
	value := CategoryUnassigned
	if stored != nil && *stored != "" {
		value = Category(*stored)
	}
	d := value.Display(source)
	return labelled{Value: string(value), Label: d.Label, Note: d.Note}
}
