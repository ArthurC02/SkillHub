package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const runMainUnderTest = "SKILLHUB_API_MAIN_UNDER_TEST"

func TestMain(m *testing.M) {
	if os.Getenv(runMainUnderTest) == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

var mainEnvironmentPrefixes = []string{
	"DATABASE_URL", "SKILLHUB_", "LLM_", "OBJSTORE_", "TRUSTED_PROXIES", "RATE_LIMIT", "DEV_LOGIN",
	"APP_URL", "COOKIE_", "SECURE_COOKIES", "API_ADDR", "METRICS_ADDR", "SANDBOX_", "RUN_PROVIDER", "PACKAGING_PROFILES_DIR",
}

type mainExit struct {
	code   int
	stdout string
	stderr string
}

func runMain(t *testing.T, args []string, env ...string) mainExit {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], args...)
	for _, kv := range os.Environ() {
		if !hasAnyPrefix(strings.ToUpper(kv), mainEnvironmentPrefixes) {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, runMainUnderTest+"=1", "API_ADDR=127.0.0.1:0")
	cmd.Env = append(cmd.Env, env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("main did not exit within the deadline; stderr:\n%s", stderr.String())
	}
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return mainExit{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

const unreachableDatabase = "DATABASE_URL=postgres://nobody@127.0.0.1:1/none?connect_timeout=1"

func TestTheCapabilitiesFlagPrintsTheTableAndExitsCleanly(t *testing.T) {
	got := runMain(t, []string{"--capabilities"}, "DATABASE_URL=::not a url")
	if got.code != 0 {
		t.Fatalf("exit %d, want 0; stderr:\n%s", got.code, got.stderr)
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &rows); err != nil || len(rows) == 0 || rows[0].ID == "" {
		t.Fatalf("stdout is not the capability table (err %v):\n%s", err, got.stdout)
	}
}

func TestAnUnparsableDatabaseURLStopsTheAPIBeforeAnythingElse(t *testing.T) {
	got := runMain(t, nil, "DATABASE_URL=::not a url")
	if got.code != 1 || !strings.Contains(got.stderr, "database pool: DATABASE_URL is not a valid connection string") {
		t.Fatalf("exit %d, want 1 naming the connection string; stderr:\n%s", got.code, got.stderr)
	}
	if strings.Contains(got.stderr, "object store") {
		t.Errorf("the object store was set up although the database URL was refused:\n%s", got.stderr)
	}
}

func TestAnUnreachableObjectStoreStopsTheAPI(t *testing.T) {
	got := runMain(t, nil, unreachableDatabase, "OBJSTORE_ENDPOINT=127.0.0.1:1")
	if got.code != 1 || !strings.Contains(got.stderr, "object store bucket") {
		t.Fatalf("exit %d, want 1 naming the bucket; stderr:\n%s", got.code, got.stderr)
	}
}

func TestAModelServiceWithoutItsTokenStopsTheAPI(t *testing.T) {
	got := runMain(t, nil, unreachableDatabase, "SKILLHUB_CLEAN_MODE=1", "LLM_SERVICE_URL=http://127.0.0.1:1")
	if got.code != 1 || !strings.Contains(got.stderr, "LLM_SERVICE_TOKEN is required when LLM_SERVICE_URL is set") {
		t.Fatalf("exit %d, want 1 naming the missing token; stderr:\n%s", got.code, got.stderr)
	}
	if strings.Contains(got.stderr, "refusing to start") {
		t.Errorf("the posture was judged although the model service was already refused:\n%s", got.stderr)
	}
}

func TestStartupRefusalsAreEachLoggedAndStopTheAPI(t *testing.T) {
	got := runMain(t, nil, unreachableDatabase, "SKILLHUB_CLEAN_MODE=1", "TRUSTED_PROXIES=caddy")
	if got.code != 1 || !strings.Contains(got.stderr, "refusing to start") || !strings.Contains(got.stderr, `\"caddy\"`) {
		t.Fatalf("exit %d, want 1 with the trusted proxies refusal; stderr:\n%s", got.code, got.stderr)
	}
	if strings.Contains(got.stderr, "api composition") || strings.Contains(got.stderr, "queue schema") {
		t.Errorf("composition went ahead after a refusal:\n%s", got.stderr)
	}
}

func TestTheCleanModeWorkerNeedsItsQueueSchema(t *testing.T) {
	got := runMain(t, nil, unreachableDatabase, "SKILLHUB_CLEAN_MODE=1", "RATE_LIMIT=off", "COOKIE_INSECURE=1")
	if got.code != 1 || !strings.Contains(got.stderr, "clean mode: queue schema") {
		t.Fatalf("exit %d, want 1 naming the queue schema; stderr:\n%s", got.code, got.stderr)
	}
	if strings.Contains(got.stderr, "api listening") {
		t.Errorf("the API started listening without its worker:\n%s", got.stderr)
	}
}
