package agentloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

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

type Status string

const (
	Running    Status = "running"
	Completed  Status = "completed"
	Incomplete Status = "incomplete"
	Stopped    Status = "stopped"
	Failed     Status = "failed"
)

type Recorder func(ctx context.Context, tx pgx.Tx, run pgtype.UUID, now time.Time) error

type Conclusion struct {
	Reason string
	Record Recorder
}

type Agent struct {
	Name        string
	ModelRole   string
	Tools       []string
	CheckResult func(result json.RawMessage, steps []StepRecord) error
	Conclude    func(ctx context.Context, result json.RawMessage, steps []StepRecord) (Conclusion, error)
}

type Ending struct {
	Status Status
	Reason string
	Result json.RawMessage
	Record Recorder
	Now    time.Time
}

type Journal interface {
	StartRun(ctx context.Context, agent string) (pgtype.UUID, error)
	BeforeStep(ctx context.Context, run pgtype.UUID) error
	SpentSince(ctx context.Context, agent string, since time.Time) (int64, error)
	SpendCapMicros(ctx context.Context, agent string) (int64, error)
	RecordKeyBudget(ctx context.Context, run pgtype.UUID, budgetUSD float64) error
	RecordStep(ctx context.Context, run pgtype.UUID, seq int, step StepRecord, model ModelCall) error
	Finish(ctx context.Context, run pgtype.UUID, ending Ending) error
}

var (
	ErrHalted      = errors.New("agentloop: the agent is disabled or the agent brake is engaged")
	ErrRunFinished = errors.New("agentloop: the run has already finished")
	errUnoffered   = errors.New("agentloop: the model asked for a tool this agent was not offered")
)

type Report struct {
	ID     pgtype.UUID
	Status Status
	Reason string
	Result json.RawMessage
}

type Runner struct {
	Journal    Journal
	Step       func(context.Context, StepRequest) (StepDecision, error)
	IssueKey   func(ctx context.Context, run string, budgetUSD float64, ttl time.Duration) (string, error)
	RevokeKey  func(ctx context.Context, run string) error
	RecordCost func(ctx context.Context, run pgtype.UUID, seq int, call ModelCall) error
	Now        func() time.Time
}

const (
	reasonSpendCap       = "the agent's daily spend cap is reached"
	reasonStepLimit      = "the run reached its step limit"
	reasonTokens         = "the run reached its token limit"
	reasonDeadline       = "the run reached its time limit"
	reasonUnrecorded     = "the run's outcome could not be recorded: "
	reasonCostUnrecorded = "a model call's cost could not be recorded: "
	finishTool           = "finish"
	settleTimeout        = 10 * time.Second
	usdMicrosPerDollar   = 1_000_000
)

func (r *Runner) Run(ctx context.Context, agent Agent, tools []Tool, limits Limits) (Report, error) {
	run, err := r.Journal.StartRun(ctx, agent.Name)
	if err != nil {
		return Report{}, err
	}
	report, err := r.runStarted(ctx, Report{ID: run}, agent, tools, limits)
	return r.settle(ctx, report, err)
}

func (r *Runner) settle(ctx context.Context, report Report, err error) (Report, error) {
	if err == nil || errors.Is(err, ErrHalted) || errors.Is(err, ErrRunFinished) {
		return report, err
	}
	failed, ferr := r.finish(ctx, report, Failed, reasonUnrecorded+err.Error(), report.Result)
	return failed, errors.Join(err, ferr)
}

func (r *Runner) runStarted(ctx context.Context, report Report, agent Agent, tools []Tool, limits Limits) (Report, error) {
	run := report.ID
	budget, err := r.remainingBudgetUSD(ctx, agent)
	if err != nil {
		return r.finish(ctx, report, Failed, err.Error(), nil)
	}
	if budget <= 0 {
		return r.finish(ctx, report, Incomplete, reasonSpendCap, nil)
	}
	if err := r.Journal.RecordKeyBudget(ctx, run, budget); err != nil {
		return r.finish(ctx, report, Failed, err.Error(), nil)
	}
	runID := pgconv.UUIDString(run)
	key, err := r.IssueKey(ctx, runID, budget, limits.Deadline)
	if err != nil {
		return r.finish(ctx, report, Failed, "no gateway key: "+err.Error(), nil)
	}
	defer r.revoke(ctx, runID)

	deadline, cancel := context.WithTimeout(ctx, limits.Deadline)
	defer cancel()
	loop := runLoop{runner: r, agent: agent, tools: offered(agent, tools), limits: limits, run: run, key: key}
	return loop.drive(deadline, report)
}

func (r *Runner) remainingBudgetUSD(ctx context.Context, agent Agent) (float64, error) {
	now := r.Now().UTC()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	spent, err := r.Journal.SpentSince(ctx, agent.Name, dayStart)
	if err != nil {
		return 0, err
	}
	capMicros, err := r.Journal.SpendCapMicros(ctx, agent.Name)
	if err != nil {
		return 0, err
	}
	return float64(capMicros-spent) / usdMicrosPerDollar, nil
}

func (r *Runner) revoke(ctx context.Context, runID string) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), settleTimeout)
	defer cancel()
	_ = r.RevokeKey(ctx, runID)
}

func (r *Runner) finish(ctx context.Context, report Report, status Status, reason string, result json.RawMessage) (Report, error) {
	return r.end(ctx, report, Ending{Status: status, Reason: reason, Result: result})
}

func (r *Runner) complete(ctx context.Context, report Report, agent Agent, result json.RawMessage, steps []StepRecord) (Report, error) {
	ending := Ending{Status: Completed, Result: result, Now: r.Now()}
	if agent.Conclude != nil {
		conclusion, err := agent.Conclude(ctx, result, steps)
		if err != nil {
			return r.finish(ctx, report, Failed, err.Error(), result)
		}
		ending.Reason, ending.Record = conclusion.Reason, conclusion.Record
	}
	return r.end(ctx, report, ending)
}

func (r *Runner) end(ctx context.Context, report Report, ending Ending) (Report, error) {
	report.Status, report.Reason, report.Result = ending.Status, ending.Reason, ending.Result
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), settleTimeout)
	defer cancel()
	return report, r.Journal.Finish(ctx, report.ID, ending)
}

func offered(agent Agent, tools []Tool) []Tool {
	var allowed []Tool
	for _, tool := range tools {
		if slices.Contains(agent.Tools, tool.Name) {
			allowed = append(allowed, tool)
		}
	}
	return allowed
}

type runLoop struct {
	runner *Runner
	agent  Agent
	tools  []Tool
	limits Limits
	run    pgtype.UUID
	key    string
	steps  []StepRecord
	tokens int64
}

func (l *runLoop) drive(ctx context.Context, report Report) (Report, error) {
	for seq := 0; seq < l.limits.MaxSteps; seq++ {
		if err := l.runner.Journal.BeforeStep(ctx, l.run); err != nil {
			report.Status = Stopped
			return report, err
		}
		decision, err := l.ask(ctx)
		if ctx.Err() != nil {
			return l.runner.finish(ctx, report, Incomplete, reasonDeadline, nil)
		}
		if err != nil {
			return l.runner.finish(ctx, report, Failed, err.Error(), nil)
		}
		if err := l.runner.RecordCost(ctx, l.run, seq, decision.Call); err != nil {
			return l.runner.finish(ctx, report, Failed, reasonCostUnrecorded+err.Error(), nil)
		}
		l.tokens += decision.Call.PromptTokens + decision.Call.CompletionTokens
		if decision.ToolIntent == nil {
			return l.conclude(ctx, report, seq, decision)
		}
		if err := l.call(ctx, seq, *decision.ToolIntent, decision.Call); err != nil {
			return l.runner.finish(ctx, report, Failed, err.Error(), nil)
		}
		if l.tokens >= l.limits.MaxTokens {
			return l.runner.finish(ctx, report, Incomplete, reasonTokens, nil)
		}
	}
	return l.runner.finish(ctx, report, Incomplete, reasonStepLimit, nil)
}

func (l *runLoop) conclude(ctx context.Context, report Report, seq int, decision StepDecision) (Report, error) {
	if err := l.record(ctx, seq, ToolCall{Tool: finishTool, Arguments: string(decision.Result)}, "", decision.Call); err != nil {
		return l.runner.finish(ctx, report, Failed, err.Error(), nil)
	}
	if err := l.check(decision.Result); err != nil {
		return l.runner.finish(ctx, report, Failed, err.Error(), decision.Result)
	}
	return l.runner.complete(ctx, report, l.agent, decision.Result, l.steps)
}

func (l *runLoop) ask(ctx context.Context) (StepDecision, error) {
	stepCtx, cancel := context.WithTimeout(ctx, l.limits.StepTimeout)
	defer cancel()
	decision, err := l.runner.Step(stepCtx, StepRequest{
		Agent: l.agent.Name, RunID: pgconv.UUIDString(l.run), ModelRole: l.agent.ModelRole,
		Tools: l.tools, Steps: l.steps, Timeout: l.limits.StepTimeout,
		MaxOutputTokens: l.limits.MaxOutputTokens, GatewayKey: l.key,
	})
	if err != nil {
		return decision, err
	}
	if decision.ToolIntent == nil && !json.Valid(decision.Result) {
		return decision, errors.New("agentloop: the final result is not JSON")
	}
	return decision, nil
}

func (l *runLoop) check(result json.RawMessage) error {
	if l.agent.CheckResult == nil {
		return nil
	}
	if err := l.agent.CheckResult(result, l.steps); err != nil {
		return fmt.Errorf("agentloop: the result failed its check: %w", err)
	}
	return nil
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
	return l.runner.Journal.RecordStep(ctx, l.run, seq, StepRecord{ToolCall: intent, Result: result}, model)
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
