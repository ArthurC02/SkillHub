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
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/checks"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/facts"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/graders"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/judging"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/modelport"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/verdict"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/modelbudget"
)

var ErrNotFound = errors.New("evaluation not found")

var errEvaluationInProgress = errors.New("evaluation already in progress")
var errEvaluationSettled = errors.New("evaluation already settled")

type ObjectStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
}

const judgeTimeout = 135 * time.Second // budget-over: evaluate.LLM_TIMEOUT_SECONDS

// JudgeBudget names this call for an operator and bounds what they may set.
var JudgeBudget = modelbudget.Endpoint{Kind: "judge-run", Deadline: judgeTimeout}

const EvaluationJobTimeout = judgeTimeout + suggestTimeout + time.Minute

type Service struct {
	Pool *pgxpool.Pool

	Judge modelport.Judge

	Budgets *modelbudget.Service

	Suggester modelport.Suggester

	Credits CreditsForUSD

	Credit CostRecorder

	Store ObjectStore

	Trace *trace.Service

	TestLab *testlab.Service

	Versions *ingest.Service

	ReadEventsOfType func(ctx context.Context, page outbox.EventPage) ([]outbox.Event, error)

	ReadRunFacts        func(ctx context.Context, workspaceID, runID pgtype.UUID) (facts.Run, bool, error)
	ReadEvaluationInput func(ctx context.Context, workspaceID, runID pgtype.UUID) (facts.EvaluationInput, bool, error)

	ReadVersion              func(ctx context.Context, workspaceID, versionID pgtype.UUID) (facts.Version, bool, error)
	ReadLatestVersion        func(ctx context.Context, workspaceID, skillID pgtype.UUID) (facts.Version, bool, error)
	ReadSkill                func(ctx context.Context, workspaceID, skillID pgtype.UUID) (facts.Skill, bool, error)
	ReadRuntimeCompatibility func(context.Context, pgtype.UUID) (facts.RuntimeCompatibility, bool, error)

	JudgeModel         string
	PanelEnabled       func(context.Context) (bool, error)
	JudgePromptVersion string
}

func (s *Service) requireTestLab() error {
	if s.TestLab == nil {
		return errors.New("eval: test lab service not injected")
	}
	return nil
}

var errRunReaderNotConfigured = errors.New("evaluation run reader is not configured")
var errRegistryReadNotConfigured = errors.New("evaluation registry reader is not configured")
var errRunStillGoing = errors.New("evaluation: the run has not finished")

func (s *Service) queries() *gen.Queries { return gen.New(s.Pool) }

func (s *Service) RedeliverEvaluation(ctx context.Context, workspaceID, runID pgtype.UUID) error {
	return s.recoverAttempt(ctx, workspaceID, runID)
}

func (s *Service) HasCurrentEvaluation(ctx context.Context, workspaceID, runID pgtype.UUID) (bool, error) {
	if s == nil || s.Pool == nil {
		return false, errors.New("evaluation persistence is not configured")
	}
	_, found, err := currentEvaluation(ctx, s.queries(), workspaceID, runID)
	return found, err
}

func currentEvaluation(ctx context.Context, q *gen.Queries, workspaceID, runID pgtype.UUID) (gen.Evaluation, bool, error) {
	row, err := q.GetCurrentEvaluation(ctx, gen.GetCurrentEvaluationParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, false, nil
	}
	return row, err == nil, err
}

type evaluationRef struct {
	workspaceID  pgtype.UUID
	evaluationID pgtype.UUID
	runID        pgtype.UUID
}

type modelCall struct {
	model         string
	promptVersion string
	usage         *modelport.ModelUsage
}

func (s *Service) recordModelUsage(
	ctx context.Context, q *gen.Queries, ref evaluationRef, operation string, call modelCall,
) error {
	usage := call.usage
	if usage == nil {
		return nil
	}
	cost := usage.ReportedCostUSD()
	return q.RecordEvaluationModelUsage(ctx, gen.RecordEvaluationModelUsageParams{
		EvaluationID:     ref.evaluationID,
		WorkspaceID:      ref.workspaceID,
		Operation:        operation,
		Model:            orUnknown(call.model),
		PromptVersion:    orUnknown(call.promptVersion),
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

	return "skillhub-judge"
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

func (s *Service) Evaluate(ctx context.Context, workspaceID, runID pgtype.UUID) error {
	m, err := s.gather(ctx, workspaceID, runID)
	if errors.Is(err, ErrNotFound) || errors.Is(err, errRunStillGoing) {
		return nil
	}
	if err != nil {
		return err
	}
	if m.AwaitsAnswer() {
		return nil
	}

	current, found, err := currentEvaluation(ctx, s.queries(), workspaceID, runID)
	if err != nil {
		return err
	}
	if found && Status(current.Status).AwaitsTheJudge() {
		return nil
	}

	ev, err := s.begin(ctx, m)
	if errors.Is(err, errEvaluationInProgress) {
		return nil
	}
	if err != nil {
		return err
	}

	findings := checks.Findings(m)

	evidenceComplete := m.EvidenceComplete()

	if len(m.Criteria) == 0 {

		return s.completeAndSuggest(ctx, m, ev, assessment{
			overall: verdict.OverallUndetermined,
			summary: "this run's test case snapshot carries no acceptance criteria, " +
				"so no task verdict could be reached; the checks below still apply",
			results:          []verdict.CriterionResult{},
			findings:         findings,
			evidenceComplete: evidenceComplete,
		})
	}

	plan := checks.PlanCriteria(m)
	if !plan.NeedsJudge() {
		return s.completeAndSuggest(ctx, m, ev, assessment{
			overall: verdict.OverallFrom(plan.Decided()),
			summary: "every acceptance criterion of this run carries a check the platform " +
				"decided from the run's own records, so no model was asked",
			results:          checks.InCriteriaOrder(m.Criteria, plan.Decided()),
			findings:         findings,
			evidenceComplete: evidenceComplete,
		})
	}

	v, err := s.judge(ctx, plan.MaterialForJudge(m), ev)
	if err != nil {

		return s.fail(ctx, m, ev, gatheredEvidence{findings: findings, complete: evidenceComplete}, err)
	}
	v.results = plan.Finish(m, v.results)
	v.overall = verdict.OverallFrom(v.results)
	v.findings = append(findings, v.findings...)
	v.evidenceComplete = evidenceComplete && v.evidenceComplete
	return s.settleJudgement(ctx, m, ev, v)
}

func (s *Service) settleJudgement(ctx context.Context, m facts.Material, ev gen.Evaluation, v assessment) error {
	ref := evaluationRef{workspaceID: ev.WorkspaceID, evaluationID: ev.ID, runID: m.Run.ID}
	s.recordEvalCost(ctx, s.Pool, credit.KindReview, ref, modelCall{model: v.model, promptVersion: v.promptVersion, usage: v.usage})
	return s.completeAndSuggest(ctx, m, ev, v)
}

func (s *Service) recoverAttempt(ctx context.Context, workspaceID, runID pgtype.UUID) error {
	current, found, err := currentEvaluation(ctx, s.queries(), workspaceID, runID)
	switch {
	case err != nil:
		return err
	case !found:
		return s.Evaluate(ctx, workspaceID, runID)
	case !Status(current.Status).AwaitsTheJudge():
		return nil
	}
	return s.recoverEvaluation(ctx, workspaceID, current.ID, runID)
}

func (s *Service) recoverEvaluation(
	ctx context.Context, workspaceID, evaluationID, runID pgtype.UUID,
) error {
	current, found, err := currentEvaluation(ctx, s.queries(), workspaceID, runID)
	if err != nil || !found || current.ID != evaluationID || !Status(current.Status).AwaitsTheJudge() {
		return err
	}
	m, err := s.gather(ctx, workspaceID, runID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	findings := checks.Findings(m)
	return s.fail(ctx, m, current, gatheredEvidence{findings: findings, complete: m.EvidenceComplete()},
		errors.New("the previous evaluation attempt was interrupted before its verdict committed"))
}

func (s *Service) completeAndSuggest(
	ctx context.Context, m facts.Material, ev gen.Evaluation, v assessment,
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

type assessment struct {
	overall          verdict.Overall
	summary          string
	results          []verdict.CriterionResult
	findings         []verdict.Finding
	evidenceComplete bool
	model            string
	promptVersion    string
	rubricVersion    string
	costUSD          *float64

	usage *modelport.ModelUsage
}

func (s *Service) judge(ctx context.Context, m facts.Material, ev gen.Evaluation) (assessment, error) {
	within := s.Budgets.Within(ctx, JudgeBudget)
	callCtx, cancel := context.WithTimeout(ctx, judgeTimeout)
	defer cancel()
	judges := judging.Judges{Judge: s.Judge, PanelEnabled: s.PanelEnabled}
	o, err := judges.Assess(callCtx, m, pgconv.UUIDString(ev.ID), within)
	if err != nil {
		return assessment{}, err
	}
	return assessment{
		summary:          o.Summary,
		results:          o.Results,
		findings:         o.Findings,
		evidenceComplete: o.EvidenceComplete,
		model:            o.Model,
		promptVersion:    o.PromptVersion,
		rubricVersion:    o.RubricVersion,
		costUSD:          o.Usage.ReportedCostUSD(),
		usage:            o.Usage,
	}, nil
}

func (s *Service) gather(ctx context.Context, workspaceID, runID pgtype.UUID) (facts.Material, error) {
	m := facts.Material{}
	if s.ReadEvaluationInput == nil {
		return m, errRunReaderNotConfigured
	}
	if s.ReadVersion == nil || s.ReadSkill == nil || s.ReadRuntimeCompatibility == nil {
		return m, errRegistryReadNotConfigured
	}
	input, err := s.existingEvaluationInput(ctx, workspaceID, runID)
	if err != nil {
		return m, err
	}
	m.Run, m.Artifacts, m.Attempt = input.Run, input.Artifacts, input.LatestAttempt
	m.Absent = input.Absent
	m.EarlierQuestions = input.EarlierQuestions
	if !m.Run.Terminal {
		return m, errRunStillGoing
	}

	if m.Version, err = s.existingVersion(ctx, workspaceID, m.Run.SkillVersionID); err != nil {
		return m, err
	}
	if m.Skill, _, err = s.ReadSkill(ctx, workspaceID, m.Version.SkillID); err != nil {
		return m, err
	}
	if err := s.gatherTestCase(ctx, workspaceID, &m); err != nil {
		return m, err
	}

	if m.Advanced, err = s.Trace.AdvancedAll(ctx, workspaceID, runID); err != nil {
		return m, err
	}
	if m.Summary, err = s.Trace.General(ctx, workspaceID, runID); err != nil {
		return m, err
	}
	compat, found, err := s.ReadRuntimeCompatibility(ctx, m.Run.SkillVersionID)
	if err != nil {
		return m, err
	}
	if found {
		m.Compat = &compat
	}

	m.Report, m.ReportOK = s.packageReport(ctx, m.Version.Stored())
	return m, nil
}

func (s *Service) existingEvaluationInput(ctx context.Context, workspaceID, runID pgtype.UUID) (facts.EvaluationInput, error) {
	return required(ctx, s.ReadEvaluationInput, workspaceID, runID)
}

func (s *Service) gatherTestCase(ctx context.Context, workspaceID pgtype.UUID, m *facts.Material) error {
	if err := s.requireTestLab(); err != nil {
		return err
	}
	var err error
	if m.Snapshot, err = s.TestLab.ReadSnapshot(ctx, workspaceID, m.Run.TestCaseSnapshotID); err != nil {
		return err
	}

	if m.Criteria, err = testlab.DecodeCriteria(m.Snapshot.AcceptanceCriteria); err != nil {
		return err
	}
	if m.Graders, err = graders.Versions(ctx, s.Pool, workspaceID, checks.ReferencedGraderVersions(m.Criteria)); err != nil {
		return err
	}
	m.Rubric, err = testlab.DecodeRubric(m.Snapshot.Rubric)
	return err
}

func (s *Service) begin(ctx context.Context, m facts.Material) (gen.Evaluation, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.Evaluation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	// Transaction-scoped advisory lock hashed from the run's key, released
	// automatically on commit or rollback, so concurrent callers for the same
	// run serialize here instead of racing on the current-row check below.
	lockKey := "evaluation:" + pgconv.UUIDString(m.Run.WorkspaceID) + ":" + pgconv.UUIDString(m.Run.ID)
	if _, err := tx.Exec(ctx,
		"SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey,
	); err != nil {
		return gen.Evaluation{}, err
	}
	if err := supersedeCurrent(ctx, tx, q, m.Run.WorkspaceID, m.Run.ID); err != nil {
		return gen.Evaluation{}, err
	}

	declaredModel, declaredPromptVersion := s.declaredJudge()
	next := startEvaluation(m.Run.WorkspaceID, m.Run.ID, EvaluationStarted{
		JudgeModel:         strPtr(declaredModel),
		JudgePromptVersion: strPtr(declaredPromptVersion),
		RubricVersion:      strPtr(rubricVersionOf(m.Rubric)),
	})
	if err := saveEvaluation(ctx, tx, next); err != nil {
		return gen.Evaluation{}, err
	}
	ev := next.row
	if err := trace.RecordOrchestratorEvent(ctx, tx, trace.OrchestratorEvent{
		WorkspaceID: m.Run.WorkspaceID, RunID: m.Run.ID, Attempt: m.Attempt,
		Type: trace.TypeEvaluationStarted, Status: "ok", Payload: map[string]any{
			trace.PayloadEvaluationID: pgconv.UUIDString(ev.ID),
			"judge_model":             s.judgeModel(),
			"judge_prompt_version":    s.judgePromptVersion(),

			"rubric_version": rubricVersion(m.Rubric),
		},
	}); err != nil {
		return gen.Evaluation{}, err
	}
	return ev, tx.Commit(ctx)
}

func (s *Service) complete(ctx context.Context, m facts.Material, ev gen.Evaluation, v assessment) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	if err := settleEvaluation(ctx, tx, q, ev, func(e *Evaluation) { e.Complete(v) }); err != nil {
		return err
	}

	ref := evaluationRef{workspaceID: ev.WorkspaceID, evaluationID: ev.ID, runID: m.Run.ID}
	call := modelCall{model: v.model, promptVersion: v.promptVersion, usage: v.usage}
	if err := s.recordModelUsage(ctx, q, ref, "judge", call); err != nil {
		return err
	}

	passed, failed, undetermined := verdict.Tally(v.results)
	if err := trace.RecordOrchestratorEvent(ctx, tx, trace.OrchestratorEvent{
		WorkspaceID: m.Run.WorkspaceID, RunID: m.Run.ID, Attempt: m.Attempt,
		Type: trace.TypeEvaluationCompleted, Status: "ok", Payload: map[string]any{
			trace.PayloadEvaluationID: pgconv.UUIDString(ev.ID),
			"overall":                 v.overall,
			"criteria_total":          len(v.results),
			"criteria_passed":         passed,
			"criteria_failed":         failed,
			"criteria_undetermined":   undetermined,
			"evidence_complete":       v.evidenceComplete,
			"cost_usd":                v.costUSD,
			"failure_reason":          nil,
		},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type gatheredEvidence struct {
	findings []verdict.Finding
	complete bool
}

func (s *Service) fail(ctx context.Context, m facts.Material, ev gen.Evaluation, gathered gatheredEvidence, cause error) error {
	findings, evidenceComplete := gathered.findings, gathered.complete

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

	if err := trace.RecordOrchestratorEvent(ctx, tx, trace.OrchestratorEvent{
		WorkspaceID: m.Run.WorkspaceID, RunID: m.Run.ID, Attempt: m.Attempt,
		Type: trace.TypeEvaluationCompleted, Status: "error", Payload: map[string]any{
			trace.PayloadEvaluationID: pgconv.UUIDString(ev.ID),
			"overall":                 verdict.OverallUndetermined,
			"criteria_total":          len(m.Criteria),
			"criteria_passed":         0,
			"criteria_failed":         0,
			"criteria_undetermined":   len(m.Criteria),
			"evidence_complete":       evidenceComplete,
			"cost_usd":                nil,
			"failure_reason":          reason,
		},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) packageReport(ctx context.Context, stored skillpkg.StoredSkill) (skillpkg.Report, bool) {
	if stored.ObjectKey == "" {
		return skillpkg.Report{}, false
	}
	fsys, _, err := s.readPackage(ctx, stored)
	if err != nil {
		return skillpkg.Report{}, false
	}
	return skillpkg.Validate(fsys), true
}

func (s *Service) runFacts(ctx context.Context, workspaceID, runID pgtype.UUID) (facts.Run, error) {
	if s.ReadRunFacts == nil {
		return facts.Run{}, errRunReaderNotConfigured
	}
	return required(ctx, s.ReadRunFacts, workspaceID, runID)
}

func nonNilFindings(f []verdict.Finding) []verdict.Finding {
	if f == nil {
		return []verdict.Finding{}
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

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

func cut(s string, limit int) (string, bool) {
	runes := []rune(s)
	if len(runes) <= limit {
		return s, false
	}
	return string(runes[:limit]), true
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
