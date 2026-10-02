package wiring

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

func WireCreationReferenceReads(s *creation.Service, versions *ingest.Service, search *catalog.Service) {
	s.ValidateDraft = func(ctx context.Context, draft creation.GeneratedSkill) (string, string, bool, error) {
		check, err := versions.ValidateCreationDraft(ctx, GeneratedSkillForIngest(draft))
		return check.ContentHash, check.Report, check.Blocked, err
	}
	s.Mask = (&trace.Masker{}).MaskString
	s.ResolveReference = referenceResolver(versions, search)
	s.ReadReferenceContent = referenceContentReader(versions)
	s.SearchReferences = func(ctx context.Context, ws identity.Workspace, query string) ([]creation.Reference, error) {
		ids, err := search.CreationReferenceIDs(ctx, query)
		if err != nil {
			return nil, err
		}
		return FirstResolvedReferences(ctx, s, ws, ids), nil
	}
}

func FirstResolvedReferences(ctx context.Context, s *creation.Service, ws identity.Workspace, ids []string) []creation.Reference {
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

func referenceContentReader(versions *ingest.Service) func(context.Context, identity.Workspace, string, string) (creation.ReferenceSkill, error) {
	return func(ctx context.Context, ws identity.Workspace, skillID, versionID string) (creation.ReferenceSkill, error) {
		sid, vid, err := parseReferenceIDs(skillID, versionID)
		if err != nil {
			return creation.ReferenceSkill{}, err
		}
		content, err := versions.ReadCreationReferenceContent(ctx, ws, sid, vid)
		return creation.ReferenceSkill{Name: content.Name, SkillMD: content.SkillMD}, err
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

func GeneratedSkillForIngest(g creation.GeneratedSkill) ingest.GeneratedSkill {
	out := ingest.GeneratedSkill{
		Name: g.Name, Description: g.Description, Compatibility: g.Compatibility,
		AllowedTools: g.AllowedTools, Body: g.Body,
	}
	for _, f := range g.Files {
		out.Files = append(out.Files, ingest.GeneratedFile{Path: f.Path, Content: f.Content})
	}
	return out
}
