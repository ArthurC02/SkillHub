package catalog

// This file defines the source-trust, license, and derivation display rules
// (CONTENT-002, 02:§3 品質與信任狀態). Like Tier in tier.go, these are
// policy definitions only: nothing here inspects package content or
// executes anything (iron rule 1) — the underlying facts come from INGEST-004
// (source_id/content_hash on skill_sources) and manual review elsewhere.

// SourceTrust is the source-traceability axis: how sure the platform is
// about where a Skill Version actually came from, independent of its
// license or spec status.
type SourceTrust string

const (
	// SourceTrustUnknown (未知): no verifiable origin recorded.
	SourceTrustUnknown SourceTrust = "unknown"
	// SourceTrustTraceable (可追溯): repo URL, commit/ref, fetch time, and
	// content hash are on file (INGEST-004) but no reviewer has confirmed
	// them against the actual upstream repo.
	SourceTrustTraceable SourceTrust = "traceable"
	// SourceTrustManuallyConfirmed (已人工確認): a reviewer walked the
	// PDM-002 回溯准入流程— checked the repo, author, and history match the
	// whitelist entry — not just that a URL string is present.
	SourceTrustManuallyConfirmed SourceTrust = "manually_confirmed"
)

// LicenseStatus is the license axis. DISC-003 requires this to default to
// Unknown and never be inferred as permissive; ADR-012 blocks download
// packaging whenever it is not Confirmed-and-redistributable.
type LicenseStatus string

const (
	// LicenseStatusUnknown (未知): no license file or manifest field found.
	// Must never be packaged for download (ADR-012, skillpkg "license-unknown").
	LicenseStatusUnknown LicenseStatus = "unknown"
	// LicenseStatusDeclared (已宣告): a license string exists (manifest
	// field or a LICENSE file) but no reviewer has verified it against the
	// actual repository content.
	LicenseStatusDeclared LicenseStatus = "declared"
	// LicenseStatusConfirmed (已人工確認): a reviewer checked the declared
	// license against the repository (PDM-002 checklist item 1). Source-
	// available licenses (e.g. anthropics/skills' docx/pdf/pptx/xlsx) reach
	// this status but still block Download Artifact generation — Confirmed
	// means "verified", not "redistributable".
	LicenseStatusConfirmed LicenseStatus = "confirmed"
)

// TrustDisplay is the label and explanation shown for one trust axis value.
type TrustDisplay struct {
	Label string
	Note  string
}

var sourceTrustDisplays = map[SourceTrust]TrustDisplay{
	SourceTrustUnknown:           {Label: "來源未知", Note: "沒有可查核的來源紀錄。"},
	SourceTrustTraceable:         {Label: "來源可追溯", Note: "已保存 repo URL、版本／Commit、擷取時間與內容雜湊,尚未經人工核對。"},
	SourceTrustManuallyConfirmed: {Label: "來源已人工確認", Note: "已由審核者核對原始 repo、作者與歷史紀錄。"},
}

var licenseStatusDisplays = map[LicenseStatus]TrustDisplay{
	LicenseStatusUnknown:   {Label: "License 未知", Note: "未宣告 License,依規則不可下載。"},
	LicenseStatusDeclared:  {Label: "License 已宣告", Note: "套件內宣告了 License,尚未經人工核對。"},
	LicenseStatusConfirmed: {Label: "License 已人工確認", Note: "已由審核者核對宣告內容;是否可下載仍需另外檢查是否允許再散布。"},
}

// Display returns the label/note copy for t.
func (t SourceTrust) Display() TrustDisplay { return sourceTrustDisplays[t] }

// Display returns the label/note copy for l.
func (l LicenseStatus) Display() TrustDisplay { return licenseStatusDisplays[l] }

// DerivationBadge is the fork-chain indicator shown on a Skill's detail
// view (DISC-003, PACK-001 衍生關係).
type DerivationBadge struct {
	Label string
	Note  string
}

// Derivation returns the fork-chain badge for a Skill given whether it is a
// fork (skills.forked_from_skill_id in 0003_skill_registry.sql).
//
// Display rule: a fork must always resolve to both the parent skill and the
// specific parent *version* it diverged from (forked_from_version_id), not
// just the parent skill. Skill Versions are immutable snapshots (iron rule
// 4), so the parent's current version can move on after the fork; showing
// only the parent skill would silently misattribute which content the fork
// actually started from.
func Derivation(isFork bool) DerivationBadge {
	if isFork {
		return DerivationBadge{
			Label: "衍生自其他 Skill",
			Note:  "顯示原始 Skill 與分岔當下的版本;原始版本之後的變更不會自動同步。",
		}
	}
	return DerivationBadge{Label: "原始 Skill", Note: "非任何既有 Skill 的分岔。"}
}
