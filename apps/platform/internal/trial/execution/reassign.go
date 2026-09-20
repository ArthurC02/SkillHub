package run

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

const (
	ProviderLostAfter = 90 * time.Second

	reassignmentsPerRun = 1
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

func lostProviders(attempts []gen.RunAttempt, halted map[string]gen.DispatchHalt) map[string]SetAsideProvider {
	avoid := make(map[string]SetAsideProvider, len(halted)+len(attempts))
	for name, halt := range halted {
		avoid[name] = SetAsideProvider{Why: "drained (" + halt.Source + ")", MayComeBack: true}
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
	if err := d.svc.queries().ClearAttemptProviderUnreachable(ctx, gen.ClearAttemptProviderUnreachableParams{
		ID: attempt.ID, WorkspaceID: attempt.WorkspaceID,
	}); err != nil {
		slog.Warn("could not record that the provider answered again",
			"run_attempt_id", pgconv.UUIDString(attempt.ID), "error", err)
	}
}

func (d *driver) providerSilentSince(ctx context.Context, attempt gen.RunAttempt) (time.Time, error) {
	since, err := d.svc.queries().MarkAttemptProviderUnreachable(ctx, gen.MarkAttemptProviderUnreachableParams{
		ID: attempt.ID, WorkspaceID: attempt.WorkspaceID,
	})
	if err != nil {
		return time.Time{}, err
	}
	return since.Time, nil
}

func (d *driver) providerLost(ctx context.Context, attempt gen.RunAttempt, reason statusReason) error {
	d.finishAttempt(ctx, attempt, errClassProviderLost, string(reason))
	d.svc.providers().forget(attempt.Provider)

	attempts, err := d.svc.Attempts(ctx, d.cur.WorkspaceID, d.cur.ID)
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
		since := time.Now().UTC().Add(-time.Hour)
		if a.CreatedAt.Valid {
			since = a.CreatedAt.Time.UTC()
		}
		used, err := s.Gateway.Usage(ctx, pgconv.UUIDString(a.ID), since)
		if err != nil {
			return AttemptUsage{}, err
		}
		total.InputTokens += used.InputTokens
		total.OutputTokens += used.OutputTokens
		total.ModelCostUSD += used.ModelCostUSD
		total.CostReported = total.CostReported || used.CostReported
	}
	return total, nil
}

var errSpendUnreadable = errors.New("the model spend of this run's earlier attempts could not be read, " +
	"so a new attempt cannot be held to what is left of its budget")

func (s *Service) budgetLeft(ctx context.Context, attempts []gen.RunAttempt) (float64, error) {
	used, err := s.usageOf(ctx, attempts)
	if err != nil {
		return 0, errSpendUnreadable
	}
	return s.Gateway.BudgetCeilingUSD() - used.ModelCostUSD, nil
}
