package providertest

import (
	"context"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

const gatewayCeilingUSD = 0.50

type Gateway struct {
	Spent run.AttemptUsage
}

func NewGateway() *Gateway { return &Gateway{} }

func (g *Gateway) Issue(_ context.Context, _, runAttemptID string, ttl time.Duration, maxBudgetUSD float64) (*run.ModelGatewayGrant, error) {
	return &run.ModelGatewayGrant{
		BaseURL:      "http://model-gateway.test",
		VirtualKey:   "sk-test-" + runAttemptID,
		MaxBudgetUSD: maxBudgetUSD,
		ExpiresAt:    time.Now().Add(ttl),
	}, nil
}

func (g *Gateway) Revoke(context.Context, string) error { return nil }

func (g *Gateway) Usage(context.Context, string, time.Time) (run.AttemptUsage, error) {
	return g.Spent, nil
}

func (g *Gateway) BudgetCeilingUSD() float64 { return gatewayCeilingUSD }
