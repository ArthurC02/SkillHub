package apiserver

import (
	"context"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	registry "github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/jackc/pgx/v5/pgtype"

	"encoding/json"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
	"github.com/jackc/pgx/v5"
)

func wireCreationReads(s *creation.Service, versions *ingest.Service, search *catalog.Service) {
	s.ValidateDraft = func(ctx context.Context, draft creation.GeneratedSkill) (string, string, bool, error) {
		check, err := versions.ValidateCreationDraft(ctx, generatedSkillForIngest(draft))
		return check.ContentHash, check.Report, check.Blocked, err
	}

	s.Mask = (&trace.Masker{}).MaskString
	s.CatalogCheck = nearestReferences(s, search, catalog.CreationMaxDistance)
	s.DuplicateCheck = nearestReferences(s, search, catalog.CreationDuplicateDistance)
	s.ResolveReference = referenceResolver(versions, search)
	s.SearchReferences = func(ctx context.Context, ws identity.Workspace, query string) ([]creation.Reference, error) {
		ids, err := search.CreationReferenceIDs(ctx, query)
		if err != nil {
			return nil, err
		}
		return firstResolvedReferences(ctx, s, ws, ids), nil
	}
}

func nearestReferences(s *creation.Service, search *catalog.Service, maxDistance float64) func(context.Context, identity.Workspace, string) ([]creation.Reference, float64, error) {
	return func(ctx context.Context, ws identity.Workspace, query string) ([]creation.Reference, float64, error) {
		knowledge, err := search.CreationKnowledgeIDs(ctx, query, maxDistance)
		if err != nil || knowledge.Degraded {
			return nil, knowledge.CostUSD, err
		}
		return firstResolvedReferences(ctx, s, ws, knowledge.IDs), knowledge.CostUSD, nil
	}
}

func firstResolvedReferences(ctx context.Context, s *creation.Service, ws identity.Workspace, ids []string) []creation.Reference {
	refs := []creation.Reference{}
	for _, id := range ids {
		r, _, err := s.ResolveReference(ctx, ws, id, "")
		if err == nil {
			refs = append(refs, r)
		}
		if len(refs) == creation.MaxReferences {
			break
		}
	}
	return refs
}

func referenceResolver(versions *ingest.Service, search *catalog.Service) func(context.Context, identity.Workspace, string, string) (creation.Reference, creation.ReferenceSkill, error) {
	return func(ctx context.Context, ws identity.Workspace, skillID, versionID string) (creation.Reference, creation.ReferenceSkill, error) {
		sid, vid, err := parseReferenceIDs(skillID, versionID)
		if err != nil {
			return creation.Reference{}, creation.ReferenceSkill{}, err
		}
		fixed, content, err := versions.ReadCreationReference(ctx, ws, sid, vid)
		ref := creation.Reference{SkillID: creation.UUID(fixed.SkillID), VersionID: creation.UUID(fixed.VersionID), Name: fixed.Name, Available: err == nil, Description: fixed.Description, Compatibility: fixed.Compatibility, AllowedTools: fixed.AllowedTools}
		addCatalogFacts(ctx, search, &ref)
		return ref, creation.ReferenceSkill{Name: content.Name, SkillMD: content.SkillMD}, err
	}
}

func parseReferenceIDs(skillID, versionID string) (pgtype.UUID, pgtype.UUID, error) {
	var vid pgtype.UUID
	sid, err := creation.ParseID(skillID)
	if err != nil {
		return sid, vid, err
	}
	if versionID != "" {
		vid, err = creation.ParseID(versionID)
	}
	return sid, vid, err
}

func addCatalogFacts(ctx context.Context, search *catalog.Service, ref *creation.Reference) {
	facts, err := search.CatalogReferenceFacts(ctx, ref.SkillID, ref.VersionID)
	if err != nil {
		return
	}
	ref.Tier, ref.ScanStatus = facts.Tier, facts.ScanStatus
	if facts.ScanStatus == "scanned" {
		ref.Warnings = &facts.Warnings
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
		result, err := versions.MaterializeGeneratedCandidate(ctx, ws, generatedSkillForIngest(draft), provenance, func(ctx context.Context, tx pgx.Tx, r ingest.Result) error {
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
	s.Adopt = func(ctx context.Context, ws identity.Workspace, skillID string) (creation.Candidate, error) {
		id, err := creation.ParseID(skillID)
		if err != nil {
			return creation.Candidate{}, err
		}
		sk, ver, err := forks.Fork(ctx, ws, id)
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

func generatedSkillForIngest(g creation.GeneratedSkill) ingest.GeneratedSkill {
	out := ingest.GeneratedSkill{
		Name: g.Name, Description: g.Description, Compatibility: g.Compatibility,
		AllowedTools: g.AllowedTools, Body: g.Body,
	}
	for _, f := range g.Files {
		out.Files = append(out.Files, ingest.GeneratedFile{Path: f.Path, Content: f.Content})
	}
	return out
}
