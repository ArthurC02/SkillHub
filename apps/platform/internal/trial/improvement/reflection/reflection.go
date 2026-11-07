package reflection

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

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
	ErrRoundCapUnset    = errors.New("管理者還沒設定每輪找改進的點數上限（reflection_round_credit_cap），一輪找多個候選無法開始，沒有呼叫模型、也沒有扣點。")
	ErrRoundOverCap     = errors.New("這一輪預估的點數超過管理者設定的每輪上限，沒有呼叫模型、也沒有扣點。請管理者調高上限或減少候選數。")

	errNotConfigured = errors.New("batch reflection is not configured")
)

type BatchTrials struct {
	SkillID           pgtype.UUID
	VersionID         pgtype.UUID
	Launching         bool
	TestCases         []BatchTestCaseTrials
	Trials            int
	WithSkillWithheld bool
}

type BatchTestCaseTrials struct {
	TestCaseID pgtype.UUID
	Trials     []BatchTrial
}

type BatchTrial struct {
	EndRunID pgtype.UUID
	Settled  bool
}

type RoundPolicy struct {
	Candidates     int
	RoundCreditCap *int64
}

type StartQuote struct {
	Balance, Threshold int64
}

type Billing struct {
	ModelCallsHalted func(ctx context.Context) bool
	Quote            func(ctx context.Context, workspaceID pgtype.UUID) (StartQuote, error)
	Charge           func(ctx context.Context, workspaceID pgtype.UUID, usage *modelport.ModelUsage) error
}

type estimator struct {
	quote func(ctx context.Context, workspaceID pgtype.UUID) (StartQuote, error)
	units int64
	short error
}

func (b Billing) admit(ctx context.Context, workspaceID pgtype.UUID, policy RoundPolicy) error {
	return b.admitEstimate(ctx, workspaceID, policy, estimator{quote: b.Quote, units: int64(policy.Candidates), short: ErrCreditThreshold})
}

func (b Billing) admitEstimate(ctx context.Context, workspaceID pgtype.UUID, policy RoundPolicy, e estimator) error {
	if b.ModelCallsHalted != nil && b.ModelCallsHalted(ctx) {
		return ErrModelCallsHalted
	}
	if policy.Candidates > 1 && policy.RoundCreditCap == nil {
		return ErrRoundCapUnset
	}
	if e.quote == nil {
		return nil
	}
	quote, err := e.quote(ctx, workspaceID)
	if err != nil {
		return err
	}
	estimate := quote.Threshold * e.units
	if policy.RoundCreditCap != nil && estimate > *policy.RoundCreditCap {
		return ErrRoundOverCap
	}
	if quote.Balance < estimate {
		return e.short
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
	Policy          func(ctx context.Context) (RoundPolicy, error)
	Candidates      CandidatePorts
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
	if err := s.requireLineNewest(ctx, ws.ID, ask.SkillID, batch.VersionID); err != nil {
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

var candidateFocuses = [...]string{"instructions", "scripts_and_tools", "references_and_examples", "free"}

func roundFocuses(candidates int) []string {
	if candidates <= 1 {
		return []string{""}
	}
	return candidateFocuses[:min(candidates, len(candidateFocuses))]
}

func (s *Service) policy(ctx context.Context) (RoundPolicy, error) {
	if s.Policy == nil {
		return RoundPolicy{Candidates: 1}, nil
	}
	return s.Policy(ctx)
}

type candidateCall struct {
	resp *modelport.Improvements
	err  error
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
	policy, err := s.policy(ctx)
	if err != nil {
		return err
	}
	if err := s.Billing.admit(ctx, round.WorkspaceID, policy); err != nil {
		return err
	}
	focuses := roundFocuses(policy.Candidates)
	calls := make([]candidateCall, len(focuses))
	budget := eval.SuggestImprovementsBudget
	callCtx, cancel := context.WithTimeout(ctx, budget.Deadline)
	defer cancel()
	request := modelport.ImprovementRequest{
		EvaluationID:     "batch:" + pgconv.UUIDString(round.BatchID),
		EvaluationDigest: digest(round.RunsRead, round.RunsNotRead, len(round.DevTestCaseIDs), reading.Sections),
		FileTree:         reading.FileTree,
		TargetFiles:      reading.TargetFiles,
		Within:           s.Eval.Budgets.Within(ctx, budget),
	}
	var wg sync.WaitGroup
	for i, focus := range focuses {
		wg.Go(func() {
			req := request
			req.Focus = focus
			calls[i].resp, calls[i].err = s.Eval.Suggester.SuggestImprovements(callCtx, req)
		})
	}
	wg.Wait()
	return s.settleRound(ctx, round, focuses, calls)
}

func (s *Service) settleRound(ctx context.Context, round *eval.NewReflection, focuses []string, calls []candidateCall) error {
	var answered []*modelport.ModelUsage
	exhausted := false
	for i, call := range calls {
		candidate := eval.ReflectionCandidate{Focus: focuses[i]}
		switch {
		case errors.Is(call.err, modelport.ErrModelBudgetExhausted):
			exhausted = true
		case call.err != nil:
			slog.Warn("batch reflection unavailable", "batch_id", pgconv.UUIDString(round.BatchID), "focus", focuses[i], "error", call.err)
		default:
			candidate.Answered, candidate.Proposals = true, call.resp.Proposals
			answered = append(answered, call.resp.Usage)
		}
		round.Candidates = append(round.Candidates, candidate)
	}
	if len(answered) == 0 {
		if exhausted {
			return ErrBudgetExhausted
		}
		return nil
	}
	s.Billing.charge(ctx, round.WorkspaceID, roundUsage(answered))
	return nil
}

func roundUsage(usages []*modelport.ModelUsage) *modelport.ModelUsage {
	var sum modelport.ModelUsage
	cost, reported, seen := 0.0, true, false
	var calls []float64
	for _, u := range usages {
		if u == nil {
			reported = false
			continue
		}
		seen = true
		sum.PromptTokens += u.PromptTokens
		sum.CompletionTokens += u.CompletionTokens
		if usd := u.ReportedCostUSD(); usd != nil {
			cost += *usd
			calls = append(calls, *usd)
		} else {
			reported = false
		}
	}
	if !seen {
		return nil
	}
	if reported {
		sum.CostUSD, sum.CostReported = &cost, true
		if len(calls) > 1 {
			sum.CallCostsUSD = calls
		}
	}
	return &sum
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
