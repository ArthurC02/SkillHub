package reflection

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

var ErrEvaluateCreditThreshold = errors.New("點數不足，無法開始評估候選：沒有建版本、也沒有開批次。請聯絡管理者為這個帳號加點。")

type CandidateLaunch struct {
	WorkspaceID, Actor, SkillID, VersionID pgtype.UUID
	TestCaseIDs                            []pgtype.UUID
	Trials                                 int
	WithSkillWithheld                      bool
	OriginKey                              string
}

type CandidateComparison struct {
	Settled bool
	Rates   CaseRates
	Source  CaseRates
	Dev     PairedVerdict
	HeldOut PairedVerdict
}

type CandidatePorts struct {
	Confirmed func(ctx context.Context, workspaceID, skillID, versionID, testCaseID pgtype.UUID) (bool, error)
	Launch    func(ctx context.Context, launch CandidateLaunch) (pgtype.UUID, error)
	RunQuote  func(ctx context.Context, workspaceID pgtype.UUID) (StartQuote, error)
	Compare   func(ctx context.Context, workspaceID, candidateBatch, sourceBatch pgtype.UUID, dev []pgtype.UUID) (CandidateComparison, error)
	Versions  func(ctx context.Context, workspaceID, skillID pgtype.UUID) ([]pgtype.UUID, error)
}

type UnconfirmedPair struct {
	VersionID, TestCaseID pgtype.UUID
}

type UnconfirmedPairsError struct{ Pairs []UnconfirmedPair }

func (e UnconfirmedPairsError) Error() string {
	return fmt.Sprintf("%d candidate test cases need their run permissions confirmed first", len(e.Pairs))
}

type EvaluateAsk struct {
	Actor, SkillID, BatchID, ReflectionID pgtype.UUID
}

func (a EvaluateAsk) candidate(k int) eval.RoundCandidate {
	return eval.RoundCandidate{SkillID: a.SkillID, ReflectionID: a.ReflectionID, Candidate: k, Actor: a.Actor}
}

func (s *Service) EvaluateCandidates(ctx context.Context, ws identity.Workspace, ask EvaluateAsk) error {
	if s.Eval == nil || s.Candidates.Launch == nil || s.Candidates.Confirmed == nil || s.Policy == nil {
		return errNotConfigured
	}
	round, err := s.roundOfBatch(ctx, ws.ID, ask)
	if err != nil {
		return err
	}
	recorded, err := s.Eval.CandidateBatches(ctx, ws.ID, ask.ReflectionID)
	if err != nil || len(recorded) == eval.CandidateCount(round.Row) {
		return err
	}
	source, err := s.batchOfSkill(ctx, ws.ID, ask.SkillID, ask.BatchID)
	if err != nil {
		return err
	}
	evaluable := evaluableCandidates(round)
	if err := s.admitEvaluation(ctx, ws.ID, source, len(evaluable)); err != nil {
		return err
	}
	builds := make([]eval.CandidateBuild, eval.CandidateCount(round.Row))
	for candidate := range builds {
		builds[candidate] = eval.CandidateBuild{Candidate: candidate}
		if !evaluable[candidate] {
			builds[candidate].Skipped = rejectedSuggestions(round, candidate)
			continue
		}
		if builds[candidate], err = s.Eval.BuildCandidate(ctx, ws, ask.candidate(candidate)); err != nil {
			return err
		}
	}
	if err := s.requireConsent(ctx, ws.ID, ask.SkillID, source, builds); err != nil {
		return err
	}
	return s.launchCandidates(ctx, ws.ID, ask, source, builds)
}

func (s *Service) roundOfBatch(ctx context.Context, workspaceID pgtype.UUID, ask EvaluateAsk) (eval.ReflectionRecord, error) {
	rounds, err := eval.ListReflections(ctx, s.Eval, workspaceID, ask.BatchID, maxRoundsShown)
	if err != nil {
		return eval.ReflectionRecord{}, err
	}
	for _, round := range rounds {
		if round.Row.ID == ask.ReflectionID {
			return round, nil
		}
	}
	return eval.ReflectionRecord{}, eval.ErrNotFound
}

func evaluableCandidates(round eval.ReflectionRecord) map[int]bool {
	evaluable := map[int]bool{}
	for _, sug := range round.Suggestions {
		if eval.Decision(sug.Decision) != eval.DecisionRejected {
			evaluable[int(sug.Candidate)] = true
		}
	}
	return evaluable
}

func rejectedSuggestions(round eval.ReflectionRecord, candidate int) []pgtype.UUID {
	var ids []pgtype.UUID
	for _, sug := range round.Suggestions {
		if int(sug.Candidate) == candidate && eval.Decision(sug.Decision) == eval.DecisionRejected {
			ids = append(ids, sug.ID)
		}
	}
	return ids
}

func trialRuns(source BatchTrials) int64 {
	arms := int64(1)
	if source.WithSkillWithheld {
		arms = 2
	}
	return int64(len(source.TestCases)) * int64(source.Trials) * arms
}

func (s *Service) admitEvaluation(ctx context.Context, workspaceID pgtype.UUID, source BatchTrials, candidates int) error {
	if candidates == 0 {
		return nil
	}
	policy, err := s.Policy(ctx)
	if err != nil {
		return err
	}
	policy.Candidates = candidates
	return s.Billing.admitEstimate(ctx, workspaceID, policy, estimator{
		quote: s.Candidates.RunQuote, units: trialRuns(source) * int64(candidates), short: ErrEvaluateCreditThreshold,
	})
}

func (s *Service) requireConsent(
	ctx context.Context, workspaceID, skillID pgtype.UUID, source BatchTrials, builds []eval.CandidateBuild,
) error {
	var missing []UnconfirmedPair
	for _, build := range builds {
		if build.Unavailable() {
			continue
		}
		for _, tc := range source.TestCases {
			confirmed, err := s.Candidates.Confirmed(ctx, workspaceID, skillID, build.VersionID, tc.TestCaseID)
			if err != nil {
				return err
			}
			if !confirmed {
				missing = append(missing, UnconfirmedPair{VersionID: build.VersionID, TestCaseID: tc.TestCaseID})
			}
		}
	}
	if len(missing) > 0 {
		return UnconfirmedPairsError{Pairs: missing}
	}
	return nil
}

func candidateOriginKey(reflectionID pgtype.UUID, candidate int) string {
	return fmt.Sprintf("batch-reflection/%s/candidate/%d", pgconv.UUIDString(reflectionID), candidate)
}

func (s *Service) launchCandidates(
	ctx context.Context, workspaceID pgtype.UUID, ask EvaluateAsk, source BatchTrials, builds []eval.CandidateBuild,
) error {
	testCases := make([]pgtype.UUID, len(source.TestCases))
	for i, tc := range source.TestCases {
		testCases[i] = tc.TestCaseID
	}
	for _, build := range builds {
		record := eval.CandidateBatch{Candidate: build.Candidate, Skipped: build.Skipped}
		if !build.Unavailable() {
			batchID, err := s.Candidates.Launch(ctx, CandidateLaunch{
				WorkspaceID: workspaceID, Actor: ask.Actor, SkillID: ask.SkillID, VersionID: build.VersionID,
				TestCaseIDs: testCases, Trials: source.Trials, WithSkillWithheld: source.WithSkillWithheld,
				OriginKey: candidateOriginKey(ask.ReflectionID, build.Candidate),
			})
			if err != nil {
				return err
			}
			record.VersionID, record.BatchID = build.VersionID, batchID
		}
		if err := s.Eval.RecordCandidateBatch(ctx, workspaceID, ask.ReflectionID, ask.Actor, record); err != nil {
			return err
		}
	}
	return nil
}

type CandidateStanding struct {
	eval.CandidateBatch
	Settled bool
	Kept    bool
	Dev     PairedVerdict
	HeldOut PairedVerdict
}

type CandidateStandings struct {
	Candidates  []CandidateStanding
	Recommended int
	Adopted     *eval.Adoption
}

func (s *Service) Standings(ctx context.Context, workspaceID pgtype.UUID, round eval.ReflectionRecord) (CandidateStandings, error) {
	out := CandidateStandings{Recommended: -1}
	recorded, err := s.Eval.CandidateBatches(ctx, workspaceID, round.Row.ID)
	if err != nil || len(recorded) == 0 || s.Candidates.Compare == nil {
		return out, err
	}
	source := CaseRates{}
	rates := make([]CaseRates, len(recorded))
	for _, batch := range recorded {
		standing := CandidateStanding{CandidateBatch: batch}
		if batch.BatchID.Valid {
			cmp, err := s.Candidates.Compare(ctx, workspaceID, batch.BatchID, round.Row.BatchID, round.Row.DevTestCaseIds)
			if err != nil {
				return out, err
			}
			standing.Settled, standing.Dev, standing.HeldOut = cmp.Settled, cmp.Dev, cmp.HeldOut
			rates[len(out.Candidates)], source = cmp.Rates, cmp.Source
		}
		out.Candidates = append(out.Candidates, standing)
	}
	kept := keptCandidates(source, rates, round.Row.DevTestCaseIds)
	for i := range out.Candidates {
		out.Candidates[i].Kept = kept[i]
	}
	out.Recommended = recommendedCandidate(out.Candidates, eval.CandidateCount(round.Row))
	if adopted, found, err := s.Eval.AdoptionOf(ctx, workspaceID, round.Row.ID); err != nil || found {
		out.Adopted = &adopted
		return out, err
	}
	return out, nil
}

func (s *Service) requireLineNewest(ctx context.Context, workspaceID, skillID, versionID pgtype.UUID) error {
	if s.Eval.ReadLatestVersion == nil {
		return errNotConfigured
	}
	newest, found, err := s.Eval.ReadLatestVersion(ctx, workspaceID, skillID)
	if err != nil {
		return err
	}
	if !found || newest.ID != versionID {
		return eval.ErrRoundVersionSuperseded
	}
	return nil
}

func (s *Service) Adopt(ctx context.Context, ws identity.Workspace, ask EvaluateAsk, candidate int) (eval.Adoption, error) {
	if s.Eval == nil {
		return eval.Adoption{}, errNotConfigured
	}
	if _, err := s.roundOfBatch(ctx, ws.ID, ask); err != nil {
		return eval.Adoption{}, err
	}
	return s.Eval.Adopt(ctx, ws, ask.candidate(candidate))
}
