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
