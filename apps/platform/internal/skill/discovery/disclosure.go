package catalog

import "github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"

type disclosure struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	Note  string `json:"note"`
}

var disclosureCatalogue = []disclosure{
	{
		Code:  skillpkg.CodeScriptFile,
		Label: "含可執行 Script 檔案",
		Note:  "套件內有 Script 檔案。平台不曾執行它們——這是靜態掃描的結果,不是行為分析。",
	},
	{

		Code:  skillpkg.CodeEmbeddedScript,
		Label: "SKILL.md 內含可執行程式碼",
		Note:  "程式碼寫在 SKILL.md 裡面,不是獨立檔案,所以看檔案清單看不出來。",
	},
	{

		Code:  skillpkg.CodeUnlabelledCodeBlock,
		Label: "含無標記的程式碼區塊",
		Note:  "SKILL.md 裡有一段沒有標語言的程式碼區塊。沒有語言標記,平台判斷不出它是什麼,也沒有把它算進上面那項或拿去抽依賴——請自己看過內容。",
	},
	{

		Code:  skillpkg.CodeSymlinkEntry,
		Label: "含符號連結",
		Note:  "套件內有指向其他路徑的連結項目,不是一般檔案。它指到哪裡由解壓的環境決定,平台沒有跟著它讀。",
	},
	{
		Code:  skillpkg.CodeUnsupportedEntryType,
		Label: "不支援的封裝項目型別",
		Note:  "套件含有裝置、FIFO、socket 或其他非普通檔案；執行環境無法安全還原這類項目，因此匯入會被阻擋。",
	},
	{
		Code:  skillpkg.CodeExternalURL,
		Label: "含外部網址",
		Note:  "套件內容指向外部位址。平台不會去取用它們,但 Skill 執行時可能會。",
	},
	{

		Code:  skillpkg.CodePossibleSecret,
		Label: "疑似含 Secret",
		Note:  "掃描比對到看起來像金鑰或憑證的字串。可能是誤判,但值得在使用前確認。",
	},
	{

		Code:  skillpkg.CodeEntryPathEscape,
		Label: "含逸出套件範圍的項目",
		Note:  "壓縮檔裡有項目宣稱自己在套件範圍之外。這類套件不會被匯入,平台也沒有解開過它。",
	},
	{
		Code:  skillpkg.CodeBinaryFile,
		Label: "含二進位檔案",
		Note:  "套件內有非文字檔。內容無法以靜態掃描判讀。",
	},
	{
		Code:  skillpkg.CodeDependencyFile,
		Label: "含依賴宣告檔",
		Note:  "套件宣告了外部依賴。實際安裝與否取決於執行環境,平台不代為解析。",
	},
	{
		Code:  skillpkg.CodeUndeclaredDependency,
		Label: "使用了沒有宣告的依賴",
		Note:  "套件的程式碼 import 了某些套件,而套件內沒有任何地方宣告它們。安裝的人不會知道要先裝什麼。",
	},
	{
		Code:  skillpkg.CodePackageDependencies,
		Label: "掃描到的外部依賴",
		Note:  "這是掃描讀出來的依賴名稱清單,不是安裝指令,平台沒有解析版本也沒有驗證它們存在。",
	},
	{

		Code:  skillpkg.CodeNestedArchive,
		Label: "內含壓縮檔",
		Note:  "套件裡有一個或多個壓縮檔，平台沒有打開它們——靜態掃描看得到的是這個套件本身的內容，壓縮檔裡面的東西不在其中。要解壓縮它們的是你的機器，不是平台。",
	},
	{

		Code:  skillpkg.CodeFileNotScanned,
		Label: "有檔案超過掃描上限",
		Note:  "套件內有檔案超過 1 MB 的掃描上限,只掃了開頭那一段。超過的部分沒有被讀過——包含 Secret 比對在內,所以「沒有掃到」在這個套件上不代表「沒有」。",
	},
}

func disclosuresFor(codes map[string]bool) []disclosure {
	out := make([]disclosure, 0, len(disclosureCatalogue))
	for _, d := range disclosureCatalogue {
		if codes[d.Code] {
			out = append(out, d)
		}
	}
	return out
}
