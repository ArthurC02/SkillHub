package wiring

import "github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"

type CreditRecomputeArgs struct {
	StatKind      credit.CostKind `json:"stat_kind"`
	WindowSeconds int64           `json:"window_seconds"`
}

func NewCreditRecomputeArgs(command credit.RecomputeArgs) CreditRecomputeArgs {
	return CreditRecomputeArgs{StatKind: command.StatKind, WindowSeconds: command.WindowSeconds}
}

func (a CreditRecomputeArgs) Command() credit.RecomputeArgs {
	return credit.RecomputeArgs{StatKind: a.StatKind, WindowSeconds: a.WindowSeconds}
}

func (CreditRecomputeArgs) Kind() string { return "credit_recompute_statistics" }
