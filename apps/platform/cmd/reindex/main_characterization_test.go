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
)

const runReindexUnderTest = "SKILLHUB_REINDEX_MAIN_UNDER_TEST"

func TestMain(m *testing.M) {
	if os.Getenv(runReindexUnderTest) == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type reindexExit struct {
	code   int
	stderr string
}

func runReindexMain(t *testing.T, databaseURL string) reindexExit {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0])
	for _, kv := range os.Environ() {
		upper := strings.ToUpper(kv)
		if !strings.HasPrefix(upper, "DATABASE_URL") && !strings.HasPrefix(upper, "LLM_") && !strings.HasPrefix(upper, "REINDEX_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, runReindexUnderTest+"=1", "DATABASE_URL="+databaseURL)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("reindex did not exit within the deadline; stderr:\n%s", stderr.String())
	}
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return reindexExit{code: code, stderr: stderr.String()}
}

func TestAnUnparsableDatabaseURLStopsTheReindex(t *testing.T) {
	got := runReindexMain(t, "::not a url")
	if got.code != 1 || !strings.Contains(got.stderr, "ERROR database pool") {
		t.Fatalf("exit %d, want 1 naming the pool; stderr:\n%s", got.code, got.stderr)
	}
}

func TestAnUnreachableDatabaseStopsTheRebuildBeforeTheBackfill(t *testing.T) {
	got := runReindexMain(t, "postgres://nobody@127.0.0.1:1/none?connect_timeout=1")
	if got.code != 1 || !strings.Contains(got.stderr, "ERROR reindex") {
		t.Fatalf("exit %d, want 1 naming the rebuild; stderr:\n%s", got.code, got.stderr)
	}
	if strings.Contains(got.stderr, "bigram") {
		t.Errorf("the backfill ran after the rebuild failed:\n%s", got.stderr)
	}
}
