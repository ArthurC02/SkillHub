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

// Display returns the badge/trust-indicator copy for t. The zero
// TierDisplay is returned for a Tier value outside the three defined above.
func (t Tier) Display() TierDisplay {
	return tierDisplays[t]
}
