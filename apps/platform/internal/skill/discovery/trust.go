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
	// SourceTrustGenerated (平台生成): the platform wrote these bytes from a task
	// description this workspace supplied, and the description, the model and the
	// prompt revision are all on file (GEN-006).
	//
	// It exists because `unknown` would be a false statement here. 02:GEN-002
	// forbids showing a generated package as an unknown source in as many words:
	// the origin IS known, it just is not a URL. It is deliberately not a rung
	// above `traceable` on the same ladder — there is nothing upstream to trace
	// to, and this value claims nothing whatsoever about quality or safety.
	SourceTrustGenerated SourceTrust = "generated"
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

// Redistribution is the ADR-027 決策 4 axis: may this content be handed on to
// somebody else at all. It decides whether a Download Artifact can be produced,
// and only `allowed` releases.
//
// It is neither of the two axes above and is never derived from them.
// 02:CONTENT-002 states plainly that a manually confirmed license is not
// thereby a redistributable one, so LicenseStatusConfirmed is not a release
// condition; and the 0023 hold is a reviewer's temporary decision about a
// handful of known skills, while this is a property of the content that every
// skill has an answer to. Reading the absence of a hold as permission would be
// asserting that whatever nobody objected to may be copied.
type Redistribution string

const (
	// RedistributionAllowed (可再散布): established as redistributable.
	RedistributionAllowed Redistribution = "allowed"
	// RedistributionBlocked (不可再散布): established as not redistributable —
	// source-available licenses land here even when the license itself was
	// confirmed.
	RedistributionBlocked Redistribution = "blocked"
	// RedistributionUnknown (未確認): where every skill starts and where
	// anything unclassifiable stays. Treated exactly like blocked.
	RedistributionUnknown Redistribution = "unknown"
	// RedistributionSelfSupplied (自己帶進來的): this workspace supplied the
	// bytes, so the platform handing them back is retrieval rather than
	// redistribution (0036). Releases the gate; asserts nothing about the
	// licence, which is why it is not `allowed`.
	RedistributionSelfSupplied Redistribution = "self_supplied"
	// RedistributionGenerated (平台生成的): the platform wrote these bytes for
	// this workspace at its request (0037). Releases the gate for the same
	// shape of reason as self_supplied — no upstream author exists for a
	// licence to protect — and stays a separate value because the question
	// left open is a different one: not "did the user have the right to pass
	// these on" but "who owns what a model wrote" (ADR-047 決策 4).
	RedistributionGenerated Redistribution = "generated"
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
	SourceTrustGenerated:         {Label: "平台依你的任務描述生成", Note: "沒有上游來源。平台保存了你當時輸入的任務描述、生成時間、提示詞版本與模型識別,那就是它的來源紀錄。這不是品質或安全的結論——沒有任何人看過它,也沒有任何試跑證據。"},
}

var licenseStatusDisplays = map[LicenseStatus]TrustDisplay{
	LicenseStatusUnknown:   {Label: "License 未知", Note: "未宣告 License,依規則不可下載。"},
	LicenseStatusDeclared:  {Label: "License 已宣告", Note: "套件內宣告了 License,尚未經人工核對。"},
	LicenseStatusConfirmed: {Label: "License 已人工確認", Note: "已由審核者核對宣告內容;是否可下載仍需另外檢查是否允許再散布。"},
}

var redistributionDisplays = map[Redistribution]TrustDisplay{
	RedistributionAllowed: {Label: "可再散布", Note: "已確認這個 Skill 的授權允許再散布,平台可以產出下載套件。"},
	RedistributionBlocked: {Label: "不可再散布", Note: "授權不允許再散布,平台不會產出任何下載套件。授權已人工確認不等於可以再散布。"},
	RedistributionUnknown: {Label: "可散布性未確認", Note: "沒有人確認過這個 Skill 可不可以再散布。未確認一律當成不可散布處理,不會產出下載套件——這不是等待中的暫時狀態,是預設就擋。"},
	RedistributionSelfSupplied: {Label: "你自己帶進來的內容", Note: "這份內容是這個工作區自己匯入的,平台把它交還給你不算再散布,所以可以打包下載。這不是對授權的判定——平台沒有、也無法替你確認它的授權允許你散布給別人。"},
	RedistributionGenerated: {Label: "平台為你生成的內容", Note: "這份內容是平台依你的任務描述生成的,交還給你不算再散布,所以可以打包下載。**這不是對授權的判定**,也不是說它可以散布給別人——模型寫出來的東西歸誰,沒有人回答過這個問題。"},
}

// Display returns the label/note copy for t.
func (t SourceTrust) Display() TrustDisplay { return sourceTrustDisplays[t] }

// Display returns the label/note copy for l.
func (l LicenseStatus) Display() TrustDisplay { return licenseStatusDisplays[l] }

// Display returns the label/note copy for r. A value nobody recognises gets the
// unknown copy rather than an empty one: the packaging gate already treats
// anything that is not exactly `allowed` as blocked, and a reader of a blocked
// skill must still be told why.
func (r Redistribution) Display() TrustDisplay {
	if d, ok := redistributionDisplays[r]; ok {
		return d
	}
	return redistributionDisplays[RedistributionUnknown]
}

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
