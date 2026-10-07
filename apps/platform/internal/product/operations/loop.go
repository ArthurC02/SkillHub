package operations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
	Run         func(ctx context.Context, arguments json.RawMessage) (any, error)
}

type ToolCall struct {
	Tool      string
	Arguments string
}

type StepRecord struct {
	ToolCall
	Result string
}

type ModelCall struct {
	Model            string
	PromptVersion    string
	PromptTokens     int64
	CompletionTokens int64
	CostUSD          *float64
}

type StepRequest struct {
	Agent           string
	RunID           string
	ModelRole       string
	Tools           []Tool
	Steps           []StepRecord
	Timeout         time.Duration
	MaxOutputTokens int
	GatewayKey      string
}

type StepDecision struct {
	ToolIntent *ToolCall
	Result     json.RawMessage
	Call       ModelCall
}

type Limits struct {
	MaxSteps        int
	MaxTokens       int64
	Deadline        time.Duration
	StepTimeout     time.Duration
	MaxOutputTokens int
}

type RunReport struct {
	ID     pgtype.UUID
	Status RunStatus
	Reason string
	Result json.RawMessage
}

type Runner struct {
	Svc        *Service
	Step       func(context.Context, StepRequest) (StepDecision, error)
	IssueKey   func(ctx context.Context, run string, budgetUSD float64, ttl time.Duration) (string, error)
	RevokeKey  func(ctx context.Context, run string) error
	RecordCost func(ctx context.Context, run pgtype.UUID, call ModelCall)
	Now        func() time.Time
}

const (
	reasonSpendCap  = "the agent's daily spend cap is reached"
	reasonStepLimit = "the run reached its step limit"
	reasonTokens    = "the run reached its token limit"
	reasonDeadline  = "the run reached its time limit"
	finishTool      = "finish"
	revokeTimeout   = 10 * time.Second
)

var errUnoffered = errors.New("operations: the model asked for a tool this agent was not offered")

func (r *Runner) Run(ctx context.Context, def Definition, tools []Tool, limits Limits) (RunReport, error) {
	run, err := r.Svc.StartRun(ctx, def.Name)
	if err != nil {
		return RunReport{}, err
	}
	report := RunReport{ID: run}
	budget, err := r.remainingBudgetUSD(ctx, def)
	if err != nil {
		return r.finish(ctx, report, RunFailed, err.Error(), nil)
	}
	if budget <= 0 {
		return r.finish(ctx, report, RunIncomplete, reasonSpendCap, nil)
	}
	runID := pgconv.UUIDString(run)
	key, err := r.IssueKey(ctx, runID, budget, limits.Deadline)
	if err != nil {
		return r.finish(ctx, report, RunFailed, "no gateway key: "+err.Error(), nil)
	}
	defer r.revoke(ctx, runID)

	deadline, cancel := context.WithTimeout(ctx, limits.Deadline)
	defer cancel()
	loop := runLoop{runner: r, def: def, tools: offered(def, tools), limits: limits, run: run, key: key}
	return loop.drive(deadline, report)
}

func (r *Runner) remainingBudgetUSD(ctx context.Context, def Definition) (float64, error) {
	now := r.Now().UTC()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	spent, err := gen.New(r.Svc.Pool).PlatformAgentSpendSince(ctx, gen.PlatformAgentSpendSinceParams{
		Name: def.Name, Since: pgconv.Timestamptz(dayStart),
	})
	if err != nil {
		return 0, err
	}
	return float64(def.DailySpendCapMicros-spent) / usdMicrosPerDollar, nil
}

const usdMicrosPerDollar = 1_000_000

func (r *Runner) revoke(ctx context.Context, runID string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), revokeTimeout)
	defer cancel()
	_ = r.RevokeKey(ctx, runID)
}

func (r *Runner) finish(ctx context.Context, report RunReport, status RunStatus, reason string, result json.RawMessage) (RunReport, error) {
	report.Status, report.Reason, report.Result = status, reason, result
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), revokeTimeout)
	defer cancel()
	if err := r.Svc.finishWithResult(ctx, report.ID, status, reason, result); err != nil {
		return report, err
	}
	return report, nil
}

func offered(def Definition, tools []Tool) []Tool {
	var allowed []Tool
	for _, tool := range tools {
		if slices.Contains(def.Tools, tool.Name) {
			allowed = append(allowed, tool)
		}
	}
	return allowed
}

type runLoop struct {
	runner *Runner
	def    Definition
	tools  []Tool
	limits Limits
	run    pgtype.UUID
	key    string
	steps  []StepRecord
	tokens int64
}

func (l *runLoop) drive(ctx context.Context, report RunReport) (RunReport, error) {
	for seq := 0; seq < l.limits.MaxSteps; seq++ {
		if err := l.runner.Svc.BeforeStep(ctx, l.run); err != nil {
			report.Status = RunStopped
			return report, err
		}
		decision, err := l.ask(ctx)
		if ctx.Err() != nil {
			return l.runner.finish(ctx, report, RunIncomplete, reasonDeadline, nil)
		}
		if err != nil {
			return l.runner.finish(ctx, report, RunFailed, err.Error(), nil)
		}
		l.runner.RecordCost(ctx, l.run, decision.Call)
		l.tokens += decision.Call.PromptTokens + decision.Call.CompletionTokens
		if decision.ToolIntent == nil {
			if err := l.record(ctx, seq, ToolCall{Tool: finishTool, Arguments: string(decision.Result)}, "", decision.Call); err != nil {
				return l.runner.finish(ctx, report, RunFailed, err.Error(), nil)
			}
			return l.runner.finish(ctx, report, RunCompleted, "", decision.Result)
		}
		if err := l.call(ctx, seq, *decision.ToolIntent, decision.Call); err != nil {
			return l.runner.finish(ctx, report, RunFailed, err.Error(), nil)
		}
		if l.tokens >= l.limits.MaxTokens {
			return l.runner.finish(ctx, report, RunIncomplete, reasonTokens, nil)
		}
	}
	return l.runner.finish(ctx, report, RunIncomplete, reasonStepLimit, nil)
}

func (l *runLoop) ask(ctx context.Context) (StepDecision, error) {
	stepCtx, cancel := context.WithTimeout(ctx, l.limits.StepTimeout)
	defer cancel()
	decision, err := l.runner.Step(stepCtx, StepRequest{
		Agent: l.def.Name, RunID: pgconv.UUIDString(l.run), ModelRole: l.def.ModelRole,
		Tools: l.tools, Steps: l.steps, Timeout: l.limits.StepTimeout,
		MaxOutputTokens: l.limits.MaxOutputTokens, GatewayKey: l.key,
	})
	if err != nil {
		return decision, err
	}
	if decision.ToolIntent == nil && !json.Valid(decision.Result) {
		return decision, errors.New("operations: the final result is not JSON")
	}
	return decision, nil
}

func (l *runLoop) call(ctx context.Context, seq int, intent ToolCall, model ModelCall) error {
	i := slices.IndexFunc(l.tools, func(t Tool) bool { return t.Name == intent.Tool })
	if i < 0 {
		return fmt.Errorf("%w: %s", errUnoffered, intent.Tool)
	}
	result := toolResult(ctx, l.tools[i], intent.Arguments)
	l.steps = append(l.steps, StepRecord{ToolCall: intent, Result: result})
	return l.record(ctx, seq, intent, result, model)
}

func (l *runLoop) record(ctx context.Context, seq int, intent ToolCall, result string, model ModelCall) error {
	return gen.New(l.runner.Svc.Pool).RecordPlatformAgentStep(ctx, gen.RecordPlatformAgentStepParams{
		RunID: l.run, Seq: int32(seq), Tool: intent.Tool, Arguments: intent.Arguments, Result: result,
		Model: model.Model, PromptTokens: model.PromptTokens, CompletionTokens: model.CompletionTokens,
		UsdMicros: usdMicros(model.CostUSD),
	})
}

func toolResult(ctx context.Context, tool Tool, arguments string) string {
	out, err := tool.Run(ctx, json.RawMessage(arguments))
	if err != nil {
		out = map[string]string{"error": err.Error()}
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return `{"error":"the tool's answer could not be encoded"}`
	}
	return string(encoded)
}

func usdMicros(costUSD *float64) *int64 {
	if costUSD == nil {
		return nil
	}
	micros := int64(math.Round(*costUSD * usdMicrosPerDollar))
	return &micros
}
