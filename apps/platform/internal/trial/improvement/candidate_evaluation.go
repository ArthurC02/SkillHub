package eval

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
)

var (
	ErrRoundVersionSuperseded = errors.New("這一輪讀的版本已經不是這個小工具正式路線上的最新版，候選是對舊內容做的改法；請先用最新版跑一批評估、再找改進")
	ErrCandidateOutOfRange    = errors.New("candidate index is not below the number of candidates the round asked for")
	ErrAlreadyAdopted         = errors.New("這一輪已經採用了另一個候選；一輪只能採用一個，下一輪請從採用的那一版開始")
	ErrCandidateUnavailable   = errors.New("這個候選沒有可以套用的建議，所以沒有建出版本，不能採用")
)

type RoundCandidate struct {
	SkillID, ReflectionID pgtype.UUID
	Candidate             int
	Actor                 pgtype.UUID
}

type CandidateBuild struct {
	Candidate int
	VersionID pgtype.UUID
	Skipped   []pgtype.UUID
}

func (b CandidateBuild) Unavailable() bool { return !b.VersionID.Valid }

func CandidateCount(row gen.BatchReflection) int {
	return max(len(row.CandidateFocuses), 1)
}

func (s *Service) BuildCandidate(ctx context.Context, ws identity.Workspace, c RoundCandidate) (CandidateBuild, error) {
	skillID, reflectionID, candidate, actor := c.SkillID, c.ReflectionID, c.Candidate, c.Actor
	build := CandidateBuild{Candidate: candidate}
	if s.Versions == nil || s.store() == nil {
		return build, errNoStore
	}
	parent := SuggestionParent{ReflectionID: reflectionID}
	base, err := s.evaluatedSkill(ctx, ws.ID, skillID, parent)
	if err != nil {
		return build, err
	}
	if base.origin.ID != base.latest.ID {
		return build, ErrRoundVersionSuperseded
	}
	chosen, rejected, err := s.acceptCandidate(ctx, ws.ID, reflectionID, candidate, actor)
	build.Skipped = rejected
	if err != nil || len(chosen) == 0 {
		return build, err
	}
	sortByTarget(chosen)
	plan := planPatches(base, chosen, len(chosen))
	build.Skipped = append(build.Skipped, notApplied(chosen, plan.applied)...)
	if len(plan.patches) == 0 {
		return build, nil
	}
	if validatePatched(base, plan.patches, plan.appliedIDs) != nil {
		build.Skipped = append(build.Skipped, plan.applied...)
		return build, nil
	}
	patched, err := patchArchive(base.latestZip, base.latest.SourcePath, plan.patches)
	if err != nil {
		return build, err
	}
	res, err := s.Versions.SaveImprovedVersion(ctx, ws, skillID, patched, ingest.Improvement{
		ReflectionID: reflectionID, SuggestionIDs: plan.applied, BaseVersionID: base.latest.ID, HeldForEvaluation: true,
	})
	if errors.Is(err, ingest.ErrBaseVersionMoved) {
		return build, ErrRoundVersionSuperseded
	}
	if err != nil {
		return build, err
	}
	if res.Report.Blocked {
		build.Skipped = append(build.Skipped, plan.applied...)
		return build, nil
	}
	build.VersionID = res.Version.ID
	return build, nil
}

func (s *Service) acceptCandidate(
	ctx context.Context, workspaceID, reflectionID pgtype.UUID, candidate int, actor pgtype.UUID,
) (chosen []gen.EvaluationSuggestion, rejected []pgtype.UUID, err error) {
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := gen.New(tx)
		rows, err := q.ListReflectionSuggestions(ctx, gen.ListReflectionSuggestionsParams{
			ReflectionIds: []pgtype.UUID{reflectionID}, WorkspaceID: workspaceID,
		})
		if err != nil {
			return err
		}
		var ids []pgtype.UUID
		for _, row := range rows {
			if int(row.Candidate) == candidate {
				ids = append(ids, row.ID)
			}
		}
		r, err := loadReflectionWithSuggestions(ctx, q, workspaceID, reflectionID, ids)
		if err != nil {
			return err
		}
		for _, id := range ids {
			switch Decision(r.suggestions[id].Decision) {
			case DecisionRejected:
				rejected = append(rejected, id)
				continue
			case DecisionPending:
				r.DecideBy(id, DecisionAccepted, actor)
			}
			chosen = append(chosen, r.suggestions[id])
		}
		return saveReflectionUnlessRefused(ctx, tx, r)
	})
	return chosen, rejected, err
}

type CandidateBatch struct {
	Candidate int
	VersionID pgtype.UUID
	BatchID   pgtype.UUID
	Skipped   []pgtype.UUID
}

func (s *Service) RecordCandidateBatch(
	ctx context.Context, workspaceID, reflectionID, actor pgtype.UUID, c CandidateBatch,
) error {
	row, err := s.queries().GetBatchReflection(ctx, gen.GetBatchReflectionParams{ID: reflectionID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if c.Candidate < 0 || c.Candidate >= CandidateCount(row) {
		return ErrCandidateOutOfRange
	}
	return s.queries().InsertReflectionCandidateBatch(ctx, gen.InsertReflectionCandidateBatchParams{
		WorkspaceID: workspaceID, ReflectionID: reflectionID, Candidate: int16(c.Candidate),
		SkillVersionID: c.VersionID, BatchID: c.BatchID, SkippedSuggestionIds: nonNil(c.Skipped), CreatedBy: actor,
	})
}

func nonNil(ids []pgtype.UUID) []pgtype.UUID {
	if ids == nil {
		return []pgtype.UUID{}
	}
	return ids
}

func (s *Service) CandidateBatches(ctx context.Context, workspaceID, reflectionID pgtype.UUID) ([]CandidateBatch, error) {
	rows, err := s.queries().ListReflectionCandidateBatches(ctx, gen.ListReflectionCandidateBatchesParams{
		ReflectionID: reflectionID, WorkspaceID: workspaceID,
	})
	out := make([]CandidateBatch, len(rows))
	for i, row := range rows {
		out[i] = CandidateBatch{
			Candidate: int(row.Candidate), VersionID: row.SkillVersionID, BatchID: row.BatchID, Skipped: row.SkippedSuggestionIds,
		}
	}
	return out, err
}

type Adoption struct {
	Candidate int
	VersionID pgtype.UUID
	AdoptedAt pgtype.Timestamptz
}

type CandidateVersion struct {
	VersionID, ReflectionID, BatchID pgtype.UUID
	Round, Candidate                 int
}

func (s *Service) CandidateVersionsOf(ctx context.Context, workspaceID pgtype.UUID, skillVersions []pgtype.UUID) ([]CandidateVersion, error) {
	rows, err := s.queries().ListCandidateVersionsOfSkill(ctx, gen.ListCandidateVersionsOfSkillParams{
		WorkspaceID: workspaceID, VersionIds: skillVersions,
	})
	if err != nil {
		return nil, err
	}
	out := make([]CandidateVersion, len(rows))
	for i, r := range rows {
		out[i] = CandidateVersion{VersionID: r.VersionID, ReflectionID: r.ReflectionID, BatchID: r.BatchID, Round: int(r.Round), Candidate: int(r.Candidate)}
	}
	return out, nil
}

func (s *Service) AdoptionOf(ctx context.Context, workspaceID, reflectionID pgtype.UUID) (Adoption, bool, error) {
	row, err := s.queries().GetReflectionAdoption(ctx, gen.GetReflectionAdoptionParams{
		ReflectionID: reflectionID, WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Adoption{}, false, nil
	}
	return Adoption{Candidate: int(row.Candidate), VersionID: row.SkillVersionID, AdoptedAt: row.AdoptedAt}, err == nil, err
}

func (s *Service) Adopt(ctx context.Context, ws identity.Workspace, c RoundCandidate) (Adoption, error) {
	skillID, reflectionID, candidate, actor := c.SkillID, c.ReflectionID, c.Candidate, c.Actor
	if adopted, found, err := s.AdoptionOf(ctx, ws.ID, reflectionID); err != nil || found {
		if found && adopted.Candidate != candidate {
			return adopted, ErrAlreadyAdopted
		}
		return adopted, err
	}
	row, err := s.queries().GetBatchReflection(ctx, gen.GetBatchReflectionParams{ID: reflectionID, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Adoption{}, ErrNotFound
	}
	if err != nil {
		return Adoption{}, err
	}
	built, err := s.candidateVersion(ctx, ws.ID, reflectionID, candidate)
	if err != nil {
		return Adoption{}, err
	}
	err = s.Versions.PromoteVersion(ctx, ws, skillID, ingest.Promotion{
		VersionID: built, BaseVersionID: row.SkillVersionID, By: actor,
	})
	if errors.Is(err, ingest.ErrBaseVersionMoved) {
		return Adoption{}, ErrRoundVersionSuperseded
	}
	if err != nil {
		return Adoption{}, err
	}
	if _, err := s.queries().InsertReflectionAdoption(ctx, gen.InsertReflectionAdoptionParams{
		ReflectionID: reflectionID, WorkspaceID: ws.ID, Candidate: int16(candidate), SkillVersionID: built, AdoptedBy: actor,
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Adoption{}, err
	}
	return s.adoptionAfterWrite(ctx, ws.ID, reflectionID, candidate)
}

func (s *Service) candidateVersion(ctx context.Context, workspaceID, reflectionID pgtype.UUID, candidate int) (pgtype.UUID, error) {
	batches, err := s.CandidateBatches(ctx, workspaceID, reflectionID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	for _, b := range batches {
		if b.Candidate == candidate && b.VersionID.Valid {
			return b.VersionID, nil
		}
		if b.Candidate == candidate {
			return pgtype.UUID{}, ErrCandidateUnavailable
		}
	}
	return pgtype.UUID{}, ErrNotFound
}

func (s *Service) adoptionAfterWrite(ctx context.Context, workspaceID, reflectionID pgtype.UUID, candidate int) (Adoption, error) {
	adopted, _, err := s.AdoptionOf(ctx, workspaceID, reflectionID)
	if err == nil && adopted.Candidate != candidate {
		return adopted, ErrAlreadyAdopted
	}
	return adopted, err
}

func notApplied(chosen []gen.EvaluationSuggestion, applied []pgtype.UUID) []pgtype.UUID {
	var out []pgtype.UUID
	for _, sug := range chosen {
		if !slices.Contains(applied, sug.ID) {
			out = append(out, sug.ID)
		}
	}
	return out
}
