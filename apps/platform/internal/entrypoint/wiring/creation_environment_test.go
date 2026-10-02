package wiring

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestCreationEnvironmentFailsClosedAndExposesOnlyWhenEnabled(t *testing.T) {
	t.Setenv("CREATION_LIMITS_JSON", "{}")
	if _, err := CreationLimitsFromEnv(); err == nil {
		t.Fatal("missing limits enabled")
	}

	t.Setenv("CREATION_LIMITS_JSON", `{"max_cost_usd":1,"max_call_cost_usd":0.5,"max_steps":1,"max_tool_calls":1,"call_timeout_seconds":1,"session_timeout_seconds":1,"retention_seconds":1,"max_output_tokens":1}`)
	if limits, err := CreationLimitsFromEnv(); err != nil || !limits.Valid() {
		t.Fatalf("valid limits = %+v, %v", limits, err)
	}

	t.Setenv("CREATION_EXPOSED", "off")
	if CreationExposedFromEnv() {
		t.Fatal("creation exposed outside its explicit setting")
	}
	t.Setenv("CREATION_EXPOSED", "on")
	if !CreationExposedFromEnv() {
		t.Fatal("creation hidden despite its explicit setting")
	}
}

func TestOnlyCreationLimitsThatAreSetButUnusableAreReported(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	t.Setenv("CREATION_LIMITS_JSON", "")
	_, _ = CreationLimitsFromEnv()
	if strings.Contains(logged.String(), "CREATION_LIMITS_JSON") {
		t.Errorf("an unset value was reported as broken:\n%s", logged.String())
	}
	t.Setenv("CREATION_LIMITS_JSON", `{"max_cost_usd":`)
	_, _ = CreationLimitsFromEnv()
	if !strings.Contains(logged.String(), "CREATION_LIMITS_JSON is set but unusable") {
		t.Errorf("a malformed value turned creation off without a word:\n%s", logged.String())
	}
}
