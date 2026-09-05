package worker

import (
	"context"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	"github.com/jackc/pgx/v5/pgtype"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"time"
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
		return creation.Reference{SkillID: creation.UUID(fixed.SkillID), VersionID: creation.UUID(fixed.VersionID), Name: fixed.Name, Available: err == nil}, content, err
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

func wireCreationGateway(s *creation.Service, gateway *run.Gateway) {
	if gateway == nil {
		return
	}
	s.IssueKey = func(ctx context.Context, sessionID, receiptID string, budget float64, ttl time.Duration) (string, error) {
		scoped := *gateway
		scoped.MaxBudgetUSD = budget
		scoped.Model = "gpt-5.4-mini"
		grant, err := scoped.IssueCreation(ctx, sessionID, receiptID, ttl)
		if err != nil {
			return "", err
		}
		return grant.VirtualKey, nil
	}
	s.RevokeKey = gateway.Revoke
}
