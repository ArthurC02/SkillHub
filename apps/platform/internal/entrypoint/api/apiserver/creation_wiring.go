package apiserver

import (
	"context"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/jackc/pgx/v5/pgtype"

	"encoding/json"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
	"github.com/jackc/pgx/v5"
)

func wireCreationReads(s *creation.Service, versions *ingest.Service, search *catalog.Service) {
	s.ValidateDraft = versions.ValidateCreationDraft
	s.ResolveReference = func(ctx context.Context, ws identity.Workspace, skillID, versionID string) (creation.Reference, llmclient.GenerateReference, error) {
		sid, err := creation.ParseID(skillID)
		if err != nil {
			return creation.Reference{}, llmclient.GenerateReference{}, err
		}
		var vid pgtype.UUID
		if versionID != "" {
			vid, err = creation.ParseID(versionID)
			if err != nil {
				return creation.Reference{}, llmclient.GenerateReference{}, err
			}
		}
		fixed, content, err := versions.ReadCreationReference(ctx, ws, sid, vid)
		return creation.Reference{SkillID: creation.UUID(fixed.SkillID), VersionID: creation.UUID(fixed.VersionID), Name: fixed.Name, Available: err == nil, Description: fixed.Description, Compatibility: fixed.Compatibility, AllowedTools: fixed.AllowedTools}, content, err
	}
	s.SearchReferences = func(ctx context.Context, ws identity.Workspace, query string) ([]creation.Reference, error) {
		ids, err := search.CreationReferenceIDs(ctx, query)
		if err != nil {
			return nil, err
		}
		refs := []creation.Reference{}
		for _, id := range ids {
			r, _, err := s.ResolveReference(ctx, ws, id, "")
			if err == nil {
				refs = append(refs, r)
			}
			if len(refs) == 3 {
				break
			}
		}
		return refs, nil
	}
}

func wireCreationWrites(s *creation.Service, versions *ingest.Service, runs *run.Service, evaluations *eval.Service) {
	s.Materialize = func(ctx context.Context, ws identity.Workspace, draft llmclient.GeneratedSkill, p creation.Provenance, after func(context.Context, pgx.Tx, creation.Candidate) error) error {
		provenance := ingest.GeneratedCandidateProvenance{TaskDescription: p.Brief, Model: p.Model, PromptVersion: p.PromptVersion, GenerationInputs: p.Inputs}
		if p.ExistingSkillID != "" {
			id, err := creation.ParseID(p.ExistingSkillID)
			if err != nil {
				return err
			}
			provenance.ExistingSkillID = &id
		}
		result, err := versions.MaterializeGeneratedCandidate(ctx, ws, draft, provenance, func(ctx context.Context, tx pgx.Tx, r ingest.Result) error {
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
			return "", err
		}
		r, found, err := runs.EvaluationRun(ctx, ws.ID, id)
		if err != nil || !found || creation.UUID(r.SkillVersionID) != candidate.VersionID {
			return "", creation.ErrNotFound
		}
		switch r.Status {
		case "succeeded", "failed", "cancelled", "timed_out":
		default:
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

// wireCreationTestCases wires the confirmed acceptance criteria into a real
// Test Case (05 R-46 (b)). Separate from wireCreationWrites rather than an
// added parameter there: wireCreationWrites' call site in app.go already
// exists with four arguments, and this keeps that line untouched — see
// app.go's call to wireCreationWrites for where the one line calling this
// function belongs.
func wireCreationTestCases(s *creation.Service, lab *testlab.Service) {
	s.CreateAcceptanceTestCase = func(ctx context.Context, tx pgx.Tx, ws identity.Workspace, skillID, name, prompt string, criteria []string) (string, error) {
		id, err := creation.ParseID(skillID)
		if err != nil {
			return "", err
		}
		tc, err := lab.CreateTestCaseWithCriteria(ctx, tx, ws, id, name, prompt, criteria)
		if err != nil {
			return "", err
		}
		return creation.UUID(tc.ID), nil
	}
}
