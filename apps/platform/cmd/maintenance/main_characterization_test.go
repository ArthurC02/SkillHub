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

const runMaintenanceUnderTest = "SKILLHUB_MAINTENANCE_MAIN_UNDER_TEST"

func TestMain(m *testing.M) {
	if os.Getenv(runMaintenanceUnderTest) == "1" {
		os.Args = append(os.Args[:1], strings.Fields(os.Getenv(runMaintenanceUnderTest+"_ARGS"))...)
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

var maintenanceEnvironmentPrefixes = []string{
	"DATABASE_URL", "SKILLHUB_", "OBJSTORE_", "MAINTENANCE_", "AUDIT_RETENTION", "PURGE_GRACE",
}

type maintenanceExit struct {
	code   int
	stderr string
}

func runMaintenanceMain(t *testing.T, args string, env ...string) maintenanceExit {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0])
	for _, kv := range os.Environ() {
		if !hasAnyPrefix(strings.ToUpper(kv), maintenanceEnvironmentPrefixes) {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, runMaintenanceUnderTest+"=1", runMaintenanceUnderTest+"_ARGS="+args)
	cmd.Env = append(cmd.Env, env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("maintenance did not exit within the deadline; stderr:\n%s", stderr.String())
	}
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return maintenanceExit{code: code, stderr: stderr.String()}
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

const unreachableMaintenanceDatabase = "DATABASE_URL=postgres://nobody@127.0.0.1:1/none?connect_timeout=1"

func TestMaintenanceWithoutAJobPrintsItsUsageAndExitsTwo(t *testing.T) {
	got := runMaintenanceMain(t, "", unreachableMaintenanceDatabase)
	if got.code != 2 || !strings.Contains(got.stderr, "usage: maintenance purge-accounts") {
		t.Fatalf("exit %d, want 2 with the usage; stderr:\n%s", got.code, got.stderr)
	}
}

func TestAnUnknownJobExitsTwo(t *testing.T) {
	got := runMaintenanceMain(t, "no-such-job", unreachableMaintenanceDatabase)
	if got.code != 2 || !strings.Contains(got.stderr, "unknown job") || !strings.Contains(got.stderr, "no-such-job") {
		t.Fatalf("exit %d, want 2 naming the unknown job; stderr:\n%s", got.code, got.stderr)
	}
}

func TestAFailedJobExitsOneAndNamesTheJob(t *testing.T) {
	got := runMaintenanceMain(t, "purge-audit", unreachableMaintenanceDatabase)
	if got.code != 1 || !strings.Contains(got.stderr, "maintenance job failed") ||
		!strings.Contains(got.stderr, "job=purge-audit") || !strings.Contains(got.stderr, "AUDIT_RETENTION") {
		t.Fatalf("exit %d, want 1 naming purge-audit and its missing retention; stderr:\n%s", got.code, got.stderr)
	}
}

func TestAnUnparsableDatabaseURLStopsMaintenance(t *testing.T) {
	got := runMaintenanceMain(t, "purge-audit", "DATABASE_URL=::not a url")
	if got.code != 1 || !strings.Contains(got.stderr, "ERROR database pool") {
		t.Fatalf("exit %d, want 1 naming the pool; stderr:\n%s", got.code, got.stderr)
	}
	if strings.Contains(got.stderr, "maintenance job failed") {
		t.Errorf("the job ran without a pool:\n%s", got.stderr)
	}
}
