package halt

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

const recoveryRounds = 2

const auditReasonKey = "reason"

type Ledger struct {
	Pool *pgxpool.Pool
}

func (l Ledger) queries() *gen.Queries { return gen.New(l.Pool) }

func (l Ledger) Active(ctx context.Context) (State, error) {
	rows, err := l.queries().ListActiveDispatchHalts(ctx)
	if err != nil {
		return State{}, err
	}
	return StateOf(rows...), nil
}

func (l Ledger) FailClosed(ctx context.Context) State {
	state, err := l.Active(ctx)
	if err == nil {
		return state
	}
	slog.Error("dispatch halt state unreadable; failing closed as if a P1 were declared", "error", err)
	return failedClosed()
}

func (l Ledger) Declare(
	ctx context.Context, provider string, source Source, reason string, actor pgtype.UUID,
) (Halt, error) {
	d, err := newDeclaration(source, reason, actor)
	if err != nil {
		return Halt{}, err
	}
	tx, err := l.Pool.Begin(ctx)
	if err != nil {
		return Halt{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := l.queries().WithTx(tx)

	halt, err := record(ctx, q, provider, d)
	if err != nil {
		return Halt{}, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        actor,
		Action:       audit.ActionDispatchHalt,
		ResourceType: audit.ResourceDispatch,
		ResourceID:   halt.ID,
		Metadata: map[string]any{
			"target": Target(provider),

			"source":           halt.Source,
			"requested_source": source,
			auditReasonKey:     halt.Reason,
		},
	}); err != nil {
		return Halt{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Halt{}, err
	}
	return haltOf(halt), nil
}

func record(ctx context.Context, q *gen.Queries, provider string, d declaration) (gen.DispatchHalt, error) {
	if err := q.LockDispatchHaltTarget(ctx, provider); err != nil {
		return gen.DispatchHalt{}, err
	}
	active, err := q.GetActiveDispatchHalt(ctx, provider)
	if errors.Is(err, pgx.ErrNoRows) {
		return q.InsertDispatchHalt(ctx, gen.InsertDispatchHaltParams{
			Provider: provider, Source: string(d.source), Reason: d.reason, DeclaredBy: d.actor,
		})
	}
	if err != nil {
		return gen.DispatchHalt{}, err
	}
	next := d.over(active)
	return q.RedeclareDispatchHalt(ctx, gen.RedeclareDispatchHaltParams{
		ID: next.ID, Source: next.Source, Reason: next.Reason, DeclaredBy: next.DeclaredBy, ClearRounds: next.ClearRounds,
	})
}

func (l Ledger) Lift(
	ctx context.Context, provider, reason string, actor pgtype.UUID, sources []Source,
) (Halt, bool, error) {
	if err := requireReason(reason); err != nil {
		return Halt{}, false, err
	}
	tx, err := l.Pool.Begin(ctx)
	if err != nil {
		return Halt{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := l.queries().WithTx(tx)

	halt, err := q.LiftDispatchHalt(ctx, gen.LiftDispatchHaltParams{
		Provider: provider, LiftReason: &reason, LiftedBy: actor, Sources: sourceValues(sources),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Halt{}, false, nil
	}
	if err != nil {
		return Halt{}, false, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        actor,
		Action:       audit.ActionDispatchResume,
		ResourceType: audit.ResourceDispatch,
		ResourceID:   halt.ID,
		Metadata: map[string]any{
			"target":       Target(provider),
			"source":       halt.Source,
			"halt_reason":  halt.Reason,
			"lift_reason":  reason,
			"clear_rounds": halt.ClearRounds,
		},
	}); err != nil {
		return Halt{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Halt{}, false, err
	}
	return haltOf(halt), true, nil
}

func sourceValues(sources []Source) []string {
	values := make([]string, len(sources))
	for i, source := range sources {
		values[i] = string(source)
	}
	return values
}

func (l Ledger) CountClearRound(ctx context.Context, provider string, minRoundGap time.Duration) {
	rounds, err := l.queries().SetDispatchHaltClearRounds(ctx, gen.SetDispatchHaltClearRoundsParams{
		Provider: provider, Sources: sourceValues(AutomaticallyRecoveringSources()), MinRoundGapSeconds: minRoundGap.Seconds(),
	})
	if errors.Is(err, pgx.ErrNoRows) {

		return
	}
	if err != nil {
		slog.Error("counting clear rounds failed", "target", Target(provider), "error", err)
		return
	}
	if int(rounds) < recoveryRounds {
		return
	}

	if _, lifted, err := l.Lift(ctx, provider, fmt.Sprintf(
		"X-04: below the threshold for %d consecutive reconciler rounds", rounds),
		pgtype.UUID{}, AutomaticallyRecoveringSources()); err != nil {
		slog.Error("could not lift the X-04 halt", "target", Target(provider), "error", err)
	} else if lifted {
		slog.Info("dispatch resumed: X-04 threshold clear", "target", Target(provider), "rounds", rounds)
	}
}

func (l Ledger) PublishMetrics(ctx context.Context) {
	state, err := l.Active(ctx)
	if err != nil {

		slog.Error("could not publish dispatch halt metrics", "error", err)
		return
	}
	metrics.DispatchHalted.Reset()
	for target, halt := range state.byTarget {
		metrics.DispatchHalted.WithLabelValues(Target(target), halt.Source).Set(1)
	}
}
