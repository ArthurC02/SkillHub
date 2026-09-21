package eval

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/modelbudget"
)

var ErrNotFound = errors.New("evaluation not found")

var errEvaluationInProgress = errors.New("evaluation already in progress")
var errEvaluationSettled = errors.New("evaluation already settled")

const (
	ResultPassed       = "passed"
	ResultFailed       = "failed"
	ResultUndetermined = "undetermined"
)

const SourceModel = "model"

type Range struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type EvidenceRef struct {
	Kind string `json:"kind"`

	Match string `json:"match"`

	ReattributedFrom string `json:"reattributed_from,omitempty"`
	TraceEventID     string `json:"trace_event_id,omitempty"`
	OccurredAt       string `json:"occurred_at,omitempty"`
	ArtifactPath     string `json:"artifact_path,omitempty"`
	ByteRange        *Range `json:"byte_range,omitempty"`
	CharRange        *Range `json:"char_range,omitempty"`
	Excerpt          string `json:"excerpt"`
	ExcerptTruncated bool   `json:"excerpt_truncated"`
	Available        bool   `json:"available"`
}

const (
	KindTraceEvent  = "trace_event"
	KindArtifact    = "artifact"
	KindAgentOutput = "agent_output"
)

const (
	MatchExact      = "exact"
	MatchNormalized = "normalized"
	MatchNotFound   = "not_found"
	MatchNotChecked = "not_checked"
)

func verifiedQuote(match string) bool {
	return match == MatchExact || match == MatchNormalized
}

type CriterionResult struct {
	CriterionID string        `json:"criterion_id"`
	Text        string        `json:"text"`
	Result      string        `json:"result"`
	Source      string        `json:"source"`
	Evidence    []EvidenceRef `json:"evidence"`
	Reason      string        `json:"reason"`
}

type Finding struct {
	Category string        `json:"category"`
	Severity string        `json:"severity"`
	Message  string        `json:"message"`
	Evidence []EvidenceRef `json:"evidence"`
}

const (
	CategorySpec          = "spec"
	CategoryActivation    = "activation"
	CategoryExecution     = "execution"
	CategoryEffect        = "effect"
	CategoryCompatibility = "compatibility"
	CategoryCost          = "cost"
)

const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

type ObjectStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
}

const judgeTimeout = 135 * time.Second // budget-over: evaluate.LLM_TIMEOUT_SECONDS

// JudgeBudget names this call for an operator and bounds what they may set.
var JudgeBudget = modelbudget.Endpoint{Kind: "judge-run", Deadline: judgeTimeout}

type Service struct {
	Pool *pgxpool.Pool

	Judge Judge

	Budgets *modelbudget.Service

	Suggester Suggester

	Credits CreditsForUSD

	Credit CostRecorder

	Store ObjectStore

	Trace *trace.Service

	TestLab *testlab.Service

	Versions *ingest.Service

	ReadEventsOfType func(ctx context.Context, eventType string, since time.Time, limit int32) ([]outbox.Event, error)

	ReadRunFacts        func(ctx context.Context, workspaceID, runID pgtype.UUID) (RunFacts, bool, error)
	ReadEvaluationInput func(ctx context.Context, workspaceID, runID pgtype.UUID) (EvaluationInput, bool, error)

	ReadVersion              func(ctx context.Context, workspaceID, versionID pgtype.UUID) (VersionFacts, bool, error)
	ReadLatestVersion        func(ctx context.Context, workspaceID, skillID pgtype.UUID) (VersionFacts, bool, error)
	ReadSkill                func(ctx context.Context, workspaceID, skillID pgtype.UUID) (SkillFacts, bool, error)
	ReadRuntimeCompatibility func(context.Context, pgtype.UUID) (RuntimeCompatibility, bool, error)

	JudgeModel         string
	JudgePromptVersion string
}

func (s *Service) requireTestLab() error {
	if s.TestLab == nil {
		return errors.New("eval: test lab service not injected")
	}
	return nil
}

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
	Terminal           bool
}

type SkillFacts struct {
	ID               pgtype.UUID
	Name             string
	Summary          *string
	AccessRestricted bool
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

type ArtifactFacts struct {
	FileName    string
	ContentType string
	SizeBytes   int64
	ContentHash string
}

type ArtifactAbsence struct {
	Deleted int
	Expired int
}

func (a ArtifactAbsence) Any() bool { return a.Deleted+a.Expired > 0 }

type EvaluationInput struct {
	Run           RunFacts
	Artifacts     []ArtifactFacts
	Absent        ArtifactAbsence
	LatestAttempt int
}

var errRunReaderNotConfigured = errors.New("evaluation run reader is not configured")
var errRegistryReadNotConfigured = errors.New("evaluation registry reader is not configured")
var errRunStillGoing = errors.New("evaluation: the run has not finished")

func (s *Service) queries() *gen.Queries { return gen.New(s.Pool) }

func (s *Service) DeliverEvaluation(ctx context.Context, workspaceID, runID pgtype.UUID, retry bool) error {
	if retry {
		return s.recoverAttempt(ctx, workspaceID, runID)
	}
	return s.Evaluate(ctx, workspaceID, runID)
}

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
	ctx context.Context, q *gen.Queries, workspaceID, evaluationID pgtype.UUID, operation string,
	model, promptVersion string, usage *ModelUsage,
) error {
	if usage == nil {
		return nil
	}
	cost := usage.ReportedCostUSD()
	return q.RecordEvaluationModelUsage(ctx, gen.RecordEvaluationModelUsageParams{
		EvaluationID:     evaluationID,
		WorkspaceID:      workspaceID,
		Operation:        operation,
		Model:            orUnknown(model),
		PromptVersion:    orUnknown(promptVersion),
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		CostUsd:          numeric(cost),
		CostSource:       costSource(cost),
	})
}

func (s *Service) judgeModel() string {
	if s.JudgeModel != "" {
		return s.JudgeModel
	}

	return "gpt-5.6-terra"
}

func (s *Service) judgePromptVersion() string {
	if s.JudgePromptVersion != "" {
		return s.JudgePromptVersion
	}
	return "unreported"
}

func (s *Service) declaredJudge() (model, promptVersion string) {
	if s.Judge == nil {
		return "", ""
	}
	return s.judgeModel(), s.JudgePromptVersion
}

type material struct {
	run      RunFacts
	version  VersionFacts
	skill    SkillFacts
	snapshot testlab.Snapshot
	criteria []testlab.Criterion

	rubric *testlab.Rubric

	advanced  trace.AdvancedView
	summary   trace.Summary
	artifacts []ArtifactFacts

	absent   ArtifactAbsence
	compat   *RuntimeCompatibility
	report   skillpkg.Report
	reportOK bool
	attempt  int
}

func (s *Service) Evaluate(ctx context.Context, workspaceID, runID pgtype.UUID) error {
	m, err := s.gather(ctx, workspaceID, runID)
	if errors.Is(err, ErrNotFound) || errors.Is(err, errRunStillGoing) {
		return nil
	}
	if err != nil {
		return err
	}

	if current, currentErr := s.queries().GetCurrentEvaluation(ctx, gen.GetCurrentEvaluationParams{
		RunID: runID, WorkspaceID: workspaceID,
	}); currentErr == nil && Status(current.Status).AwaitsTheJudge() {
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

	findings := s.deterministicFindings(m)

	evidenceComplete := m.advanced.Complete && !m.absent.Any()

	if len(m.criteria) == 0 {

		return s.completeAndSuggest(ctx, m, ev, verdict{
			overall: OverallUndetermined,
			summary: "this run's test case snapshot carries no acceptance criteria, " +
				"so no task verdict could be reached; the checks below still apply",
			results:          []CriterionResult{},
			findings:         findings,
			evidenceComplete: evidenceComplete,
		})
	}

	v, err := s.judge(ctx, m, ev)
	if err != nil {

		return s.fail(ctx, m, ev, findings, evidenceComplete, err)
	}
	v.findings = append(findings, v.findings...)
	v.evidenceComplete = evidenceComplete && v.evidenceComplete
	return s.completeAndSuggest(ctx, m, ev, v)
}

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
	if !Status(current.Status).AwaitsTheJudge() {
		return nil
	}
	return s.recoverEvaluation(ctx, workspaceID, current.ID, runID)
}

func (s *Service) recoverEvaluation(
	ctx context.Context, workspaceID, evaluationID, runID pgtype.UUID,
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
	if current.ID != evaluationID || !Status(current.Status).AwaitsTheJudge() {
		return nil
	}
	m, err := s.gather(ctx, workspaceID, runID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	findings := s.deterministicFindings(m)
	return s.fail(ctx, m, current, findings, m.advanced.Complete,
		errors.New("the previous evaluation attempt was interrupted before its verdict committed"))
}

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

type verdict struct {
	overall          Overall
	summary          string
	results          []CriterionResult
	findings         []Finding
	evidenceComplete bool
	model            string
	promptVersion    string
	rubricVersion    string
	costUSD          *float64

	usage *ModelUsage
}

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
	m.absent = input.Absent
	if !m.run.Terminal {
		return m, errRunStillGoing
	}

	if m.version, found, err = s.ReadVersion(ctx, workspaceID, m.run.SkillVersionID); !found && err == nil {
		return m, ErrNotFound
	} else if err != nil {
		return m, err
	}
	if m.skill, _, err = s.ReadSkill(ctx, workspaceID, m.version.SkillID); err != nil {
		return m, err
	}
	if err := s.requireTestLab(); err != nil {
		return m, err
	}
	if m.snapshot, err = s.TestLab.ReadSnapshot(ctx, workspaceID, m.run.TestCaseSnapshotID); err != nil {
		return m, err
	}

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

func (s *Service) begin(ctx context.Context, m material) (gen.Evaluation, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.Evaluation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	// Transaction-scoped advisory lock hashed from the run's key, released
	// automatically on commit or rollback, so concurrent callers for the same
	// run serialize here instead of racing on the current-row check below.
	lockKey := "evaluation:" + pgconv.UUIDString(m.run.WorkspaceID) + ":" + pgconv.UUIDString(m.run.ID)
	if _, err := tx.Exec(ctx,
		"SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey,
	); err != nil {
		return gen.Evaluation{}, err
	}
	if err := supersedeCurrent(ctx, tx, q, m.run.WorkspaceID, m.run.ID); err != nil {
		return gen.Evaluation{}, err
	}

	declaredModel, declaredPromptVersion := s.declaredJudge()
	next := startEvaluation(m.run.WorkspaceID, m.run.ID, EvaluationStarted{
		JudgeModel:         strPtr(declaredModel),
		JudgePromptVersion: strPtr(declaredPromptVersion),
		RubricVersion:      strPtr(rubricVersionOf(m.rubric)),
	})
	if err := saveEvaluation(ctx, tx, next); err != nil {
		return gen.Evaluation{}, err
	}
	ev := next.row
	if err := trace.RecordOrchestratorEvent(ctx, tx, m.run.WorkspaceID, m.run.ID, m.attempt,
		trace.TypeEvaluationStarted, "ok", map[string]any{
			"evaluation_id":        pgconv.UUIDString(ev.ID),
			"judge_model":          s.judgeModel(),
			"judge_prompt_version": s.judgePromptVersion(),

			"rubric_version": rubricVersion(m.rubric),
		}); err != nil {
		return gen.Evaluation{}, err
	}
	return ev, tx.Commit(ctx)
}

func (s *Service) complete(ctx context.Context, m material, ev gen.Evaluation, v verdict) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	if err := settleEvaluation(ctx, tx, q, ev, func(e *Evaluation) { e.Complete(v) }); err != nil {
		return err
	}

	if err := s.recordModelUsage(ctx, q, ev.WorkspaceID, ev.ID, "judge",
		v.model, v.promptVersion, v.usage); err != nil {
		return err
	}

	s.recordEvalCost(ctx, tx, credit.KindReview, ev.WorkspaceID, ev.ID, m.run.ID,
		v.model, v.promptVersion, v.usage)

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

func (s *Service) fail(
	ctx context.Context, m material, ev gen.Evaluation,
	findings []Finding, evidenceComplete bool, cause error,
) error {

	const summary = "這次判定沒有跑完：模型閘道或證據讀取失敗。原因已記在這個 Run 的執行紀錄（進階模式）裡。"
	reason := fmt.Sprintf("the task-effect judgement could not be produced: %v", cause)

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	if err := settleEvaluation(ctx, tx, q, ev, func(e *Evaluation) {
		e.Fail(failure{summary: summary, findings: findings, evidenceComplete: evidenceComplete})
	}); errors.Is(err, errEvaluationSettled) {
		return nil
	} else if err != nil {
		return err
	}

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

func overallFrom(results []CriterionResult) Overall {
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

func rubricVersion(r *testlab.Rubric) any {
	if r == nil {
		return nil
	}
	return r.Version
}

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

func costSource(cost *float64) *string {
	if cost == nil {
		return nil
	}
	source := string(credit.CostSourceGateway)
	return &source
}
