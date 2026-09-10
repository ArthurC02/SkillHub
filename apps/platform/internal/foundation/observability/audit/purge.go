package audit

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func PurgeExpired(ctx context.Context, pool *pgxpool.Pool, retention time.Duration) (int64, error) {
	if pool == nil {
		return 0, errors.New("audit: database handle is not configured")
	}
	if retention <= 0 {

		return 0, errors.New("audit: retention window must be positive")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// The immutability trigger on this table blocks DELETE unless this flag is
	// set; SET LOCAL scopes it to this transaction only, so no other statement
	// in this connection's session can delete a row through it.
	if _, err := tx.Exec(ctx, "SET LOCAL skillhub.purge = 'on'"); err != nil {
		return 0, err
	}
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-retention), Valid: true}
	n, err := gen.New(tx).DeleteExpiredAuditEvents(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return n, nil
}
