// Package queue wires the Postgres job queue (River, ADR-014/016). Infrastructure
// only: no business rules live here, and Go is the only consumer (iron rule 7).
//
// River owns its own schema and versions it with its releases, so EnsureSchema
// applies River's migrations rather than db/migrations carrying a frozen copy that
// would drift on the first dependency upgrade. See the header of
// db/migrations/0016_run_orchestration.sql.
package queue

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

// New builds a River client on pool. A nil cfg (or one with no Workers) yields an
// insert-only client, which is what cmd/api needs: the API enqueues, it never works
// a job.
func New(pool *pgxpool.Pool, cfg *river.Config) (*river.Client[pgx.Tx], error) {
	if cfg == nil {
		cfg = &river.Config{}
	}
	return river.NewClient(riverpgxv5.New(pool), cfg)
}

// EnsureSchema brings River's tables up to the version this build expects.
//
// ponytail: called at worker startup, which is fine for the single-node E1
// deployment (ADR-018) — River takes an advisory lock, so concurrent starts are
// safe. Move it to a deploy step if the platform ever runs migrations separately
// from process start.
func EnsureSchema(ctx context.Context, pool *pgxpool.Pool) error {
	m, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return err
	}
	_, err = m.Migrate(ctx, rivermigrate.DirectionUp, nil)
	return err
}

// StopTimeout bounds a graceful shutdown before it is escalated to cancellation.
//
// Thirty seconds is long enough for a dispatching run to finish the HTTP call it
// is in and short enough to sit inside every default container termination grace
// period, so the escalation below happens while the process is still allowed to
// run its deferred closers.
const StopTimeout = 30 * time.Second

// Stop shuts a client down, then takes it. Both composition roots call this
// rather than river's Stop directly, because "wait for jobs to notice the
// cancelled context" has no bound of its own: a job blocked acquiring a
// connection from an exhausted pool never reaches a cancellation check at all,
// and an unbounded wait means SIGTERM never completes, the orchestrator sends
// SIGKILL after its grace period, and SIGKILL runs no defer — so the connection
// pool and the object store are torn down by the kernel instead of by us.
//
// context.Background() and not the process context: the context the jobs were
// given is already cancelled, and the deadline here is the bound.
func Stop(client *river.Client[pgx.Tx]) {
	ctx, cancel := context.WithTimeout(context.Background(), StopTimeout)
	defer cancel()
	if err := client.Stop(ctx); err == nil {
		return
	} else {
		slog.Warn("queue did not stop gracefully; cancelling the jobs still running",
			"error", err, "waited", StopTimeout)
	}
	cancelCtx, cancelStop := context.WithTimeout(context.Background(), StopTimeout)
	defer cancelStop()
	if err := client.StopAndCancel(cancelCtx); err != nil {
		slog.Error("queue stop", "error", err)
	}
}
