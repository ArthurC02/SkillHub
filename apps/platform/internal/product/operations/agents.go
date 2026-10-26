package operations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/agentloop"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	limits "github.com/ArthurC02/skillhub/apps/platform/internal/shared/contractlimits/gen"
)

type Definition struct {
	Name                string
	Purpose             string
	ModelRole           string
	DailySpendCapMicros int64
	Tools               []string
	Actions             []string
	CheckResult         func(result json.RawMessage, steps []agentloop.StepRecord) error
	Sightings           func(result json.RawMessage, steps []agentloop.StepRecord) []Sighting
	Proposals           func(result json.RawMessage) []ProposalRequest
}

type Agent struct {
	Definition
	ID                     pgtype.UUID
	Enabled                bool
	OwnerID                pgtype.UUID
	SpendCapOverrideMicros *int64
}

const MaxDailySpendCapMicros = limits.SetPlatformAgentSpendCapBodyDailySpendCapUsdMicrosMaximum

func (a Agent) EffectiveSpendCapMicros() int64 {
	if a.SpendCapOverrideMicros != nil {
		return *a.SpendCapOverrideMicros
	}
	return a.DailySpendCapMicros
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
	auditTo     = "to"
)

var (
	ErrUnknownAgent       = errors.New("operations: no agent is registered under that name")
	ErrSpendCapOutOfRange = errors.New("operations: a daily spend cap must be above zero and at most the platform's ceiling")
)

type Service struct {
	Pool *pgxpool.Pool

	PlatformModelBudgetUSD func(context.Context) (float64, error)
}

const agentShareOfModelBudget = 0.10

var ErrModelBudgetUnknown = errors.New("讀不到平台的模型總額，所以無法確認平台 Agent 的上限總和，這次沒有改動。請稍後再試。")

type AgentCapsOverShare struct {
	EnabledMicros int64
	AllowedMicros int64
}

func (e *AgentCapsOverShare) Error() string {
	return fmt.Sprintf("已啟用的平台 Agent 每日上限合計會是 %s 美元，超過平台模型總額一成的 %s 美元；"+
		"Agent 會和所有人的模型呼叫搶同一筆總額。這次沒有改動，請先調低某個 Agent 的上限或停用一個。",
		dollars(e.EnabledMicros), dollars(e.AllowedMicros))
}

func dollars(micros int64) string {
	return strconv.FormatFloat(float64(micros)/usdMicrosPerDollar, 'f', 2, 64)
}

func agentAllowanceMicros(ctx context.Context, platformBudgetUSD func(context.Context) (float64, error)) (allowance int64, checked bool, err error) {
	if platformBudgetUSD == nil {
		return 0, false, nil
	}
	budget, err := platformBudgetUSD(ctx)
	if err != nil {
		return 0, false, fmt.Errorf("%w: %w", ErrModelBudgetUnknown, err)
	}
	return int64(math.Round(budget * agentShareOfModelBudget * usdMicrosPerDollar)), true, nil
}

// The advisory lock serializes every change that can raise the enabled
// total, so two operators enabling at once cannot both pass the check.
func requireEnabledCapsWithin(ctx context.Context, tx pgx.Tx, allowanceMicros int64) error {
	q := gen.New(tx)
	if err := q.LockPlatformAgentCaps(ctx); err != nil {
		return err
	}
	rows, err := q.ListPlatformAgents(ctx)
	if err != nil {
		return err
	}
	agents := make([]Agent, len(rows))
	for i, row := range rows {
		agents[i] = agent(row)
	}
	return enabledCapsWithin(agents, allowanceMicros)
}

func enabledCapsWithin(agents []Agent, allowanceMicros int64) error {
	var enabled int64
	for _, a := range agents {
		if a.Enabled {
			enabled += a.EffectiveSpendCapMicros()
		}
	}
	if enabled > allowanceMicros {
		return &AgentCapsOverShare{EnabledMicros: enabled, AllowedMicros: allowanceMicros}
	}
	return nil
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
	allowance, checked, err := agentAllowanceMicros(ctx, s.PlatformModelBudgetUSD)
	if err != nil && to.enabled {
		return Agent{}, err
	}
	var updated Agent
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
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
		if checked && to.enabled {
			if err := requireEnabledCapsWithin(ctx, tx, allowance); err != nil {
				return err
			}
		}
		return audit.Log(ctx, tx, audit.Event{
			Actor: operator, Action: to.action, ResourceType: audit.ResourcePlatformAgent, ResourceID: row.ID,
			Metadata: map[string]any{auditAgent: name, auditNote: note},
		})
	})
	return updated, err
}

func (s *Service) SetSpendCap(
	ctx context.Context, name string, operator pgtype.UUID, capMicros *int64, note string,
) (Agent, error) {
	if capMicros != nil && (*capMicros <= 0 || *capMicros > MaxDailySpendCapMicros) {
		return Agent{}, ErrSpendCapOutOfRange
	}
	allowance, checked, err := agentAllowanceMicros(ctx, s.PlatformModelBudgetUSD)
	if err != nil {
		return Agent{}, err
	}
	var updated Agent
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := gen.New(tx)
		before, err := q.GetPlatformAgentSpendCap(ctx, name)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnknownAgent
		}
		if err != nil {
			return err
		}
		row, err := q.SetPlatformAgentSpendCapOverride(ctx, gen.SetPlatformAgentSpendCapOverrideParams{
			Name: name, CapMicros: capMicros,
		})
		if err != nil {
			return err
		}
		updated = agent(row)
		if checked {
			if err := requireEnabledCapsWithin(ctx, tx, allowance); err != nil {
				return err
			}
		}
		return audit.Log(ctx, tx, audit.Event{
			Actor: operator, Action: audit.ActionAgentSpendCapSet, ResourceType: audit.ResourcePlatformAgent, ResourceID: row.ID,
			Metadata: map[string]any{auditAgent: name, auditNote: note, auditFrom: before, auditTo: updated.EffectiveSpendCapMicros()},
		})
	})
	return updated, err
}

func (s *Service) SpendCapMicros(ctx context.Context, name string) (int64, error) {
	capMicros, err := gen.New(s.Pool).GetPlatformAgentSpendCap(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrUnknownAgent
	}
	return capMicros, err
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
		ID: row.ID, Enabled: row.Enabled, OwnerID: row.OwnerID, SpendCapOverrideMicros: row.DailySpendCapOverrideMicros,
	}
}

func nonNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}
