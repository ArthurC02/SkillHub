package catalog

type Tier string

const (
	TierCurated Tier = "curated"

	TierIndexed Tier = "indexed"

	TierExternal Tier = "external"
)

type TierDisplay struct {
	Badge          string
	TrustIndicator string
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

func (t Tier) Display() TierDisplay {
	if d, ok := tierDisplays[t]; ok {
		return d
	}
	return TierDisplay{
		Badge:          string(t),
		TrustIndicator: "這個平台版本沒有這個層級的說明,值照原樣顯示,不猜測它的意思。",
	}
}
