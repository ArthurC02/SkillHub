package audit

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// PurgeExpired deletes audit events older than the retention window.
//
// Three quarters of this mechanism already existed and nobody had written the
// fourth: 0013's column comment says "400 day retention (PDM-006 6)", the index
// above it is named for the sweep, and the DELETE branch of enforce_immutable
// was opened specifically so the sweep could reach these rows. There was no
// query, no command and no caller. The consent document a beta participant is
// asked to sign says the row is kept for 400 days and then goes; what actually
// ran was "kept forever", on the one table the immutability trigger makes hard
// to clean up by hand afterwards.
//
// The retention window is a parameter and not a constant here for the same
// reason TRACE_RETENTION and ANALYTICS_RETENTION are deployment variables: the
// number a deployment promises its participants is the deployment's to state,
// and a default compiled in here would be a promise this package invented.
//
// The transaction is this function's own, and it must be: SET LOCAL lasts
// exactly as long as the transaction it runs in, and the flag is what the 0013
// trigger looks for. Every other DELETE on this table stays refused, which is
// what makes the trail a trail.
func PurgeExpired(ctx context.Context, pool *pgxpool.Pool, retention time.Duration) (int64, error) {
	if pool == nil {
		return 0, errors.New("audit: database handle is not configured")
	}
	if retention <= 0 {
		// Fail closed. A zero or negative window would delete the entire trail,
		// and "the sweep ran with no window configured" must not be the way
		// NFR-001's evidence disappears.
		return 0, errors.New("audit: retention window must be positive")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

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
