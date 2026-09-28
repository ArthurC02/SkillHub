package worker

import (
	"context"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/jackc/pgx/v5/pgtype"
	"os"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"time"
)

func wireCreationReads(s *creation.Service, versions *ingest.Service, search *catalog.Service) {
	s.SearchKnowledge = func(ctx context.Context, ws identity.Workspace, queries []string) ([]creation.Reference, float64, error) {
		rankings, cost, err := knowledgeRankings(ctx, search, queries)
		if err != nil {
			return nil, cost, err
		}
		return firstResolvedReferences(ctx, s, ws, catalog.FuseRanked(rankings)), cost, nil
	}
	s.ValidateDraft = func(ctx context.Context, draft creation.GeneratedSkill) (string, string, bool, error) {
		check, err := versions.ValidateCreationDraft(ctx, generatedSkillForIngest(draft))
		return check.ContentHash, check.Report, check.Blocked, err
	}
	s.Mask = (&trace.Masker{}).MaskString
	s.ResolveReference = referenceResolver(versions, search)
	s.SearchReferences = func(ctx context.Context, ws identity.Workspace, query string) ([]creation.Reference, error) {
		ids, err := search.CreationReferenceIDs(ctx, query)
		if err != nil {
			return nil, err
		}
		return firstResolvedReferences(ctx, s, ws, ids), nil
	}
}

func knowledgeRankings(ctx context.Context, search *catalog.Service, queries []string) ([][]string, float64, error) {
	var rankings [][]string
	var cost float64
	for _, query := range queries {
		knowledge, err := search.CreationKnowledgeIDs(ctx, query, catalog.CreationMaxDistance)
		cost += knowledge.CostUSD
		if err != nil {
			return nil, cost, err
		}
		rankings = append(rankings, knowledge.IDs)
	}
	return rankings, cost, nil
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

func wireCreationFetch(s *creation.Service) {
	s.Fetch = creation.NewFetcher().Fetch
}

func wireCreationGateway(s *creation.Service, gateway *run.Gateway) {
	if gateway == nil {
		return
	}
	s.IssueKey = func(ctx context.Context, sessionID, receiptID string, budget float64, ttl time.Duration) (string, error) {
		grant, err := gateway.IssueCreationForModel(ctx, sessionID, receiptID, run.CreationKeyTerms{
			TTL: ttl, BudgetUSD: budget, Model: creationModel(),
		})
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

func creationModel() string {
	if m := os.Getenv("CREATION_MODEL"); m != "" {
		return m
	}
	return "skillhub-creation"
}
