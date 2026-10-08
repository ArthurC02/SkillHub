package operations

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type Definition struct {
	Name                string
	Purpose             string
	ModelRole           string
	DailySpendCapMicros int64
	Tools               []string
	Actions             []string
	CheckResult         func(result json.RawMessage, steps []StepRecord) error
	Sightings           func(result json.RawMessage, steps []StepRecord) []Sighting
	Proposals           func(result json.RawMessage) []ProposalRequest
}

type Agent struct {
	Definition
	ID      pgtype.UUID
	Enabled bool
	OwnerID pgtype.UUID
}

type Brake struct {
	EngagedBy pgtype.UUID
	EngagedAt time.Time
	Reason    string
}

type agentSwitch struct {
	enabled bool
	action  string
}

var (
	switchOn  = agentSwitch{enabled: true, action: audit.ActionAgentEnable}
	switchOff = agentSwitch{enabled: false, action: audit.ActionAgentDisable}
)

const (
	auditNote   = "note"
	auditError  = "error"
	auditRun    = "run"
	auditAction = "action"
	auditAgent  = "agent"
	auditFrom   = "from"
)

var ErrUnknownAgent = errors.New("operations: no agent is registered under that name")

type Service struct {
	Pool *pgxpool.Pool
}

func (s *Service) Register(ctx context.Context, definitions []Definition) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := gen.New(tx)
		for _, d := range definitions {
			if err := q.RegisterPlatformAgent(ctx, gen.RegisterPlatformAgentParams{
				Name: d.Name, Purpose: d.Purpose, ModelRole: d.ModelRole,
				DailySpendCapMicros: d.DailySpendCapMicros, Tools: nonNil(d.Tools), Actions: nonNil(d.Actions),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) Agents(ctx context.Context) ([]Agent, *Brake, error) {
	q := gen.New(s.Pool)
	rows, err := q.ListPlatformAgents(ctx)
	if err != nil {
		return nil, nil, err
	}
	agents := make([]Agent, len(rows))
	for i, row := range rows {
		agents[i] = agent(row)
	}
	brake, err := q.GetPlatformAgentBrake(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return agents, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return agents, &Brake{EngagedBy: brake.EngagedBy, EngagedAt: brake.EngagedAt.Time, Reason: brake.Reason}, nil
}

func (s *Service) Enable(ctx context.Context, name string, operator pgtype.UUID, note string) (Agent, error) {
	return s.flip(ctx, name, operator, note, switchOn)
}

func (s *Service) Disable(ctx context.Context, name string, operator pgtype.UUID, note string) (Agent, error) {
	return s.flip(ctx, name, operator, note, switchOff)
}

func (s *Service) flip(ctx context.Context, name string, operator pgtype.UUID, note string, to agentSwitch) (Agent, error) {
	var updated Agent
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		row, err := gen.New(tx).SetPlatformAgentEnabled(ctx, gen.SetPlatformAgentEnabledParams{
			Name: name, Enabled: to.enabled, OwnerID: operator,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnknownAgent
		}
		if err != nil {
			return err
		}
		updated = agent(row)
		return audit.Log(ctx, tx, audit.Event{
			Actor: operator, Action: to.action, ResourceType: audit.ResourcePlatformAgent, ResourceID: row.ID,
			Metadata: map[string]any{auditAgent: name, auditNote: note},
		})
	})
	return updated, err
}

func (s *Service) EngageBrake(ctx context.Context, operator pgtype.UUID, reason string) (Brake, error) {
	var engaged Brake
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		row, err := gen.New(tx).EngagePlatformAgentBrake(ctx, gen.EngagePlatformAgentBrakeParams{
			EngagedBy: operator, Reason: reason,
		})
		if err != nil {
			return err
		}
		engaged = Brake{EngagedBy: row.EngagedBy, EngagedAt: row.EngagedAt.Time, Reason: row.Reason}
		return audit.Log(ctx, tx, audit.Event{
			Actor: operator, Action: audit.ActionAgentBrakeEngage, ResourceType: audit.ResourcePlatformAgent,
			Metadata: map[string]any{auditNote: reason},
		})
	})
	return engaged, err
}

func (s *Service) ReleaseBrake(ctx context.Context, operator pgtype.UUID, reason string) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		released, err := gen.New(tx).ReleasePlatformAgentBrake(ctx)
		if err != nil || released == 0 {
			return err
		}
		return audit.Log(ctx, tx, audit.Event{
			Actor: operator, Action: audit.ActionAgentBrakeRelease, ResourceType: audit.ResourcePlatformAgent,
			Metadata: map[string]any{auditNote: reason},
		})
	})
}

func agent(row gen.PlatformAgent) Agent {
	return Agent{
		Definition: Definition{
			Name: row.Name, Purpose: row.Purpose, ModelRole: row.ModelRole,
			DailySpendCapMicros: row.DailySpendCapMicros, Tools: row.Tools, Actions: row.Actions,
		},
		ID: row.ID, Enabled: row.Enabled, OwnerID: row.OwnerID,
	}
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
