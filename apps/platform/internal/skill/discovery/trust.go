package catalog

type SourceTrust string

const (
	SourceTrustUnknown SourceTrust = "unknown"

	SourceTrustTraceable SourceTrust = "traceable"

	SourceTrustManuallyConfirmed SourceTrust = "manually_confirmed"

	SourceTrustGenerated SourceTrust = "generated"
)

type LicenseStatus string

const (
	LicenseStatusUnknown LicenseStatus = "unknown"

	LicenseStatusDeclared LicenseStatus = "declared"

	LicenseStatusConfirmed LicenseStatus = "confirmed"
)

type Redistribution string

const (
	RedistributionAllowed Redistribution = "allowed"

	RedistributionBlocked Redistribution = "blocked"

	RedistributionUnknown Redistribution = "unknown"

	RedistributionSelfSupplied Redistribution = "self_supplied"

	RedistributionGenerated Redistribution = "generated"
)

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
	RedistributionAllowed:      {Label: "可再散布", Note: "已確認這個 Skill 的授權允許再散布,平台可以產出下載套件。"},
	RedistributionBlocked:      {Label: "不可再散布", Note: "授權不允許再散布,平台不會產出任何下載套件。授權已人工確認不等於可以再散布。"},
	RedistributionUnknown:      {Label: "可散布性未確認", Note: "沒有人確認過這個 Skill 可不可以再散布。未確認一律當成不可散布處理,不會產出下載套件——這不是等待中的暫時狀態,是預設就擋。"},
	RedistributionSelfSupplied: {Label: "你自己帶進來的內容", Note: "這份內容是這個工作區自己匯入的,平台把它交還給你不算再散布,所以可以打包下載。這不是對授權的判定——平台沒有、也無法替你確認它的授權允許你散布給別人。"},
	RedistributionGenerated:    {Label: "平台為你生成的內容", Note: "這份內容是平台依你的任務描述生成的,交還給你不算再散布,所以可以打包下載。**這不是對授權的判定**,也不是說它可以散布給別人——模型寫出來的東西歸誰,沒有人回答過這個問題。"},
}

func (t SourceTrust) Display() TrustDisplay { return sourceTrustDisplays[t] }

func (l LicenseStatus) Display() TrustDisplay { return licenseStatusDisplays[l] }

func (r Redistribution) Display() TrustDisplay {
	if d, ok := redistributionDisplays[r]; ok {
		return d
	}
	return redistributionDisplays[RedistributionUnknown]
}

type DerivationBadge struct {
	Label string
	Note  string
}

func Derivation(isFork bool) DerivationBadge {
	if isFork {
		return DerivationBadge{
			Label: "衍生自其他 Skill",
			Note:  "顯示原始 Skill 與分岔當下的版本;原始版本之後的變更不會自動同步。",
		}
	}
	return DerivationBadge{Label: "原始 Skill", Note: "非任何既有 Skill 的分岔。"}
}
