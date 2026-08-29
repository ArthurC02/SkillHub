package catalog

// Tier is where a Skill result sits on the collection ladder (CONTENT-001,
// 02:§3 品質與信任狀態). Tier is about how much human review a result has
// had, never a safety claim — NFR-001 forbids a single "safe" badge, so a
// Tier is always shown together with the separate SourceTrust/LicenseStatus
// axes in trust.go rather than standing in for them.
type Tier string

const (
	// TierCurated (精選): passed the PDM-002 nine-item checklist in full —
	// license confirmed and redistributable (or explicitly index-only),
	// source traceable, spec valid, scripts reviewed line-by-line (<=300
	// lines total), no likely secrets, no network/MCP dependency, a
	// plain-language summary exists (CONTENT-005), at least one baseline
	// sandbox run passed (CONTENT-007/008), and the author is identifiable.
	TierCurated Tier = "curated"
	// TierIndexed (已索引): imported and passed static validation
	// (skillpkg.Validate did not block) but has not been through manual
	// curation review. May still carry unresolved warnings, e.g. an unknown
	// license or an unreviewed script.
	TierIndexed Tier = "indexed"
	// TierExternal (外部): a discovery result surfaced from outside the
	// platform (e.g. a user-submitted URL) that has not been imported at
	// all. No static validation has run against it yet.
	TierExternal Tier = "external"
)

// TierDisplay is the badge and trust-indicator copy shown next to a result
// for a given Tier (DISC-003). Wording stays factual about what was
// checked — never "safe", "official", or "endorsed" — per the PDM-002 risk
// table ("白名單被視為 Skill Hub 背書") and NFR-001.
type TierDisplay struct {
	Badge          string // short label for cards/list rows
	TrustIndicator string // one-line explanation of what the badge does and does not mean
}

var tierDisplays = map[Tier]TierDisplay{
	TierCurated: {
		Badge:          "精選",
		TrustIndicator: "已完成人工檢視:來源、License、規格、Script、Secret 掃描、白話摘要與至少一次基準試跑,不代表安全保證。",
	},
	TierIndexed: {
		Badge:          "已索引",
		TrustIndicator: "已通過規格與靜態驗證,尚未經人工精選審查;來源與 License 狀態可能仍為未知。",
	},
	TierExternal: {
		Badge:          "外部",
		TrustIndicator: "來自外部搜尋結果,尚未匯入本平台,未經任何靜態驗證。",
	},
}

// Display returns the badge/trust-indicator copy for t. A value nobody
// recognises keeps its raw value as the badge rather than rendering blank, which
// is what axis() does two files over and what Redistribution.Display does one
// file over. It used to return the zero value, and a test locked that in.
//
// Blank is the worse of the two outcomes: a badge with no text is a result row
// that says nothing about how much review it has had, and NFR-001's whole point
// is that a reader must never be able to read silence as reassurance. A word
// they have to look up at least sends them looking. Unreachable today — the
// query coalesces to one of two values — which is exactly why it is the kind of
// thing that gets discovered by the fourth tier, on the screen, in production.
func (t Tier) Display() TierDisplay {
	if d, ok := tierDisplays[t]; ok {
		return d
	}
	return TierDisplay{
		Badge:          string(t),
		TrustIndicator: "這個平台版本沒有這個層級的說明,值照原樣顯示,不猜測它的意思。",
	}
}
