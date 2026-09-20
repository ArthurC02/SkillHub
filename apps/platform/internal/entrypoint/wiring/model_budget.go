package wiring

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/modelbudget"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	testlab "github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

// NewModelBudgets is the roster of model calls an operator may retime. A call
// missing from it keeps its compiled deadline and cannot be set.
func NewModelBudgets(pool *pgxpool.Pool) *modelbudget.Service {
	return &modelbudget.Service{
		Pool: pool,
		Endpoints: []modelbudget.Endpoint{
			catalog.MatchReasonsBudget,
			ingest.EnrichBudget,
			ingest.GenerateBudget,
			testlab.SuggestCriteriaBudget,
			eval.JudgeBudget,
			eval.SuggestImprovementsBudget,
		},
	}
}
