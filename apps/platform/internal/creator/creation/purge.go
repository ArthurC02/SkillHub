package creation

import (
	"context"
	"errors"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

func PurgeExpired(ctx context.Context, pool *pgxpool.Pool, retention time.Duration) (int64, error) {
	if retention <= 0 {
		return 0, errors.New("creation: retention is not configured")
	}
	return gen.New(pool).DeleteExpiredCreationSessions(ctx)
}
