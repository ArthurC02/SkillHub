package queue

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMain(m *testing.M) {
	if os.Getenv("SKILLHUB_TEST_DATABASE_URL") == "" && os.Getenv("SKILLHUB_REQUIRE_DB") == "1" {
		fmt.Fprintln(os.Stderr, "SKILLHUB_REQUIRE_DB=1 but SKILLHUB_TEST_DATABASE_URL is unset; "+
			"this run would have skipped every database test and still reported success")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func poolInFreshSchema(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("SKILLHUB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SKILLHUB_TEST_DATABASE_URL is unset")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("queue_schema_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestProcessesStartingTogetherOnASmallPoolAllFindTheQueueSchemaReady(t *testing.T) {
	pool := poolInFreshSchema(t)
	const processes = 6
	errs := make([]error, processes)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range processes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			errs[i] = EnsureSchema(ctx, pool)
		}()
	}
	close(start)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("process %d could not prepare the queue schema: %v", i, err)
		}
	}
	var tables int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'river_job'",
	).Scan(&tables); err != nil || tables != 1 {
		t.Fatalf("river_job tables in the schema: %d (%v), want 1", tables, err)
	}
}

func TestTheSoleProcessPreparesTheQueueSchemaOnItsOnlyPoolConnection(t *testing.T) {
	cfg := poolInFreshSchema(t).Config()
	cfg.MaxConns = 1
	var dials atomic.Int32
	dial := cfg.ConnConfig.DialFunc
	cfg.ConnConfig.DialFunc = func(ctx context.Context, network, addr string) (net.Conn, error) {
		dials.Add(1)
		return dial(ctx, network, addr)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := EnsureSchemaAsSoleProcess(ctx, pool); err != nil {
		t.Fatalf("the queue schema could not be prepared on a one-connection pool: %v", err)
	}

	var tables int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'river_job'",
	).Scan(&tables); err != nil || tables != 1 {
		t.Fatalf("river_job tables in the schema: %d (%v), want 1", tables, err)
	}
	if got := dials.Load(); got != 1 {
		t.Fatalf("connections opened: %d, want 1 — a database that accepts one connection refuses the rest", got)
	}
}
