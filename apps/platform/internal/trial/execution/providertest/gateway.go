package providertest

import (
	"context"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

type Gateway struct {
	Spent run.AttemptUsage
}

func NewGateway() *Gateway { return &Gateway{} }

func (g *Gateway) Issue(_ context.Context, _, runAttemptID string, terms run.AttemptKeyTerms) (*run.ModelGatewayGrant, error) {
	return &run.ModelGatewayGrant{
		BaseURL:      "http://model-gateway.test",
		VirtualKey:   "sk-test-" + runAttemptID,
		MaxBudgetUSD: terms.BudgetUSD,
		TPMLimit:     terms.TPMLimit,
		ExpiresAt:    time.Now().Add(terms.TTL),
	}, nil
}

func (g *Gateway) Revoke(context.Context, string) error { return nil }

func (g *Gateway) Usage(context.Context, string, time.Time) (run.AttemptUsage, error) {
	return g.Spent, nil
}
