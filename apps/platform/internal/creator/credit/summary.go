package credit

import (
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

const statisticsCostSource = CostSourceGateway

type sessionStep struct {
	userID    pgtype.UUID
	usdMicros int64
	source    CostSource
	at        pgtype.Timestamptz
}

type sessionCostSummary struct {
	sessionID  pgtype.UUID
	userID     pgtype.UUID
	usdMicros  int64
	steps      int32
	estimated  bool
	lastStepAt pgtype.Timestamptz
}

func summarizeSession(sessionID pgtype.UUID, stepsInOrder []sessionStep) sessionCostSummary {
	summary := sessionCostSummary{sessionID: sessionID}
	for _, step := range stepsInOrder {
		summary.usdMicros += step.usdMicros
		summary.steps++
		summary.estimated = summary.estimated || step.source == CostSourceEstimated
		summary.userID = step.userID
		summary.lastStepAt = step.at
	}
	return summary
}

func (s sessionCostSummary) upsert() gen.UpsertSessionCostSummaryParams {
	return gen.UpsertSessionCostSummaryParams{
		SessionID: s.sessionID, UserID: s.userID, UsdMicros: s.usdMicros,
		Steps: s.steps, Estimated: s.estimated, LastStepAt: s.lastStepAt,
	}
}

func sessionStepsByID(rows []gen.ListSessionStepCostsRow) map[pgtype.UUID][]sessionStep {
	bySession := map[pgtype.UUID][]sessionStep{}
	for _, row := range rows {
		bySession[row.SessionID] = append(bySession[row.SessionID], sessionStep{
			userID: row.UserID, usdMicros: row.UsdMicros, source: CostSource(row.CostSource), at: row.CreatedAt,
		})
	}
	return bySession
}
