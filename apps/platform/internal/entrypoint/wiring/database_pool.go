package wiring

import (
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	APIPoolMaxConns    = 32
	WorkerPoolMaxConns = 16
	poolMaxConnIdle    = 5 * time.Minute
)

func DatabasePoolConfig(connString string, maxConns int32) (*pgxpool.Config, error) {
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
	return cfg, nil
}
