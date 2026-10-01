package wiring

import (
	"testing"
	"time"
)

func TestTheDatabasePoolTakesTheServiceDefaultWhenTheConnectionStringIsSilent(t *testing.T) {
	cfg, err := DatabasePoolConfig("postgres://u:p@db:5432/skillhub", APIPoolMaxConns, APIPoolAcquireWait)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxConns != APIPoolMaxConns || cfg.MaxConnIdleTime != poolMaxConnIdle {
		t.Fatalf("max conns %d idle %v, want %d and %v", cfg.MaxConns, cfg.MaxConnIdleTime, APIPoolMaxConns, poolMaxConnIdle)
	}
}

func TestTheConnectionStringOverridesThePoolDefaults(t *testing.T) {
	cfg, err := DatabasePoolConfig("postgres://u:p@db:5432/skillhub?pool_max_conns=7&pool_max_conn_idle_time=1m", APIPoolMaxConns, APIPoolAcquireWait)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxConns != 7 || cfg.MaxConnIdleTime != time.Minute {
		t.Fatalf("max conns %d idle %v, want the connection string's 7 and 1m", cfg.MaxConns, cfg.MaxConnIdleTime)
	}
}

func TestAnInvalidConnectionStringIsAnError(t *testing.T) {
	if _, err := DatabasePoolConfig("postgres://u:p@db:5432/skillhub?pool_max_conns=many", APIPoolMaxConns, APIPoolAcquireWait); err == nil {
		t.Fatal("an unparsable pool_max_conns was accepted")
	}
}

func TestOnlyAPoolGivenAWaitLimitStopsWaitingForAConnection(t *testing.T) {
	api, err := DatabasePoolConfig("postgres://u:p@db:5432/skillhub", APIPoolMaxConns, APIPoolAcquireWait)
	if err != nil {
		t.Fatal(err)
	}
	if limit, ok := api.ConnConfig.Tracer.(acquireWaitLimit); !ok || time.Duration(limit) != APIPoolAcquireWait {
		t.Fatalf("API pool tracer = %#v, want a %v acquire wait limit", api.ConnConfig.Tracer, APIPoolAcquireWait)
	}
	worker, err := DatabasePoolConfig("postgres://u:p@db:5432/skillhub", WorkerPoolMaxConns, WorkerPoolAcquireWait)
	if err != nil {
		t.Fatal(err)
	}
	if worker.ConnConfig.Tracer != nil {
		t.Fatalf("worker pool tracer = %#v, want none: its queue bookkeeping shares the pool and must wait", worker.ConnConfig.Tracer)
	}
}
