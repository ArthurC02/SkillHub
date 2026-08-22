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
		Code:  "script-file",
		Label: "含可執行 Script 檔案",
		Note:  "套件內有 Script 檔案。平台不曾執行它們——這是靜態掃描的結果,不是行為分析。",
	},
	{
		// SKILL-003: its own code, not folded into script-file — no file list can
		// show runnable code that lives inside SKILL.md itself.
		Code:  "embedded-script",
		Label: "SKILL.md 內含可執行程式碼",
		Note:  "程式碼寫在 SKILL.md 裡面,不是獨立檔案,所以看檔案清單看不出來。",
	},
	{
		Code:  "external-url",
		Label: "含外部網址",
		Note:  "套件內容指向外部位址。平台不會去取用它們,但 Skill 執行時可能會。",
	},
	{
		Code:  "possible-secret",
		Label: "疑似含 Secret",
		Note:  "掃描比對到看起來像金鑰或憑證的字串。可能是誤判,但值得在使用前確認。",
	},
	{
		Code:  "binary-file",
		Label: "含二進位檔案",
		Note:  "套件內有非文字檔。內容無法以靜態掃描判讀。",
	},
	{
		Code:  "dependency-file",
		Label: "含依賴宣告檔",
		Note:  "套件宣告了外部依賴。實際安裝與否取決於執行環境,平台不代為解析。",
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
