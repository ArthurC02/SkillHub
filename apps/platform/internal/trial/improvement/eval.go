// The Evaluation aggregate's write paths: begin, complete and fail. The
// invariants they hold, and why four UPDATE statements do not contradict
// ADR-026's append-only rule, are in doc.go.
package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

// ErrNotFound is "no such run or evaluation, or not yours". Existence is private
// (WS-006), so callers turn it into a 404 whichever of the three it was.
var ErrNotFound = errors.New("evaluation not found")

var errEvaluationInProgress = errors.New("evaluation already in progress")
var errEvaluationSettled = errors.New("evaluation already settled")

// The four overall verdicts (02:EVAL-001 clause 3) and the three per-criterion
// ones (clause 2). Both vocabularies are fixed by CHECK constraints in the
// database; these are the Go copies, not second definitions.
const (
	OverallMet          = "met"
	OverallPartiallyMet = "partially_met"
	OverallNotMet       = "not_met"
	OverallUndetermined = "undetermined"

	ResultPassed       = "passed"
	ResultFailed       = "failed"
	ResultUndetermined = "undetermined"
)

// Who decided. `model` is set by this package from where a verdict came, never
// from anything the model said about itself (02:EVAL-001 clause 5).
const SourceModel = "model"

// Evaluation statuses. `failed` is the evaluation not running; `undetermined` is
// it running and honestly reaching no verdict. Collapsing them would make "the
// evaluator broke" indistinguishable from "the evidence does not settle it".
const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// Range is a half-open [start, end) interval. Computed by this package after it
// locates a quote in material it holds, never taken from the model.
type Range struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// EvidenceRef is a pointer to the material a verdict rests on together with a copy
// of it (public.yaml EvidenceRef). Both, because they expire differently: trace
// events live in monthly partitions the retention policy drops, while an
// evaluation report is read long after. `available` is answered at read time.
type EvidenceRef struct {
	Kind             string `json:"kind"`
	TraceEventID     string `json:"trace_event_id,omitempty"`
	OccurredAt       string `json:"occurred_at,omitempty"`
	ArtifactPath     string `json:"artifact_path,omitempty"`
	ByteRange        *Range `json:"byte_range,omitempty"`
	CharRange        *Range `json:"char_range,omitempty"`
	Excerpt          string `json:"excerpt"`
	ExcerptTruncated bool   `json:"excerpt_truncated"`
	Available        bool   `json:"available"`
}

// Evidence kinds.
const (
	KindTraceEvent  = "trace_event"
	KindArtifact    = "artifact"
	KindAgentOutput = "agent_output"
)

// CriterionResult is one acceptance criterion's verdict (public.yaml
// CriterionResult). `criterion_id` refers to the run's frozen snapshot, so editing
// the draft afterwards cannot rewrite what was judged (iron rule 4).
type CriterionResult struct {
	CriterionID string        `json:"criterion_id"`
	Text        string        `json:"text"`
	Result      string        `json:"result"`
	Source      string        `json:"source"`
	Evidence    []EvidenceRef `json:"evidence"`
	Reason      string        `json:"reason"`
}

// Finding is one deterministic problem about the run itself (public.yaml
// DeterministicFinding). Kept apart from CriterionResult because the two answer
// different questions: "what is wrong with this run" versus "did this criterion
// pass". An empty list is not a clean bill of health.
type Finding struct {
	Category string        `json:"category"`
	Severity string        `json:"severity"`
	Message  string        `json:"message"`
	Evidence []EvidenceRef `json:"evidence"`
}

// The six categories of 02:EVAL-001 clause 1. `effect` is the only one a model
// produces; the other five are rules over the platform's own records.
const (
	CategorySpec          = "spec"
	CategoryActivation    = "activation"
	CategoryExecution     = "execution"
	CategoryEffect        = "effect"
	CategoryCompatibility = "compatibility"
	CategoryCost          = "cost"
)

// Severities, the same three levels skillpkg.Finding uses so one vocabulary
// covers the product.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

// Judge is the task-effect leg. An interface with one implementation on purpose:
// the real one is llmclient.Client, and the tests need a judge that answers
// without a Python service running (this is a seam, not an abstraction layer).
type Judge interface {
	JudgeRun(ctx context.Context, req llmclient.JudgeRunRequest) (*llmclient.JudgeRunResponse, error)
}

// ObjectStore is the one thing this package reads bytes from: the stored skill
// package, handed straight to skillpkg.Validate. Deliberately read-only and
// deliberately not internal/run's store interface — an evaluator that could reach
// the provider client would be an evaluator with sandbox control (ADR-009).
type ObjectStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
}

// judgeTimeout bounds one /judge-run call. Longer than the other LLM calls
// because the judge reads far more input, and the only deadline the internal call
// has (llmclient carries none of its own, iron rule 7).
const judgeTimeout = 120 * time.Second

// Service evaluates finished runs.
type Service struct {
	Pool *pgxpool.Pool
	// Judge is apps/llm. Nil is a deployment with no judge: evaluations of
	// runs that have acceptance criteria are recorded as `failed` saying so,
	// rather than as a verdict nothing produced (evaluation-design §6.2).
	Judge Judge
	// Suggester is apps/llm's improvement-proposal endpoint (EVAL-002). Nil is
	// a deployment that produces verdicts and no advice; an evaluation is complete
	// either way, so its absence is never an evaluation failure.
	Suggester Suggester
	// Store reads the stored package for the `spec` check, for the files a
	// suggestion is written against, and for the archive a new version is patched
	// from. Nil means those report themselves unavailable — never that the package
	// is clean.
	Store ObjectStore
	// Trace is where the run's output, errors and cost come from, and the only
	// place they come from (handoff 丙-1): no query in this package reads
	// trace_events directly, so the completeness rules that view applies apply
	// here too.
	//
	// Injected by the composition root; never constructed inside a method
	// (ADR-032 §5). Building one here looked harmless and was not — the local
	// copy carried no Signer, so it was a differently configured Trace context
	// than the one the rest of the process uses.
	Trace *trace.Service
	// TestLab owns the immutable snapshot evaluated here.
	TestLab *testlab.Service
	// Versions is the ordinary version-creation path (POST /skills/{id}/versions).
	// Applying accepted suggestions writes through it rather than around it, so a
	// suggestion-built version goes through the same validation, storage, license
	// provenance and audit trail as an uploaded one (EVAL-002 clause 4).
	Versions *ingest.Service
	// RunFacts and EvaluationInput are Run-owned reads injected by the
	// composition root. Evaluation never receives generated Run rows.
	ReadRunFacts        func(context.Context, pgtype.UUID, pgtype.UUID) (RunFacts, bool, error)
	ReadEvaluationInput func(context.Context, pgtype.UUID, pgtype.UUID) (EvaluationInput, bool, error)
	// Registry facts are injected as consumer-owned DTOs; Eval never receives
	// generated Skill or Version rows.
	ReadVersion              func(context.Context, pgtype.UUID, pgtype.UUID) (VersionFacts, bool, error)
	ReadLatestVersion        func(context.Context, pgtype.UUID, pgtype.UUID) (VersionFacts, bool, error)
	ReadSkill                func(context.Context, pgtype.UUID, pgtype.UUID) (SkillFacts, bool, error)
	ReadRuntimeCompatibility func(context.Context, pgtype.UUID) (RuntimeCompatibility, bool, error)
	// JudgeModel and JudgePromptVersion are what the platform intends to use, and
	// are what the `evaluation_started` event declares. What actually ran comes
	// back in the response and is what the evaluations row records; when they
	// differ, the row is the authority.
	JudgeModel         string
	JudgePromptVersion string
}

// RunFacts is the consumer-owned Run shape used by Evaluation.
type RunFacts struct {
	ID                 pgtype.UUID
	WorkspaceID        pgtype.UUID
	SkillVersionID     pgtype.UUID
	TestCaseSnapshotID pgtype.UUID
	Status             string
	StatusReason       *string
	RuntimeSnapshot    []byte
	StartedAt          *time.Time
	FinishedAt         *time.Time
	FailureClass       *string
}

type SkillFacts struct {
	ID                pgtype.UUID
	Name              string
	Summary           *string
	AccessRestriction *string
}

type VersionFacts struct {
	ID               pgtype.UUID
	SkillID          pgtype.UUID
	PackageObjectKey string
}

type RuntimeCompatibility struct {
	Capability   string
	Runtime      string
	RuntimeImage string
}

// ArtifactFacts is one live output-manifest row used as evidence.
type ArtifactFacts struct {
	FileName    string
	ContentType string
	SizeBytes   int64
	ContentHash string
}

// EvaluationInput is the Run-owned aggregate used by gather.
type EvaluationInput struct {
	Run           RunFacts
	Artifacts     []ArtifactFacts
	LatestAttempt int
}

var errRunReaderNotConfigured = errors.New("evaluation run reader is not configured")
var errRegistryReadNotConfigured = errors.New("evaluation registry reader is not configured")

func (s *Service) queries() *gen.Queries { return gen.New(s.Pool) }

// HasCurrentEvaluation is Eval's narrow owner read for outbox redelivery
// idempotency. Callers need existence, not the generated evaluation row.
func (s *Service) HasCurrentEvaluation(ctx context.Context, workspaceID, runID pgtype.UUID) (bool, error) {
	if s == nil || s.Pool == nil {
		return false, errors.New("evaluation persistence is not configured")
	}
	_, err := s.queries().GetCurrentEvaluation(ctx, gen.GetCurrentEvaluationParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Service) recordModelUsage(
	ctx context.Context, evaluationID, workspaceID pgtype.UUID, operation string,
	model, promptVersion string, usage *llmclient.GatewayUsage,
) error {
	if usage == nil {
		return nil
	}
	return s.queries().RecordEvaluationModelUsage(ctx, gen.RecordEvaluationModelUsageParams{
		EvaluationID:     evaluationID,
		WorkspaceID:      workspaceID,
		Operation:        operation,
		Model:            orUnknown(model),
		PromptVersion:    orUnknown(promptVersion),
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		CostUsd:          numeric(usage.CostUSD),
		CostSource:       costSource(usage.CostUSD, usage.CostSource),
	})
}

func (s *Service) judgeModel() string {
	if s.JudgeModel != "" {
		return s.JudgeModel
	}
	// PDM-003 §3 judge tier, ratified by ADR-026 decision 4. Deliberately not the
	// mini tier the sandbox workload runs on: a judge sharing a model with what it
	// judges has a thumb on the scale.
	return "gpt-5.6-terra"
}

func (s *Service) judgePromptVersion() string {
	if s.JudgePromptVersion != "" {
		return s.JudgePromptVersion
	}
	return "unreported"
}

// declaredJudge is what the row created by begin says this attempt will be judged
// under. Empty means "record NULL", and each empty is a different fact worth
// keeping distinct from a forgotten write:
//
//   - no Judge configured: no model was ever going to be involved, so naming one
//     would describe a call this deployment cannot make.
//   - no JudgePromptVersion configured: the platform declares none. The real
//     version is learned from the judge's response, which a failed attempt never
//     got — judgePromptVersion()'s "unreported" placeholder is fine in a trace
//     payload but would be a fake version in a column reports read.
func (s *Service) declaredJudge() (model, promptVersion string) {
	if s.Judge == nil {
		return "", ""
	}
	return s.judgeModel(), s.JudgePromptVersion
}

// material is everything one evaluation reads, gathered once. Every field comes
// from the platform's own records under the run's workspace (iron rule 3).
type material struct {
	run      RunFacts
	version  VersionFacts
	skill    SkillFacts
	snapshot testlab.Snapshot
	criteria []testlab.Criterion
	// rubric is CONTENT-007's, read from the snapshot for the same reason the
	// criteria are: what a run was judged against is what it was asked to do at
	// the time, not what the draft says now (iron rule 4). nil is "no rubric",
	// which is not the same as "the default rubric".
	rubric *testlab.Rubric
	// advanced and summary are the ONLY trace access path (handoff 丙-1). There is
	// no direct trace_events query anywhere in this package.
	advanced  trace.AdvancedView
	summary   trace.Summary
	artifacts []ArtifactFacts
	compat    *RuntimeCompatibility
	report    skillpkg.Report
	reportOK  bool
	attempt   int
}

// Evaluate produces one evaluation for one finished run.
//
// It is safe to call twice, but not idempotent in the "no-op" sense: a second
// call writes a second, superseding revision (ADR-026 decision 1). That is why
// the job that calls it is unique per run while live — a redelivery must not
// quietly double the judge spend.
//
// A run that is not finished is not an error: the job arrived before the
// transition it belongs to, and the terminal transition enqueues again.
func (s *Service) Evaluate(ctx context.Context, workspaceID, runID pgtype.UUID) error {
	m, err := s.gather(ctx, workspaceID, runID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !terminal(m.run.Status) {
		return nil
	}

	// Evaluate is the first-attempt path. A current pending row belongs to a call
	// already in flight, so a concurrent caller joins by doing nothing; only the
	// River recovery path and periodic stale sweep may close interrupted work.
	if current, currentErr := s.queries().GetCurrentEvaluation(ctx, gen.GetCurrentEvaluationParams{
		RunID: runID, WorkspaceID: workspaceID,
	}); currentErr == nil && current.Status == StatusPending {
		return nil
	} else if currentErr != nil && !errors.Is(currentErr, pgx.ErrNoRows) {
		return currentErr
	}

	ev, err := s.begin(ctx, m)
	if errors.Is(err, errEvaluationInProgress) {
		return nil
	}
	if err != nil {
		return err
	}

	findings := s.deterministicFindings(ctx, m)
	// 丙-1: a trace with holes cannot support a `passed`, so the fact travels with
	// the verdict rather than being noticed by whoever reads it.
	evidenceComplete := m.advanced.Complete

	if len(m.criteria) == 0 {
		// Nothing was asked of this run, so there is nothing to have met. That is
		// `undetermined` and a completed evaluation, not a failed one: the
		// deterministic findings still say what is wrong with the run.
		return s.completeAndSuggest(ctx, m, ev, verdict{
			overall: OverallUndetermined,
			summary: "this run's test case snapshot carries no acceptance criteria, " +
				"so no task verdict could be reached; the checks below still apply",
			results:          []CriterionResult{},
			findings:         findings,
			evidenceComplete: evidenceComplete,
			// No judge fields. On a `completed` row these columns mean "what
			// actually produced this verdict", and this path returns before the
			// judge is reached — naming a model here would record a metered call
			// that never happened. The declaration begin() wrote is replaced by
			// NULL for the same reason: intent is only readable as intent while
			// the row is pending or failed.
		})
	}

	v, err := s.judge(ctx, m, ev)
	if err != nil {
		// The gateway is down, or the answer did not fit the schema. Recorded as a
		// failed evaluation, never downgraded to a lenient verdict
		// (evaluation-design §6.2).
		return s.fail(ctx, m, ev, findings, evidenceComplete, err)
	}
	v.findings = append(findings, v.findings...)
	v.evidenceComplete = evidenceComplete && v.evidenceComplete
	return s.completeAndSuggest(ctx, m, ev, v)
}

// recoverAttempt is the only work a retried River job may do. A current row of
// any status proves the first attempt reached begin(): completed may be the
// result of a Commit whose acknowledgement was lost, while pending means it
// stopped before the verdict committed. Neither case may call the paid Judge
// again. Only absence of a row proves the first attempt failed before starting.
func (s *Service) recoverAttempt(ctx context.Context, workspaceID, runID pgtype.UUID) error {
	current, err := s.queries().GetCurrentEvaluation(ctx, gen.GetCurrentEvaluationParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return s.Evaluate(ctx, workspaceID, runID)
	}
	if err != nil {
		return err
	}
	if current.Status != StatusPending {
		return nil
	}
	return s.recoverEvaluation(ctx, current.ID, workspaceID, runID)
}

// recoverEvaluation closes only the exact pending revision selected by a
// recovery scan. A newer revision may supersede it between the scan and this
// call; looking up the run's then-current row without comparing IDs would fail
// that newer evaluation instead.
func (s *Service) recoverEvaluation(
	ctx context.Context, evaluationID, workspaceID, runID pgtype.UUID,
) error {
	current, err := s.queries().GetCurrentEvaluation(ctx, gen.GetCurrentEvaluationParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if current.ID != evaluationID || current.Status != StatusPending {
		return nil
	}
	m, err := s.gather(ctx, workspaceID, runID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	findings := s.deterministicFindings(ctx, m)
	return s.fail(ctx, m, current, findings, m.advanced.Complete,
		errors.New("the previous evaluation attempt was interrupted before its verdict committed"))
}

// completeAndSuggest stores the verdict and then asks for improvement suggestions
// (EVAL-002). In that order and not the other way round: the verdict is what this
// job owes, and suggestions are advice about a verdict that already exists. If the
// suggestion call fails, the evaluation is still complete and nothing is retried —
// paying for a second judgement to recover advice would be the more expensive half.
func (s *Service) completeAndSuggest(
	ctx context.Context, m material, ev gen.Evaluation, v verdict,
) error {
	if err := s.complete(ctx, m, ev, v); err != nil {
		if errors.Is(err, errEvaluationSettled) {
			return nil
		}
		return err
	}
	s.suggest(ctx, m, ev, v)
	return nil
}

// verdict is the merged answer on its way to the database.
type verdict struct {
	overall          string
	summary          string
	results          []CriterionResult
	findings         []Finding
	evidenceComplete bool
	model            string
	promptVersion    string
	rubricVersion    string
	costUSD          *float64
	costSource       string
}

// gather reads every input under the run's own workspace.
func (s *Service) gather(ctx context.Context, workspaceID, runID pgtype.UUID) (material, error) {
	m := material{}
	if s.ReadEvaluationInput == nil {
		return m, errRunReaderNotConfigured
	}
	if s.ReadVersion == nil || s.ReadSkill == nil || s.ReadRuntimeCompatibility == nil {
		return m, errRegistryReadNotConfigured
	}
	input, found, err := s.ReadEvaluationInput(ctx, workspaceID, runID)
	if !found && err == nil {
		return m, ErrNotFound
	}
	if err != nil {
		return m, err
	}
	m.run, m.artifacts, m.attempt = input.Run, input.Artifacts, input.LatestAttempt

	if m.version, found, err = s.ReadVersion(ctx, workspaceID, m.run.SkillVersionID); !found && err == nil {
		return m, ErrNotFound
	} else if err != nil {
		return m, err
	}
	if m.skill, _, err = s.ReadSkill(ctx, workspaceID, m.version.SkillID); err != nil {
		return m, err
	}
	if s.TestLab == nil || s.TestLab.Pool == nil {
		return m, errors.New("eval: test lab service not injected")
	}
	if m.snapshot, err = s.TestLab.ReadSnapshot(ctx, workspaceID, m.run.TestCaseSnapshotID); err != nil {
		return m, err
	}
	// The criteria come from the snapshot, not from the editable test case
	// (iron rule 4): what was judged is what the run was asked to do.
	if m.criteria, err = testlab.DecodeCriteria(m.snapshot.AcceptanceCriteria); err != nil {
		return m, err
	}
	if m.rubric, err = testlab.DecodeRubric(m.snapshot.Rubric); err != nil {
		return m, err
	}

	if m.advanced, err = s.Trace.AdvancedAll(ctx, workspaceID, runID); err != nil {
		return m, err
	}
	if m.summary, err = s.Trace.General(ctx, workspaceID, runID); err != nil {
		return m, err
	}
	compat, found, err := s.ReadRuntimeCompatibility(ctx, m.run.SkillVersionID)
	if err == nil && found {
		m.compat = &compat
	} else if err != nil {
		return m, err
	}

	m.report, m.reportOK = s.packageReport(ctx, m.version.PackageObjectKey)
	return m, nil
}

// begin supersedes the standing verdict, writes the new pending row, and emits
// `evaluation_started` — all in one transaction, so the trace event and the row it
// describes commit together (iron rule 9, handoff 丙-2).
//
// The supersede goes first and must: evaluations_current_key is a partial unique
// index and is not deferrable, so an insert before it collides with the standing
// verdict rather than replacing it.
func (s *Service) begin(ctx context.Context, m material) (gen.Evaluation, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.Evaluation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)
	// Re-evaluation is append-only, but two callers may arrive together. Serialize
	// the supersede/create pair so the partial current-row index is not a race.
	lockKey := "evaluation:" + pgconv.UUIDString(m.run.WorkspaceID) + ":" + pgconv.UUIDString(m.run.ID)
	if _, err := tx.Exec(ctx,
		"SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey,
	); err != nil {
		return gen.Evaluation{}, err
	}
	if current, err := q.GetCurrentEvaluation(ctx, gen.GetCurrentEvaluationParams{
		RunID: m.run.ID, WorkspaceID: m.run.WorkspaceID,
	}); err == nil && current.Status == StatusPending {
		return gen.Evaluation{}, errEvaluationInProgress
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return gen.Evaluation{}, err
	}

	if _, err := q.SupersedeCurrentEvaluation(ctx, gen.SupersedeCurrentEvaluationParams{
		RunID: m.run.ID, WorkspaceID: m.run.WorkspaceID,
	}); err != nil {
		return gen.Evaluation{}, err
	}
	// The conditions this attempt is about to be judged under, recorded on the row
	// before anything runs. complete() overwrites all three with what the response
	// says actually ran; fail() leaves them, which is the whole point — a failed
	// evaluation is the one where "which judge could not answer" is asked first,
	// and until now that lived only in the `evaluation_started` event below, which
	// retention drops with its partition (ADR-026 decision 1).
	declaredModel, declaredPromptVersion := s.declaredJudge()
	ev, err := q.CreateEvaluation(ctx, gen.CreateEvaluationParams{
		WorkspaceID:        m.run.WorkspaceID,
		RunID:              m.run.ID,
		JudgeModel:         strPtr(declaredModel),
		JudgePromptVersion: strPtr(declaredPromptVersion),
		RubricVersion:      strPtr(rubricVersionOf(m.rubric)),
	})
	if err != nil {
		return gen.Evaluation{}, err
	}
	if err := trace.RecordOrchestratorEvent(ctx, tx, m.run.WorkspaceID, m.run.ID, m.attempt,
		trace.TypeEvaluationStarted, "ok", map[string]any{
			"evaluation_id":        pgconv.UUIDString(ev.ID),
			"judge_model":          s.judgeModel(),
			"judge_prompt_version": s.judgePromptVersion(),
			// The rubric this run's snapshot froze, or null when it has none.
			// Declared here for the same reason judge_model is: this is what the
			// platform is about to judge under. What was actually in force is on
			// the evaluations row (judge.go), and where they differ the row wins.
			"rubric_version": rubricVersion(m.rubric),
		}); err != nil {
		return gen.Evaluation{}, err
	}
	return ev, tx.Commit(ctx)
}

// complete writes the verdict and its `evaluation_completed` event in one
// transaction. Nothing here touches `runs` (ADR-025).
func (s *Service) complete(ctx context.Context, m material, ev gen.Evaluation, v verdict) error {
	results, err := json.Marshal(v.results)
	if err != nil {
		return err
	}
	findings, err := json.Marshal(nonNilFindings(v.findings))
	if err != nil {
		return err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	if _, err := q.CompleteEvaluation(ctx, gen.CompleteEvaluationParams{
		ID: ev.ID, WorkspaceID: ev.WorkspaceID,
		Overall:               v.overall,
		Summary:               strPtr(v.summary),
		CriterionResults:      results,
		DeterministicFindings: findings,
		JudgeModel:            strPtr(v.model),
		JudgePromptVersion:    strPtr(v.promptVersion),
		RubricVersion:         strPtr(v.rubricVersion),
		EvidenceComplete:      v.evidenceComplete,
		CostUsd:               numeric(v.costUSD),
		CostSource:            costSource(v.costUSD, v.costSource),
	}); errors.Is(err, pgx.ErrNoRows) {
		return errEvaluationSettled
	} else if err != nil {
		return err
	}

	passed, failed, undetermined := tally(v.results)
	if err := trace.RecordOrchestratorEvent(ctx, tx, m.run.WorkspaceID, m.run.ID, m.attempt,
		trace.TypeEvaluationCompleted, "ok", map[string]any{
			"evaluation_id":         pgconv.UUIDString(ev.ID),
			"overall":               v.overall,
			"criteria_total":        len(v.results),
			"criteria_passed":       passed,
			"criteria_failed":       failed,
			"criteria_undetermined": undetermined,
			"evidence_complete":     v.evidenceComplete,
			"cost_usd":              v.costUSD,
			"failure_reason":        nil,
		}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// fail records an evaluation that could not reach a verdict. The deterministic
// findings are kept: they came from the platform's own records and are still true
// when only the model leg broke.
func (s *Service) fail(
	ctx context.Context, m material, ev gen.Evaluation,
	findings []Finding, evidenceComplete bool, cause error,
) error {
	reason := fmt.Sprintf("the task-effect judgement could not be produced: %v", cause)
	encoded, err := json.Marshal(nonNilFindings(findings))
	if err != nil {
		return err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	if _, err := q.FailEvaluation(ctx, gen.FailEvaluationParams{
		ID: ev.ID, WorkspaceID: ev.WorkspaceID,
		Summary:               strPtr(reason),
		DeterministicFindings: encoded,
		EvidenceComplete:      evidenceComplete,
	}); errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	// status `error` on the envelope, `failure_reason` set: "the evaluation broke"
	// and "the judge looked and could not tell" are three states with `ok` +
	// `undetermined`, and must not be shown as one (contract README §4.1). The
	// reason quotes a gateway error, so it goes through the masker like every other
	// orchestrator payload (iron rule 11) — RecordOrchestratorEvent does that.
	if err := trace.RecordOrchestratorEvent(ctx, tx, m.run.WorkspaceID, m.run.ID, m.attempt,
		trace.TypeEvaluationCompleted, "error", map[string]any{
			"evaluation_id":         pgconv.UUIDString(ev.ID),
			"overall":               OverallUndetermined,
			"criteria_total":        len(m.criteria),
			"criteria_passed":       0,
			"criteria_failed":       0,
			"criteria_undetermined": len(m.criteria),
			"evidence_complete":     evidenceComplete,
			"cost_usd":              nil,
			"failure_reason":        reason,
		}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// packageReport reads the stored package and validates it without executing any of
// it (iron rule 1). ok is false when the scan could not be performed at all, which
// callers must treat as a different answer from "scanned and clean" (SEC-002).
func (s *Service) packageReport(ctx context.Context, objectKey string) (skillpkg.Report, bool) {
	if s.Store == nil || objectKey == "" {
		return skillpkg.Report{}, false
	}
	data, err := s.Store.Get(ctx, objectKey)
	if err != nil {
		return skillpkg.Report{}, false
	}
	fsys, err := skillpkg.PackageFS(data)
	if err != nil {
		return skillpkg.Report{}, false
	}
	return skillpkg.Validate(fsys), true
}

// overallFrom is the stored verdict, recomputed here rather than taken from the
// model: the judge's own `overall` is a proposal made before Go downgraded
// anything whose evidence did not verify (llm-internal.yaml JudgeVerdict.overall).
func overallFrom(results []CriterionResult) string {
	if len(results) == 0 {
		return OverallUndetermined
	}
	passed, failed, _ := tally(results)
	switch {
	case passed == len(results):
		return OverallMet
	case failed == len(results):
		return OverallNotMet
	case passed > 0:
		return OverallPartiallyMet
	case failed > 0:
		// Nothing passed and at least one criterion definitely did not. The rest
		// being undetermined does not soften that into "partly".
		return OverallNotMet
	default:
		return OverallUndetermined
	}
}

func tally(results []CriterionResult) (passed, failed, undetermined int) {
	for _, r := range results {
		switch r.Result {
		case ResultPassed:
			passed++
		case ResultFailed:
			failed++
		default:
			undetermined++
		}
	}
	return passed, failed, undetermined
}

func terminal(status string) bool {
	switch status {
	case "succeeded", "failed", "cancelled", "timed_out":
		return true
	default:
		return false
	}
}

func (s *Service) runFacts(ctx context.Context, workspaceID, runID pgtype.UUID) (RunFacts, error) {
	if s.ReadRunFacts == nil {
		return RunFacts{}, errRunReaderNotConfigured
	}
	run, found, err := s.ReadRunFacts(ctx, workspaceID, runID)
	if err != nil {
		return RunFacts{}, err
	}
	if !found {
		return RunFacts{}, ErrNotFound
	}
	return run, nil
}

func nonNilFindings(f []Finding) []Finding {
	if f == nil {
		return []Finding{}
	}
	return f
}

// rubricVersion renders a rubric for the trace payload: the version string, or
// JSON null when there is no rubric. Not "" — an empty string in a payload reads
// as "a rubric with an unnamed version", which is a different claim.
func rubricVersion(r *testlab.Rubric) any {
	if r == nil {
		return nil
	}
	return r.Version
}

// rubricVersionOf is the same fact in the form the column takes: "" where
// rubricVersion renders null, because strPtr turns "" into that same NULL.
func rubricVersionOf(r *testlab.Rubric) string {
	if r == nil {
		return ""
	}
	return r.Version
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// numeric converts a cost to the column's type. A nil cost stays NULL, which the
// UI must render as "unreported" and never as 0 — 0 says the judgement was free.
func numeric(v *float64) pgtype.Numeric {
	var n pgtype.Numeric
	if v == nil {
		return n
	}
	if err := n.Scan(fmt.Sprintf("%.6f", *v)); err != nil {
		return pgtype.Numeric{}
	}
	return n
}

// costSource keeps the 0024 constraint that a cost and its provenance are both
// present or both absent: an unlabelled number is exactly what NFR-001 calls
// misleading.
func costSource(v *float64, source string) *string {
	if v == nil || source != "gateway" {
		return nil
	}
	return &source
}
