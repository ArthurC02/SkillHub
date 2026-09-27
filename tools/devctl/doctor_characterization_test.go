package main

import "testing"

func TestAMissingRequiredToolFailsTheDoctor(t *testing.T) {
	t.Parallel()
	result := checkRequiredVersion("skillhub-no-such-tool", nil, "")
	if result.status != "FAIL" || !result.required || result.detail != "not found on PATH" {
		t.Fatalf("got %+v, want a required FAIL saying the tool is not on PATH", result)
	}
}

func TestAMissingOptionalToolOnlyWarns(t *testing.T) {
	t.Parallel()
	result := checkOptionalVersion("skillhub-no-such-tool", nil, "")
	if result.status != "WARN" || result.required || result.detail != "not found on PATH" {
		t.Fatalf("got %+v, want an optional WARN saying the tool is not on PATH", result)
	}
}

func TestARequiredToolAtTheWrongVersionFailsTheDoctor(t *testing.T) {
	t.Parallel()
	result := checkRequiredVersion("go", []string{"version"}, "go0.1")
	if result.status != "FAIL" || !result.required {
		t.Fatalf("got %+v, want a required FAIL for a version mismatch", result)
	}
}
