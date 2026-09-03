package catalog

// The disclosure catalogue: the one place a scan finding code becomes words.
//
// It exists because two payloads used to carry parallel `has_*` booleans and
// **they were not the same set** — the search row disclosed `dependency-file`
// and the detail view did not, so the screen a reader meets first said more than
// the screen they open next. Two boolean sets can drift like that silently; one
// ordered list cannot (04 丙-29 ④).
//
// The wording moved server-side for the reason 設計系統 §4.4 gives: the front end
// had two copies of these labels and they disagreed — the search row said
// 「含 Script 檔案」 where the detail view said 「含可執行 Script 檔案」. 可執行 is the
// word doing the work; dropping it is not a shorter label, it is a smaller claim.
//
// **Absence is not a claim.** A code that is not here means the scan did not find
// it, not that the package is free of it, which is why there is no "clean" member
// and no `false` entry (NFR-001, DISC-004 不得自行推定為通過).

import "github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"

// disclosure is one thing the package declares about itself, with its words.
// The contract schema is `Disclosure`.
type disclosure struct {
	Code  string `json:"code"`
	Label string `json:"label"`
	Note  string `json:"note"`
}

// disclosureCatalogue is ordered most-consequential first, and that order is what
// both surfaces render in — a reader comparing two skills is comparing positions,
// not just words.
var disclosureCatalogue = []disclosure{
	{
		Code:  skillpkg.CodeScriptFile,
		Label: "含可執行 Script 檔案",
		Note:  "套件內有 Script 檔案。平台不曾執行它們——這是靜態掃描的結果,不是行為分析。",
	},
	{
		// SKILL-003: its own code, not folded into script-file — no file list can
		// show runnable code that lives inside SKILL.md itself.
		Code:  skillpkg.CodeEmbeddedScript,
		Label: "SKILL.md 內含可執行程式碼",
		Note:  "程式碼寫在 SKILL.md 裡面,不是獨立檔案,所以看檔案清單看不出來。",
	},
	{
		// The same content one language tag short. Its own entry rather than a
		// footnote on the one above, because what the platform can say about it is
		// weaker: it knows the lines are there and nothing else.
		Code:  skillpkg.CodeUnlabelledCodeBlock,
		Label: "含無標記的程式碼區塊",
		Note:  "SKILL.md 裡有一段沒有標語言的程式碼區塊。沒有語言標記,平台判斷不出它是什麼,也沒有把它算進上面那項或拿去抽依賴——請自己看過內容。",
	},
	{
		// 04 丙-15 D-3 added this disclosure and nothing on the list layer could say
		// it, which is the drift this catalogue exists to make impossible.
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
		// Error level, so a package carrying one is blocked and never gets a version
		// row: on search and detail this entry is unreachable, and it is kept for the
		// import preview, which reports the findings of a package that was refused.
		Code:  skillpkg.CodePossibleSecret,
		Label: "疑似含 Secret",
		Note:  "掃描比對到看起來像金鑰或憑證的字串。可能是誤判,但值得在使用前確認。",
	},
	{
		// Error level and preview-only for the same reason as possible-secret above.
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
		// Last, and info level, but it is the one entry that qualifies every other:
		// what it says is that part of the package was not read at all.
		Code:  skillpkg.CodeFileNotScanned,
		Label: "有檔案超過掃描上限",
		Note:  "套件內有檔案超過 1 MB 的掃描上限,只掃了開頭那一段。超過的部分沒有被讀過——包含 Secret 比對在內,所以「沒有掃到」在這個套件上不代表「沒有」。",
	},
}

// disclosuresFor returns the catalogue entries whose code the scan recorded, in
// catalogue order. Never nil: an empty list serialises as `[]` and means the scan
// recorded none of them, which the surfaces are required not to render as 安全.
func disclosuresFor(codes map[string]bool) []disclosure {
	out := make([]disclosure, 0, len(disclosureCatalogue))
	for _, d := range disclosureCatalogue {
		if codes[d.Code] {
			out = append(out, d)
		}
	}
	return out
}
