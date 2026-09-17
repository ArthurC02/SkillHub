package main

import (
	"strings"
	"testing"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func TestCleanModeRefusal(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   string
		unset   bool
		refuses bool
	}{
		{name: "unset (the shipped default)", unset: true},
		{name: "empty", value: ""},
		{name: "0", value: "0"},
		{name: "true", value: "true"},
		{name: "1", value: "1", refuses: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.unset {
				t.Setenv("SKILLHUB_CLEAN_MODE", "")
			} else {
				t.Setenv("SKILLHUB_CLEAN_MODE", tc.value)
			}
			reason := cleanModeRefusal()
			if (reason != "") != tc.refuses {
				t.Fatalf("SKILLHUB_CLEAN_MODE=%q -> refusal %q, want refusal=%v", tc.value, reason, tc.refuses)
			}
			if tc.refuses && !strings.Contains(reason, "single process") {
				t.Errorf("the refusal does not say which decision it enforces: %q", reason)
			}
		})
	}
}

func TestTheWorkerRefusesToStartWithDevLoginATokenlessProviderOrCleanMode(t *testing.T) {
	for _, name := range []string{"APP_URL", "DEV_CORS_ORIGIN", "IMPORT_ALLOW_INSECURE", "IMPORT_EXTRA_HOSTS", "COOKIE_INSECURE"} {
		t.Setenv(name, "")
	}
	t.Setenv("DEV_LOGIN", "1")
	t.Setenv("SKILLHUB_CLEAN_MODE", "1")
	refusals := startupRefusals(run.NewRegistry(&run.Provider{Name: "tokenless"}))
	if len(refusals) != 3 || !strings.Contains(refusals[0], "DEV_LOGIN") || !strings.Contains(refusals[1], "tokenless") || !strings.Contains(refusals[2], "single process") {
		t.Fatalf("refusals = %q, want the dev login, the tokenless provider, then clean mode", refusals)
	}

	t.Setenv("COOKIE_INSECURE", "1")
	t.Setenv("SKILLHUB_CLEAN_MODE", "")
	if refusals := startupRefusals(run.NewRegistry()); len(refusals) != 0 {
		t.Fatalf("a local worker with dev login on insecure cookies was refused: %q", refusals)
	}
}
