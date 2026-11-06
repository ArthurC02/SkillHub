package reflection

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/textx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/modelport"
)

const (
	maxRunsRead    = 8
	headerReserve  = 800
	maxRoundsShown = 20
)

var (
	ErrInvalid          = errors.New("reflection request is invalid")
	ErrNotReady         = errors.New("開發集裡還有試跑沒有跑完或還沒評估完，等它們都有結果再找改進。")
	ErrCreditThreshold  = errors.New("點數不足，無法開始找改進，沒有呼叫模型。請聯絡管理者為這個帳號加點。")
	ErrModelCallsHalted = errors.New("平台暫停中，這次沒有呼叫模型、也沒有扣點。請稍後再試。")
	ErrBudgetExhausted  = errors.New("平台的模型總額度暫時用完，這次沒有扣點。請在台北時間每天上午 8 點額度恢復後再試。")

	errNotConfigured = errors.New("batch reflection is not configured")
)

type BatchTrials struct {
	SkillID   pgtype.UUID
	VersionID pgtype.UUID
	Launching bool
	TestCases []BatchTestCaseTrials
}

type BatchTestCaseTrials struct {
	TestCaseID pgtype.UUID
	Trials     []BatchTrial
}

type BatchTrial struct {
	EndRunID pgtype.UUID
	Settled  bool
}

type Billing struct {
	ModelCallsHalted func(ctx context.Context) bool
	CanStart         func(ctx context.Context, workspaceID pgtype.UUID) (bool, error)
	Charge           func(ctx context.Context, workspaceID pgtype.UUID, usage *modelport.ModelUsage) error
}

func (b Billing) admit(ctx context.Context, workspaceID pgtype.UUID) error {
	if b.ModelCallsHalted != nil && b.ModelCallsHalted(ctx) {
		return ErrModelCallsHalted
	}
	if b.CanStart == nil {
		return nil
	}
	ok, err := b.CanStart(ctx, workspaceID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrCreditThreshold
	}
	return nil
}

func (b Billing) charge(ctx context.Context, workspaceID pgtype.UUID, usage *modelport.ModelUsage) {
	if b.Charge == nil {
		return
	}
	if err := b.Charge(context.WithoutCancel(ctx), workspaceID, usage); err != nil {
		slog.Error("reflection: batch reflection not charged", "error", err)
	}
}

type Service struct {
	Eval            *eval.Service
	ReadBatchTrials func(ctx context.Context, workspaceID, batchID pgtype.UUID) (BatchTrials, bool, error)
	Billing         Billing
}

type Ask struct {
	Actor, SkillID, BatchID pgtype.UUID
	DevTestCaseIDs          []pgtype.UUID
}

func (s *Service) Reflect(ctx context.Context, ws identity.Workspace, ask Ask) (eval.ReflectionRecord, error) {
	if s.Eval == nil || s.ReadBatchTrials == nil || s.Eval.ReadSkill == nil {
		return eval.ReflectionRecord{}, errNotConfigured
	}
	batch, err := s.batchOfSkill(ctx, ws.ID, ask.SkillID, ask.BatchID)
	if err != nil {
		return eval.ReflectionRecord{}, err
	}
	dev, err := devTrials(batch, ask.DevTestCaseIDs)
	if err != nil {
		return eval.ReflectionRecord{}, err
	}
	skill, found, err := s.Eval.ReadSkill(ctx, ws.ID, ask.SkillID)
	if err != nil {
		return eval.ReflectionRecord{}, err
	}
	if !found {
		return eval.ReflectionRecord{}, eval.ErrNotFound
	}
	round := eval.NewReflection{
		WorkspaceID: ws.ID, Actor: ask.Actor, BatchID: ask.BatchID, SkillVersionID: batch.VersionID,
		DevTestCaseIDs: ask.DevTestCaseIDs,
	}
	if skill.AccessRestricted {
		round.Outcome = eval.SuggestionsRestricted
		return eval.RecordReflection(ctx, s.Eval, round)
	}
	failing, err := s.failingRuns(ctx, ws.ID, dev)
	if err != nil {
		return eval.ReflectionRecord{}, err
	}
	if len(failing) == 0 {
		round.Outcome = eval.SuggestionsNotNeeded
		return eval.RecordReflection(ctx, s.Eval, round)
	}
	if err := s.reflect(ctx, &round, failing); err != nil {
		return eval.ReflectionRecord{}, err
	}
	return eval.RecordReflection(ctx, s.Eval, round)
}

func (s *Service) batchOfSkill(ctx context.Context, workspaceID, skillID, batchID pgtype.UUID) (BatchTrials, error) {
	batch, found, err := s.ReadBatchTrials(ctx, workspaceID, batchID)
	if err != nil {
		return BatchTrials{}, err
	}
	if !found || batch.SkillID != skillID {
		return BatchTrials{}, eval.ErrNotFound
	}
	return batch, nil
}

func devTrials(batch BatchTrials, devIDs []pgtype.UUID) ([][]BatchTrial, error) {
	if batch.Launching {
		return nil, ErrNotReady
	}
	byID := make(map[pgtype.UUID][]BatchTrial, len(batch.TestCases))
	for _, tc := range batch.TestCases {
		byID[tc.TestCaseID] = tc.Trials
	}
	seen := make(map[pgtype.UUID]bool, len(devIDs))
	for _, id := range devIDs {
		if _, ok := byID[id]; !ok || seen[id] {
			return nil, fmt.Errorf("%w: 開發集只能放這一批裡的測試題，而且每題只放一次", ErrInvalid)
		}
		seen[id] = true
	}
	switch {
	case len(devIDs) == 0:
		return nil, fmt.Errorf("%w: 開發集至少要放一題", ErrInvalid)
	case len(devIDs) >= len(byID):
		return nil, fmt.Errorf("%w: 至少要留一題不給模型看，之後用它檢查改進有沒有用", ErrInvalid)
	}
	out := make([][]BatchTrial, 0, len(devIDs))
	for _, tc := range batch.TestCases {
		if !seen[tc.TestCaseID] {
			continue
		}
		for _, trial := range tc.Trials {
			if !trial.Settled {
				return nil, ErrNotReady
			}
		}
		out = append(out, tc.Trials)
	}
	return out, nil
}

func (s *Service) failingRuns(ctx context.Context, workspaceID pgtype.UUID, dev [][]BatchTrial) ([]eval.RunFailure, error) {
	var out []eval.RunFailure
	for _, runID := range readingOrder(dev) {
		failure, worth, err := eval.FailureOfRun(ctx, s.Eval, workspaceID, runID)
		if err != nil {
			return nil, err
		}
		if worth {
			out = append(out, failure)
		}
	}
	return out, nil
}

func readingOrder(dev [][]BatchTrial) []pgtype.UUID {
	var out []pgtype.UUID
	for round := 0; ; round++ {
		more := false
		for _, trials := range dev {
			if round < len(trials) {
				more = true
				out = append(out, trials[round].EndRunID)
			}
		}
		if !more {
			return out
		}
	}
}

func readable(failing []eval.RunFailure) (read []eval.RunFailure, notRead int) {
	read = failing[:min(len(failing), maxRunsRead)]
	return read, len(failing) - len(read)
}

func (s *Service) reflect(ctx context.Context, round *eval.NewReflection, failing []eval.RunFailure) error {
	read, notRead := readable(failing)
	round.RunsRead, round.RunsNotRead = len(read), notRead
	reading, err := eval.ReadFailures(ctx, s.Eval, round.WorkspaceID, read, (eval.MaxDigestChars-headerReserve)/len(read))
	if err != nil {
		return err
	}
	round.Refs = reading.Refs
	if len(reading.Refs) == 0 {
		round.Outcome = eval.SuggestionsNoEvidence
		return nil
	}
	if err := s.Billing.admit(ctx, round.WorkspaceID); err != nil {
		return err
	}
	budget := eval.SuggestImprovementsBudget
	callCtx, cancel := context.WithTimeout(ctx, budget.Deadline)
	defer cancel()
	resp, err := s.Eval.Suggester.SuggestImprovements(callCtx, modelport.ImprovementRequest{
		EvaluationID:     "batch:" + pgconv.UUIDString(round.BatchID),
		EvaluationDigest: digest(round.RunsRead, round.RunsNotRead, len(round.DevTestCaseIDs), reading.Sections),
		FileTree:         reading.FileTree,
		TargetFiles:      reading.TargetFiles,
		Within:           s.Eval.Budgets.Within(ctx, budget),
	})
	if errors.Is(err, modelport.ErrModelBudgetExhausted) {
		return ErrBudgetExhausted
	}
	if err != nil {
		slog.Warn("batch reflection unavailable", "batch_id", pgconv.UUIDString(round.BatchID), "error", err)
		round.Outcome = eval.SuggestionsUnavailable
		return nil
	}
	s.Billing.charge(ctx, round.WorkspaceID, resp.Usage)
	round.Proposals, round.Outcome = resp.Proposals, eval.SuggestionsNoneProposed
	return nil
}

func digest(runsRead, notRead, devCases int, sections []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "This round reads %d failing runs from %d development test cases of one evaluation batch, "+
		"all against the same package version. Look for problems that recur across runs before one-off ones.\n", runsRead, devCases)
	if notRead > 0 {
		fmt.Fprintf(&b, "%d further failing runs were not read, to stay within the request size; this reading is partial.\n", notRead)
	}
	b.WriteString("Each run's section may be cut where it ends with …[truncated].\n")
	for i, section := range sections {
		fmt.Fprintf(&b, "\n## failing run %d of %d\n%s", i+1, len(sections), section)
	}
	out, _ := textx.CutRunes(b.String(), eval.MaxDigestChars)
	return out
}

func (s *Service) List(ctx context.Context, workspaceID, skillID, batchID pgtype.UUID) ([]eval.ReflectionRecord, error) {
	if s.Eval == nil || s.ReadBatchTrials == nil {
		return nil, errNotConfigured
	}
	if _, err := s.batchOfSkill(ctx, workspaceID, skillID, batchID); err != nil {
		return nil, err
	}
	return eval.ListReflections(ctx, s.Eval, workspaceID, batchID, maxRoundsShown)
}
