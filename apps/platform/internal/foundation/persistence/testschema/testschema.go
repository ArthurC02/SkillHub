package testschema

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func MustLock(ctx context.Context, pool *pgxpool.Pool) (unlock func()) {
	unlock, err := Lock(ctx, pool)
	if err != nil {
		panic(err)
	}
	return unlock
}

func Lock(ctx context.Context, pool *pgxpool.Pool) (unlock func(), err error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	if err := gen.New(conn).LockTestSchema(ctx); err != nil {
		conn.Release()
		return nil, err
	}
	return func() {
		_ = gen.New(conn).UnlockTestSchema(context.WithoutCancel(ctx))
		conn.Release()
	}, nil
}
