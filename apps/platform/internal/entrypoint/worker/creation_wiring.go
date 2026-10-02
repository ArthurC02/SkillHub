package worker

import (
	"context"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
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
		return wiring.FirstResolvedReferences(ctx, s, ws, catalog.FuseRanked(rankings)), cost, nil
	}
	wiring.WireCreationReferenceReads(s, versions, search)
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

func creationModel() string {
	if m := os.Getenv("CREATION_MODEL"); m != "" {
		return m
	}
	return "skillhub-creation"
}
