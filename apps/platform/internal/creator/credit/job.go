package credit

type RecomputeArgs struct {
	StatKind      CostKind `json:"stat_kind"`
	WindowSeconds int64    `json:"window_seconds"`
}
