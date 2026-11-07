package creation

import (
	"context"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5/pgxpool"
)

func PurgeExpired(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	return gen.New(pool).DeleteExpiredCreationSessions(ctx)
}
