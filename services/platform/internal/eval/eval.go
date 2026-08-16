// Package eval is the control-plane half of the evaluation pipeline (EVAL-001).
//
// It answers a different question from internal/run. `runs.status` records what
// happened while a run executed; `evaluations.overall` records whether the task
// was achieved, and this package never writes the former (ADR-025). A run that
// finished `succeeded` with an evaluation of `not_met` is an ordinary consistent
// state, not a contradiction to be smoothed over.
//
// Everything here runs in the control plane and executes nothing (iron rule 1/2):
// trace events are read as stored JSON, artifacts as a manifest of names and
// hashes, packages as bytes handed to skillpkg.Validate. No user-supplied check
// script is run — that is untrusted content and would have to go back to a
// sandbox, which 02:EVAL-001's boundary clause puts outside the MVP.
//
// Five of EVAL-001's six problem classes are decided by rules over the platform's
// own records (deterministic.go). Only "task effect" reaches a model, through
// services/llm (judge.go), and every reference that comes back is re-verified
// here before it is stored (ADR-026 defence 3).
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

	"github.com/ArthurC02/skillhub/services/platform/internal/ingest"
	"github.com/ArthurC02/skillhub/services/platform/internal/llmclient"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/services/platform/internal/skillpkg"
	"github.com/ArthurC02/skillhub/services/platform/internal/testlab"
	"github.com/ArthurC02/skillhub/services/platform/internal/trace"
)

// ErrNotFound is "no such run or evaluation, or not yours". Existence is private
// (WS-006), so callers turn it into a 404 whichever of the three it was.
var ErrNotFound = errors.New("evaluation not found")

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
const (
	SourceRule  = "rule"
	SourceModel = "model"
	SourceUser  = "user"
)

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
	// Judge is services/llm. Nil is a deployment with no judge: evaluations of
	// runs that have acceptance criteria are recorded as `failed` saying so,
	// rather than as a verdict nothing produced (evaluation-design §6.2).
	Judge Judge
	// Store reads the stored package for the `spec` check. Nil means that one
	// check reports itself unavailable — never that the package is clean.
	Store ObjectStore
	// JudgeModel and JudgePromptVersion are what the platform intends to use, and
	// are what the `evaluation_started` event declares. What actually ran comes
	// back in the response and is what the evaluations row records; when they
	// differ, the row is the authority.
	JudgeModel         string
	JudgePromptVersion string
}

func (s *Service) queries() *gen.Queries { return gen.New(s.Pool) }

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

// material is everything one evaluation reads, gathered once. Every field comes
// from the platform's own records under the run's workspace (iron rule 3).
type material struct {
	run      gen.Run
	version  gen.SkillVersion
	skill    gen.Skill
	snapshot gen.TestCaseSnapshot
	criteria []testlab.Criterion
	// advanced and summary are the ONLY trace access path (handoff 丙-1). There is
	// no direct trace_events query anywhere in this package.
	advanced  trace.AdvancedView
	summary   trace.Summary
	artifacts []gen.Artifact
	compat    *gen.GetSkillRuntimeCompatibilityRow
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

	ev, err := s.begin(ctx, m)
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
		return s.complete(ctx, m, ev, verdict{
			overall: OverallUndetermined,
			summary: "this run's test case snapshot carries no acceptance criteria, " +
				"so no task verdict could be reached; the checks below still apply",
			results:          []CriterionResult{},
			findings:         findings,
			evidenceComplete: evidenceComplete,
			model:            s.judgeModel(),
			promptVersion:    s.judgePromptVersion(),
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
	return s.complete(ctx, m, ev, v)
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
	q := s.queries()
	m := material{attempt: 1}

	run, err := q.GetRun(ctx, gen.GetRunParams{ID: runID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	if err != nil {
		return m, err
	}
	m.run = run

	if m.version, err = q.GetSkillVersion(ctx, gen.GetSkillVersionParams{
		ID: run.SkillVersionID, WorkspaceID: workspaceID,
	}); err != nil {
		return m, err
	}
	if m.skill, err = q.GetSkill(ctx, gen.GetSkillParams{
		ID: m.version.SkillID, WorkspaceID: workspaceID,
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return m, err
	}
	if m.snapshot, err = q.GetTestCaseSnapshot(ctx, gen.GetTestCaseSnapshotParams{
		ID: run.TestCaseSnapshotID, WorkspaceID: workspaceID,
	}); err != nil {
		return m, err
	}
	// The criteria come from the snapshot, not from the editable test case
	// (iron rule 4): what was judged is what the run was asked to do.
	if m.criteria, err = testlab.DecodeCriteria(m.snapshot.AcceptanceCriteria); err != nil {
		return m, err
	}

	traceSvc := &trace.Service{Pool: s.Pool}
	if m.advanced, err = traceSvc.Advanced(ctx, workspaceID, runID); err != nil {
		return m, err
	}
	if m.summary, err = traceSvc.General(ctx, workspaceID, runID); err != nil {
		return m, err
	}
	for _, e := range m.advanced.Events {
		if e.Attempt > m.attempt {
			m.attempt = e.Attempt
		}
	}

	if m.artifacts, err = q.ListRunArtifacts(ctx, gen.ListRunArtifactsParams{
		RunID: runID, WorkspaceID: workspaceID,
	}); err != nil {
		return m, err
	}
	compat, err := q.GetSkillRuntimeCompatibility(ctx, run.SkillVersionID)
	if err == nil {
		m.compat = &compat
	} else if !errors.Is(err, pgx.ErrNoRows) {
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

	if _, err := q.SupersedeCurrentEvaluation(ctx, gen.SupersedeCurrentEvaluationParams{
		RunID: m.run.ID, WorkspaceID: m.run.WorkspaceID,
	}); err != nil {
		return gen.Evaluation{}, err
	}
	ev, err := q.CreateEvaluation(ctx, gen.CreateEvaluationParams{
		WorkspaceID: m.run.WorkspaceID, RunID: m.run.ID,
	})
	if err != nil {
		return gen.Evaluation{}, err
	}
	if err := trace.RecordOrchestratorEvent(ctx, q, m.run.WorkspaceID, m.run.ID, m.attempt,
		trace.TypeEvaluationStarted, "ok", map[string]any{
			"evaluation_id":        uuidString(ev.ID),
			"judge_model":          s.judgeModel(),
			"judge_prompt_version": s.judgePromptVersion(),
			"rubric_version":       nil,
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
	}); err != nil {
		return err
	}

	passed, failed, undetermined := tally(v.results)
	if err := trace.RecordOrchestratorEvent(ctx, q, m.run.WorkspaceID, m.run.ID, m.attempt,
		trace.TypeEvaluationCompleted, "ok", map[string]any{
			"evaluation_id":         uuidString(ev.ID),
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
	}); err != nil {
		return err
	}
	// status `error` on the envelope, `failure_reason` set: "the evaluation broke"
	// and "the judge looked and could not tell" are three states with `ok` +
	// `undetermined`, and must not be shown as one (contract README §4.1). The
	// reason quotes a gateway error, so it goes through the masker like every other
	// orchestrator payload (iron rule 11) — RecordOrchestratorEvent does that.
	if err := trace.RecordOrchestratorEvent(ctx, q, m.run.WorkspaceID, m.run.ID, m.attempt,
		trace.TypeEvaluationCompleted, "error", map[string]any{
			"evaluation_id":         uuidString(ev.ID),
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
	fsys, err := ingest.PackageFS(data)
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

func terminal(status gen.RunStatus) bool {
	switch status {
	case gen.RunStatusSucceeded, gen.RunStatusFailed, gen.RunStatusCancelled, gen.RunStatusTimedOut:
		return true
	default:
		return false
	}
}

func nonNilFindings(f []Finding) []Finding {
	if f == nil {
		return []Finding{}
	}
	return f
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
	if v == nil {
		return nil
	}
	if source != "gateway" {
		source = "estimated"
	}
	return &source
}

func uuidString(u pgtype.UUID) string {
	v, _ := u.Value()
	s, _ := v.(string)
	return s
}
