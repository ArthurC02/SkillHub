package mxcdrv

import (
	"context"
	"os"
	"testing"
)

func requireRealMXC(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("SKILLHUB_SANDBOX_TEST_MXC_BIN")
	if bin == "" {
		t.Skip("SKILLHUB_SANDBOX_TEST_MXC_BIN is not set: these tests need a real mxc executable (lxc-exec or wxc-exec.exe); every other test in this package uses the fake")
	}
	return bin
}

func TestARealMXCStartsTheRuntime(t *testing.T) {
	h := newHarness(t, requireRealMXC(t), "exits-zero.mjs")
	if !h.driver.Healthy(context.Background()) {
		t.Fatal("Healthy() = false: node --version did not exit 0 inside the real mxc")
	}
}

func TestARealMXCRunsAWorkloadAndReturnsItsExitCode(t *testing.T) {
	h := newHarness(t, requireRealMXC(t), "exits-127.mjs")
	_, out, err := h.run(t)
	if err != nil {
		t.Fatalf("Wait: %v (output %q)", err, out.Output)
	}
	if out.ExitCode != 127 {
		t.Errorf("exit code = %d, want the workload's 127 (output %q)", out.ExitCode, out.Output)
	}
}
