package eval

import (
	"context"
	"math"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type TrialOutcome string

const (
	TrialPending        TrialOutcome = "pending"
	TrialRunning        TrialOutcome = "running"
	TrialAwaitingAnswer TrialOutcome = "awaiting_answer"
	TrialMet            TrialOutcome = "met"
	TrialPartiallyMet   TrialOutcome = "partially_met"
	TrialNotMet         TrialOutcome = "not_met"
	TrialUndetermined   TrialOutcome = "undetermined"
	TrialNotEvaluated   TrialOutcome = "not_evaluated"
)

func (o TrialOutcome) judged() bool {
	return o == TrialMet || o == TrialPartiallyMet || o == TrialNotMet
}

const (
	normalZ95 = 1.959963984540054
	runFailed = "failed"
)

type TrialResult struct {
	Trial    int
	RunID    pgtype.UUID
	EndRunID pgtype.UUID
	Outcome  TrialOutcome
}

type TestCaseResult struct {
	TestCaseID         pgtype.UUID
	Trials             []TrialResult
	Passed             int
	Judged             int
	PassAtK            *bool
	PassAllK           *bool
	PromptVariedAcross bool
}

type Interval struct {
	Low  float64
	High float64
}

type ArmReport struct {
	TestCases        []TestCaseResult
	Passed           int
	Judged           int
	PassRate         *float64
	PassRate95       *Interval
	TestCasesAllK    int
	TestCasesDecided int
}

type BatchReport struct {
	Batch Batch
	ArmReport
	WithoutSkill *ArmReport
	SkillEffect  *PairedDifference
}

func (s *BatchService) BatchReport(ctx context.Context, workspaceID, skillID, batchID pgtype.UUID) (BatchReport, error) {
	if err := s.Ports.ready(); err != nil {
		return BatchReport{}, err
	}
	batch, err := s.batch(ctx, workspaceID, batchID)
	if err != nil {
		return BatchReport{}, err
	}
	if batch.SkillID != skillID {
		return BatchReport{}, ErrNotFound
	}
	items, err := s.queries().ListEvaluationBatchItems(ctx, gen.ListEvaluationBatchItemsParams{
		BatchID: batchID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return BatchReport{}, err
	}
	outcomes, err := s.trialOutcomes(ctx, workspaceID, items)
	if err != nil {
		return BatchReport{}, err
	}
	withSkill, withheld := splitByArm(items)
	report := BatchReport{Batch: batch, ArmReport: armReport(withSkill, outcomes, batch.Trials)}
	if batch.WithBaseline {
		without := armReport(withheld, outcomes, batch.Trials)
		effect := pairArms(report.ArmReport, without)
		report.WithoutSkill, report.SkillEffect = &without, &effect
	}
	return report, nil
}

func splitByArm(items []gen.EvaluationBatchItem) (withSkill, withheld []gen.EvaluationBatchItem) {
	for _, item := range items {
		if item.SkillWithheld {
			withheld = append(withheld, item)
		} else {
			withSkill = append(withSkill, item)
		}
	}
	return withSkill, withheld
}

func armReport(items []gen.EvaluationBatchItem, outcomes map[pgtype.UUID]resolvedTrial, trials int) ArmReport {
	var arm ArmReport
	for _, group := range groupByTestCase(items) {
		arm.add(testCaseResult(group, outcomes, trials))
	}
	arm.PassRate, arm.PassRate95 = passRate(arm.Passed, arm.Judged)
	return arm
}

type resolvedTrial struct {
	end     ChainEnd
	outcome TrialOutcome
	prompt  string
}

func (s *BatchService) trialOutcomes(ctx context.Context, workspaceID pgtype.UUID, items []gen.EvaluationBatchItem) (map[pgtype.UUID]resolvedTrial, error) {
	var runIDs []pgtype.UUID
	for _, item := range items {
		if item.RunID.Valid {
			runIDs = append(runIDs, item.RunID)
		}
	}
	resolved := make(map[pgtype.UUID]resolvedTrial, len(runIDs))
	if len(runIDs) == 0 {
		return resolved, nil
	}
	ends, err := s.Ports.ReadChainEnds(ctx, workspaceID, runIDs)
	if err != nil {
		return nil, err
	}
	endIDs := make([]pgtype.UUID, 0, len(ends))
	snapshotIDs := make([]pgtype.UUID, 0, len(ends))
	for _, end := range ends {
		endIDs = append(endIDs, end.RunID)
		snapshotIDs = append(snapshotIDs, end.StartSnapshotID)
	}
	evaluations, err := s.queries().ListCurrentEvaluations(ctx, gen.ListCurrentEvaluationsParams{
		WorkspaceID: workspaceID, RunIds: endIDs,
	})
	if err != nil {
		return nil, err
	}
	verdicts := make(map[pgtype.UUID]gen.ListCurrentEvaluationsRow, len(evaluations))
	for _, row := range evaluations {
		verdicts[row.RunID] = row
	}
	prompts, err := s.Ports.ReadSnapshotContents(ctx, workspaceID, snapshotIDs)
	if err != nil {
		return nil, err
	}
	for start, end := range ends {
		evaluation, evaluated := verdicts[end.RunID]
		outcome, err := s.trialOutcome(ctx, workspaceID, end, evaluation, evaluated)
		if err != nil {
			return nil, err
		}
		resolved[start] = resolvedTrial{end: end, outcome: outcome, prompt: prompts[end.StartSnapshotID]}
	}
	return resolved, nil
}

func (s *BatchService) trialOutcome(
	ctx context.Context, workspaceID pgtype.UUID, end ChainEnd, evaluation gen.ListCurrentEvaluationsRow, evaluated bool,
) (TrialOutcome, error) {
	switch {
	case !end.Terminal:
		return TrialRunning, nil
	case evaluated && Status(evaluation.Status) == StatusCompleted:
		return TrialOutcome(evaluation.Overall), nil
	case evaluated && Status(evaluation.Status) == StatusFailed:
		return TrialNotEvaluated, nil
	case evaluated:
		return TrialRunning, nil
	case end.Status == runSucceeded:
		asked, err := s.Ports.ReadEndedWithQuestion(ctx, workspaceID, end.RunID)
		if err != nil || !asked {
			return TrialRunning, err
		}
		return TrialAwaitingAnswer, nil
	case end.Status == runFailed:
		return TrialRunning, nil
	default:
		return TrialNotEvaluated, nil
	}
}

func groupByTestCase(items []gen.EvaluationBatchItem) [][]gen.EvaluationBatchItem {
	var groups [][]gen.EvaluationBatchItem
	for _, item := range items {
		last := len(groups) - 1
		if last >= 0 && groups[last][0].TestCaseID == item.TestCaseID {
			groups[last] = append(groups[last], item)
			continue
		}
		groups = append(groups, []gen.EvaluationBatchItem{item})
	}
	return groups
}

func testCaseResult(group []gen.EvaluationBatchItem, resolved map[pgtype.UUID]resolvedTrial, trials int) TestCaseResult {
	result := TestCaseResult{TestCaseID: group[0].TestCaseID}
	prompts := map[string]bool{}
	for _, item := range group {
		trial := TrialResult{Trial: int(item.Trial), RunID: item.RunID, Outcome: TrialPending}
		if r, ok := resolved[item.RunID]; item.RunID.Valid && ok {
			trial.EndRunID, trial.Outcome = r.end.RunID, r.outcome
			prompts[r.prompt] = true
		}
		if trial.Outcome.judged() {
			result.Judged++
		}
		if trial.Outcome == TrialMet {
			result.Passed++
		}
		result.Trials = append(result.Trials, trial)
	}
	result.PromptVariedAcross = len(prompts) > 1
	result.PassAtK = passAtK(result.Passed, result.Judged, trials)
	result.PassAllK = passAllK(result.Passed, result.Judged, trials)
	return result
}

func passAtK(passed, judged, trials int) *bool {
	switch {
	case passed > 0:
		return new(true)
	case judged == trials:
		return new(false)
	default:
		return nil
	}
}

func passAllK(passed, judged, trials int) *bool {
	switch {
	case judged > passed:
		return new(false)
	case judged == trials:
		return new(true)
	default:
		return nil
	}
}

func (r *ArmReport) add(result TestCaseResult) {
	r.TestCases = append(r.TestCases, result)
	r.Passed += result.Passed
	r.Judged += result.Judged
	if result.PassAllK != nil {
		r.TestCasesDecided++
		if *result.PassAllK {
			r.TestCasesAllK++
		}
	}
}

func passRate(passed, judged int) (*float64, *Interval) {
	if judged == 0 {
		return nil, nil
	}
	n, p := float64(judged), float64(passed)/float64(judged)
	// Wilson score interval: the normal approximation recentred and shrunk toward 1/2,
	// so it stays inside [0,1] and keeps its coverage at small n or p near 0 or 1.
	z2 := normalZ95 * normalZ95
	denominator := 1 + z2/n
	centre := (p + z2/(2*n)) / denominator
	half := normalZ95 * math.Sqrt(p*(1-p)/n+z2/(4*n*n)) / denominator
	return &p, &Interval{Low: centre - half, High: centre + half}
}
