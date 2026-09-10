package main

import (
	"strings"
	"testing"
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
			if tc.refuses && !strings.Contains(reason, "ADR-060") {
				t.Errorf("the refusal does not say which decision it enforces: %q", reason)
			}
		})
	}
}
