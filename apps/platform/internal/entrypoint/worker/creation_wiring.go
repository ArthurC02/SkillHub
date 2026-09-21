package worker

import (
	"context"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/jackc/pgx/v5/pgtype"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"time"
)

func wireCreationReads(s *creation.Service, versions *ingest.Service, search *catalog.Service) {
	s.SearchKnowledge = func(ctx context.Context, ws identity.Workspace, queries []string) ([]creation.Reference, float64, error) {
		var rankings [][]string
		var cost float64
		for _, query := range queries {
			ids, c, _, err := search.CreationKnowledgeIDs(ctx, query, catalog.CreationMaxDistance)
			cost += c
			if err != nil {
				return nil, cost, err
			}
			rankings = append(rankings, ids)
		}
		ids := catalog.FuseRanked(rankings)
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
		return refs, cost, nil
	}
	s.ValidateDraft = func(ctx context.Context, draft creation.GeneratedSkill) (string, string, bool, error) {
		return versions.ValidateCreationDraft(ctx, generatedSkillForIngest(draft))
	}
	s.Mask = (&trace.Masker{}).MaskString
	s.ResolveReference = func(ctx context.Context, ws identity.Workspace, skillID, versionID string) (creation.Reference, creation.ReferenceSkill, error) {
		sid, err := creation.ParseID(skillID)
		if err != nil {
			return creation.Reference{}, creation.ReferenceSkill{}, err
		}
		var vid pgtype.UUID
		if versionID != "" {
			vid, err = creation.ParseID(versionID)
			if err != nil {
				return creation.Reference{}, creation.ReferenceSkill{}, err
			}
		}
		fixed, content, err := versions.ReadCreationReference(ctx, ws, sid, vid)
		ref := creation.Reference{SkillID: creation.UUID(fixed.SkillID), VersionID: creation.UUID(fixed.VersionID), Name: fixed.Name, Available: err == nil, Description: fixed.Description, Compatibility: fixed.Compatibility, AllowedTools: fixed.AllowedTools}

		if tier, scan, warnings, ferr := search.CatalogReferenceFacts(ctx, ref.SkillID, ref.VersionID); ferr == nil {
			ref.Tier, ref.ScanStatus = tier, scan
			if scan == "scanned" {
				w := warnings
				ref.Warnings = &w
			}
		}
		return ref, creation.ReferenceSkill{Name: content.Name, SkillMD: content.SkillMD}, err
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
			if len(refs) == creation.MaxReferences {
				break
			}
		}
		return refs, nil
	}
}

func wireCreationFetch(s *creation.Service) {
	s.Fetch = creation.NewFetcher(false).Fetch
}

func wireCreationGateway(s *creation.Service, gateway *run.Gateway) {
	if gateway == nil {
		return
	}
	s.IssueKey = func(ctx context.Context, sessionID, receiptID string, budget float64, ttl time.Duration) (string, error) {
		grant, err := gateway.IssueCreationForModel(ctx, sessionID, receiptID, ttl, budget, "gpt-5.4-mini")
		if err != nil {
			return "", err
		}
		return grant.VirtualKey, nil
	}
	s.RevokeKey = gateway.Revoke
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
