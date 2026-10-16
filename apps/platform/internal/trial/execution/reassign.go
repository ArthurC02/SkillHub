package run

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/halt"
)

const (
	ProviderLostAfter = 90 * time.Second

	reassignmentsPerRun = 1

	lostKeyRevokeTimeout = gatewayRequestTimeout
)

func providerForgotAttempt(err error) bool { return errors.Is(err, ErrAttemptUnknown) }

func timesLost(attempts []gen.RunAttempt) int {
	lost := 0
	for _, a := range attempts {
		if a.ErrorClass != nil && *a.ErrorClass == errClassProviderLost {
			lost++
		}
	}
	return lost
}

type SetAsideProvider struct {
	Why         string
	MayComeBack bool
}

func lostProviders(attempts []gen.RunAttempt, halted []halt.Halt) map[string]SetAsideProvider {
	avoid := make(map[string]SetAsideProvider, len(halted)+len(attempts))
	for _, drained := range halted {
		avoid[drained.Provider] = SetAsideProvider{Why: "drained (" + string(drained.Source) + ")", MayComeBack: true}
	}
	for _, a := range attempts {
		if a.ErrorClass != nil && *a.ErrorClass == errClassProviderLost {
			avoid[a.Provider] = SetAsideProvider{Why: "lost this run's earlier attempt"}
		}
	}
	return avoid
}

func (d *driver) providerAnswered(ctx context.Context, attempt gen.RunAttempt) {
	if !attempt.ProviderUnreachableSince.Valid {
		return
	}
	if err := d.svc.recordProviderAnswer(ctx, attempt); err != nil {
		slog.Warn("could not record that the provider answered again",
			"run_attempt_id", pgconv.UUIDString(attempt.ID), "error", err)
	}
}

func (d *driver) providerSilentSince(ctx context.Context, attempt gen.RunAttempt) (time.Time, error) {
	since, err := d.svc.recordProviderSilence(ctx, attempt)
	if err != nil {
		return time.Time{}, err
	}
	return since, nil
}

func (d *driver) providerLost(ctx context.Context, attempt gen.RunAttempt, reason statusReason) error {
	if err := d.finishAttempt(ctx, attempt, errClassProviderLost, string(reason)); err != nil {
		return err
	}
	revokeLostKey(ctx, d.svc.Gateway, attempt)
	d.svc.providers().forget(attempt.Provider)

	attempts, err := d.svc.attempts(ctx, d.cur.WorkspaceID, d.cur.ID)
	if err != nil {
		return err
	}
	if timesLost(attempts) > reassignmentsPerRun {
		return d.finish(ctx, attempt.ID, gen.RunStatusFailed, failureProvider,
			reason+";這個 Run 已經改派過一次")
	}
	slog.Warn("provider lost this attempt; reassigning the run to another provider once",
		"run_id", pgconv.UUIDString(d.cur.ID), "provider", attempt.Provider, "reason", reason)
	return d.dispatch(ctx)
}

func revokeLostKey(ctx context.Context, gateway ModelGateway, attempt gen.RunAttempt) {
	if gateway == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), lostKeyRevokeTimeout)
	defer cancel()
	if err := gateway.Revoke(ctx, pgconv.UUIDString(attempt.ID)); err != nil {
		metrics.GatewayRevokeFailed.Inc()
		slog.Warn("could not revoke a lost attempt's model key; cleanup revokes it again",
			"run_attempt_id", pgconv.UUIDString(attempt.ID), "error", err)
	}
}

func reassignableAfter(attempts []gen.RunAttempt) bool {
	if len(attempts) == 0 {
		return false
	}
	last := attempts[len(attempts)-1]
	return last.ErrorClass != nil && *last.ErrorClass == errClassProviderLost &&
		timesLost(attempts) <= reassignmentsPerRun
}

func (d *driver) budgetForNextAttempt(ctx context.Context, attempts []gen.RunAttempt) (float64, error) {
	if d.svc.Gateway == nil || len(attempts) == 0 {
		return 0, nil
	}
	left, err := d.svc.budgetLeft(ctx, attempts)
	if err != nil {
		return 0, err
	}
	if left <= 0 {
		return 0, fmt.Errorf("this run has spent its model budget of %.2f USD, so no further attempt can run within it",
			d.svc.Gateway.BudgetCeilingUSD())
	}
	return left, nil
}

func (s *Service) usageOf(ctx context.Context, attempts []gen.RunAttempt) (AttemptUsage, error) {
	var total AttemptUsage
	for _, a := range attempts {
		used, err := s.Gateway.Usage(ctx, pgconv.UUIDString(a.ID), attemptUsageSince(a))
		if err != nil {
			return AttemptUsage{}, err
		}
		total.InputTokens += used.InputTokens
		total.OutputTokens += used.OutputTokens
		total.ModelCostUSD += used.ModelCostUSD
		total.CostReported = total.CostReported || used.CostReported
		total.Incomplete = total.Incomplete || used.Incomplete
	}
	return total, nil
}

var errSpendUnreadable = errors.New("the model spend of this run's earlier attempts could not be read, " +
	"so a new attempt cannot be held to what is left of its budget")

func (s *Service) budgetLeft(ctx context.Context, attempts []gen.RunAttempt) (float64, error) {
	used, err := s.usageOf(ctx, attempts)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", errSpendUnreadable, err)
	}
	if used.Incomplete {
		return 0, errSpendUnreadable
	}
	return s.Gateway.BudgetCeilingUSD() - used.ModelCostUSD, nil
}
