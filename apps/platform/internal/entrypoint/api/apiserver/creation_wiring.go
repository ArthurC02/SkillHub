package apiserver

import (
	"context"
	"errors"
	"fmt"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	registry "github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"

	"encoding/json"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
	"github.com/jackc/pgx/v5"
)

func wireCreationReads(s *creation.Service, versions *ingest.Service, search *catalog.Service) {
	wiring.WireCreationReferenceReads(s, versions, search)
	s.CatalogCheck = nearestReferences(s, search, catalog.CreationMaxDistance)
	s.DuplicateCheck = nearestReferences(s, search, catalog.CreationDuplicateDistance)
}

func nearestReferences(s *creation.Service, search *catalog.Service, maxDistance float64) func(context.Context, identity.Workspace, string) ([]creation.Reference, float64, error) {
	return func(ctx context.Context, ws identity.Workspace, query string) ([]creation.Reference, float64, error) {
		knowledge, err := search.CreationKnowledgeIDs(ctx, query, maxDistance)
		if err != nil || knowledge.Degraded {
			return nil, knowledge.CostUSD, err
		}
		return s.FirstResolvedReferences(ctx, ws, knowledge.IDs), knowledge.CostUSD, nil
	}
}

func wireCreationWrites(s *creation.Service, versions *ingest.Service, runs *run.Service, evaluations *eval.Service) {
	s.Materialize = func(ctx context.Context, ws identity.Workspace, draft creation.GeneratedSkill, p creation.Provenance, after func(context.Context, pgx.Tx, creation.Candidate) error) error {
		provenance := ingest.GeneratedCandidateProvenance{TaskDescription: p.Brief, Model: p.Model, PromptVersion: p.PromptVersion, GenerationInputs: p.Inputs}
		if p.ExistingSkillID != "" {
			id, err := creation.ParseID(p.ExistingSkillID)
			if err != nil {
				return err
			}
			provenance.ExistingSkillID = &id
		}
		result, err := versions.MaterializeGeneratedCandidate(ctx, ws, wiring.GeneratedSkillForIngest(draft), provenance, func(ctx context.Context, tx pgx.Tx, r ingest.Result) error {
			return after(ctx, tx, creation.Candidate{SkillID: creation.UUID(r.Skill.ID), VersionID: creation.UUID(r.Version.ID)})
		})
		if err == nil && result.Report.Blocked {
			return creation.ErrInvalidCommand
		}
		return err
	}
	s.ReadRun = func(ctx context.Context, ws identity.Workspace, runID string, candidate creation.Candidate) (string, error) {
		id, err := creation.ParseID(runID)
		if err != nil {
			return "", fmt.Errorf("%w: %w", creation.ErrNotFound, err)
		}
		r, found, err := runs.EvaluationRun(ctx, ws.ID, id)
		if err != nil {
			return "", err
		}
		if !found || creation.UUID(r.SkillVersionID) != candidate.VersionID {
			return "", creation.ErrNotFound
		}
		if !r.Terminal {
			return "", creation.ErrInvalidCommand
		}
		feedback, err := evaluations.CreationFeedback(ctx, ws.ID, id)
		if err != nil {
			return "", err
		}
		b, err := json.Marshal(map[string]any{"run_id": runID, "skill_version_id": candidate.VersionID, "execution_status": r.Status, "failure_class": r.FailureClass, "evaluation": feedback})
		return string(b), err
	}
}

func wireCreationAdopt(s *creation.Service, forks *registry.Service) {
	s.Adopt = func(ctx context.Context, tx pgx.Tx, ws identity.Workspace, skillID string) (creation.Candidate, error) {
		id, err := creation.ParseID(skillID)
		if err != nil {
			return creation.Candidate{}, fmt.Errorf("%w: %w", creation.ErrNotFound, err)
		}
		sk, ver, err := forks.ForkIn(ctx, tx, ws, id)
		if errors.Is(err, registry.ErrNotFound) {
			return creation.Candidate{}, fmt.Errorf("%w: %w", creation.ErrNotFound, err)
		}
		if errors.Is(err, registry.ErrNameTaken) {
			return creation.Candidate{}, fmt.Errorf("%w: %w", creation.ErrConflict, err)
		}
		if err != nil {
			return creation.Candidate{}, err
		}
		return creation.Candidate{SkillID: creation.UUID(sk.ID), VersionID: creation.UUID(ver.ID)}, nil
	}
}

func wireCreationTestCases(s *creation.Service, lab *testlab.Service) {
	s.CreateAcceptanceTestCase = func(ctx context.Context, tx pgx.Tx, ws identity.Workspace, skillID, name, prompt string, criteria []string) (string, error) {
		id, err := creation.ParseID(skillID)
		if err != nil {
			return "", err
		}
		tc, err := lab.CreateTestCaseWithCriteria(ctx, tx, ws, id, testlab.ConfirmedTestCase{Name: name, Prompt: prompt, Criteria: criteria})
		if err != nil {
			return "", err
		}
		return creation.UUID(tc.ID), nil
	}
}
