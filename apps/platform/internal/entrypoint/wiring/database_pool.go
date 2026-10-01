package wiring

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	APIPoolMaxConns       = 32
	WorkerPoolMaxConns    = 16
	APIPoolAcquireWait    = 10 * time.Second
	WorkerPoolAcquireWait = time.Duration(0)
	poolMaxConnIdle       = 5 * time.Minute
)

func DatabasePoolConfig(connString string, maxConns int32, acquireWait time.Duration) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(connString, "pool_max_conns") {
		cfg.MaxConns = maxConns
	}
	if !strings.Contains(connString, "pool_max_conn_idle_time") {
		cfg.MaxConnIdleTime = poolMaxConnIdle
	}
	if acquireWait > 0 {
		cfg.ConnConfig.Tracer = acquireWaitLimit(acquireWait)
	}
	return cfg, nil
}

type acquireWaitLimit time.Duration

type endAcquireWait struct{}

func (l acquireWaitLimit) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	limited, cancel := context.WithTimeout(ctx, time.Duration(l))
	return context.WithValue(limited, endAcquireWait{}, cancel)
}

func (acquireWaitLimit) TraceAcquireEnd(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireEndData) {
	if cancel, ok := ctx.Value(endAcquireWait{}).(context.CancelFunc); ok {
		cancel()
	}
}

func (acquireWaitLimit) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	return ctx
}

func (acquireWaitLimit) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
