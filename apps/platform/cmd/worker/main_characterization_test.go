package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/testschema"
)

const runWorkerUnderTest = "SKILLHUB_WORKER_MAIN_UNDER_TEST"

func TestMain(m *testing.M) {
	if os.Getenv(runWorkerUnderTest) == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

var workerEnvironmentPrefixes = []string{
	"DATABASE_URL", "SKILLHUB_", "LLM_", "OBJSTORE_", "DEV_LOGIN", "APP_URL", "COOKIE_", "DEV_CORS_ORIGIN",
	"IMPORT_", "METRICS_ADDR", "CREATION_", "SANDBOX_",
}

type workerExit struct {
	code   int
	stderr string
}

func runWorkerMain(t *testing.T, env ...string) workerExit {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0])
	for _, kv := range os.Environ() {
		if !hasAnyPrefix(strings.ToUpper(kv), workerEnvironmentPrefixes) {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, runWorkerUnderTest+"=1")
	cmd.Env = append(cmd.Env, env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("the worker did not exit within the deadline; stderr:\n%s", stderr.String())
	}
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return workerExit{code: code, stderr: stderr.String()}
}

func holdTheTestSchema(t *testing.T, dsn string) {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	unlock, err := testschema.Lock(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(unlock)
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func TestARefusedPostureStopsTheWorkerBeforeTheDatabase(t *testing.T) {
	got := runWorkerMain(t, "DEV_LOGIN=1", "DATABASE_URL=::not a url")
	if got.code != 1 || !strings.Contains(got.stderr, "worker refuses to start") {
		t.Fatalf("exit %d, want 1 with a refusal; stderr:\n%s", got.code, got.stderr)
	}
	if strings.Contains(got.stderr, "database pool") {
		t.Errorf("the database was opened although the worker refused to start:\n%s", got.stderr)
	}
}

func TestAnUnparsableDatabaseURLStopsTheWorker(t *testing.T) {
	got := runWorkerMain(t, "COOKIE_INSECURE=1", "DATABASE_URL=::not a url")
	if got.code != 1 || !strings.Contains(got.stderr, "ERROR database pool") {
		t.Fatalf("exit %d, want 1 naming the pool; stderr:\n%s", got.code, got.stderr)
	}
	if strings.Contains(got.stderr, "queue schema") {
		t.Errorf("the queue schema was attempted without a pool:\n%s", got.stderr)
	}
}

func TestAnUnreachableDatabaseStopsTheWorkerAtTheQueueSchema(t *testing.T) {
	got := runWorkerMain(t, "COOKIE_INSECURE=1", "DATABASE_URL=postgres://nobody@127.0.0.1:1/none?connect_timeout=1")
	if got.code != 1 || !strings.Contains(got.stderr, "queue schema") {
		t.Fatalf("exit %d, want 1 naming the queue schema; stderr:\n%s", got.code, got.stderr)
	}
	if strings.Contains(got.stderr, "sandbox provider") {
		t.Errorf("the worker went on configuring providers after the schema failed:\n%s", got.stderr)
	}
}

func TestAJudgeServiceWithoutItsTokenStopsTheWorkerAfterReportingItsDependencies(t *testing.T) {
	dsn := os.Getenv("SKILLHUB_TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("SKILLHUB_REQUIRE_DB") == "1" {
			t.Fatal("SKILLHUB_REQUIRE_DB=1 but SKILLHUB_TEST_DATABASE_URL is unset")
		}
		t.Skip("SKILLHUB_TEST_DATABASE_URL not set; skipping the database-backed start-up path")
	}
	holdTheTestSchema(t, dsn)
	got := runWorkerMain(t, "COOKIE_INSECURE=1", "DATABASE_URL="+dsn, "LLM_SERVICE_URL=http://127.0.0.1:1")
	if got.code != 1 || !strings.Contains(got.stderr, "LLM_SERVICE_TOKEN is required when LLM_SERVICE_URL is set") {
		t.Fatalf("exit %d, want 1 naming the missing token; stderr:\n%s", got.code, got.stderr)
	}
	for _, want := range []string{
		"no sandbox provider configured; runs will fail at dispatch",
		"trace ingestion not configured",
		"no model gateway configured; runs will be dispatched with no model credential",
	} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr does not report %q:\n%s", want, got.stderr)
		}
	}
	if strings.Contains(got.stderr, "worker composition") || strings.Contains(got.stderr, "worker started") {
		t.Errorf("the worker went on after refusing the judge service:\n%s", got.stderr)
	}
}
