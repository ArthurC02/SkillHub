package operations

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/agentloop"
)

func (s *Service) LoopAgent(def Definition, actions []Action) agentloop.Agent {
	return agentloop.Agent{
		Name: def.Name, ModelRole: def.ModelRole, DailySpendCapMicros: def.DailySpendCapMicros,
		Tools: def.Tools, CheckResult: def.CheckResult,
		Conclude: func(ctx context.Context, result json.RawMessage, steps []agentloop.StepRecord) (agentloop.Conclusion, error) {
			return s.conclude(ctx, def, actions, result, steps)
		},
	}
}

func (s *Service) conclude(
	ctx context.Context, def Definition, actions []Action, result json.RawMessage, steps []agentloop.StepRecord,
) (agentloop.Conclusion, error) {
	var prepared preparedProposals
	if def.Proposals != nil {
		var err error
		if prepared, err = prepareProposals(ctx, def, actions, def.Proposals(result)); err != nil {
			return agentloop.Conclusion{}, err
		}
	}
	tracksFindings := def.Sightings != nil
	var sightings []Sighting
	if tracksFindings {
		sightings = def.Sightings(result, steps)
	}
	return agentloop.Conclusion{
		Reason: strings.Join(prepared.unpreviewed, "; "),
		Record: func(ctx context.Context, tx pgx.Tx, run pgtype.UUID, now time.Time) error {
			if tracksFindings {
				if err := s.recordFindings(ctx, tx, run, sightings, now); err != nil {
					return err
				}
			}
			return s.recordProposals(ctx, tx, run, prepared.proposals, now)
		},
	}, nil
}
