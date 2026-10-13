package eval

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/verdict"
)

func compatMaterial(compat *RuntimeCompatibility, runtimeSnapshot []byte) material {
	return material{
		run:    RunFacts{RuntimeSnapshot: runtimeSnapshot},
		compat: compat,
	}
}

func TestCompatibilityFindingsWithNoMeasurementIsInfoAndClaimsNothing(t *testing.T) {
	findings := compatibilityFindings(compatMaterial(nil, nil))
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(findings))
	}
	if findings[0].Category != verdict.CategoryCompatibility {
		t.Errorf("category = %q, want %q", findings[0].Category, verdict.CategoryCompatibility)
	}
	if findings[0].Severity != verdict.SeverityInfo {
		t.Errorf("severity = %q, want %q", findings[0].Severity, verdict.SeverityInfo)
	}
	if !strings.Contains(findings[0].Message, "no compatibility measurement exists") {
		t.Errorf("message = %q, want it to say no measurement exists", findings[0].Message)
	}
}

func TestCompatibilityFindingsActivatedIsInfoAndNamesTheImage(t *testing.T) {
	compat := &RuntimeCompatibility{Capability: "activated", Runtime: "native", RuntimeImage: "sha256:abc123"}
	findings := compatibilityFindings(compatMaterial(compat, nil))
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(findings))
	}
	if findings[0].Severity != verdict.SeverityInfo {
		t.Errorf("severity = %q, want %q", findings[0].Severity, verdict.SeverityInfo)
	}
	if !strings.Contains(findings[0].Message, compat.RuntimeImage) {
		t.Errorf("message = %q, want it to name the runtime image %q", findings[0].Message, compat.RuntimeImage)
	}
}

func TestCompatibilityFindingsNotActivatedIsWarning(t *testing.T) {
	compat := &RuntimeCompatibility{Capability: "not_activated", Runtime: "transpiled", RuntimeImage: "sha256:def456"}
	findings := compatibilityFindings(compatMaterial(compat, nil))
	if findings[0].Severity != verdict.SeverityWarning {
		t.Errorf("severity = %q, want %q", findings[0].Severity, verdict.SeverityWarning)
	}
}

func TestCompatibilityFindingsUnverifiedIsWarning(t *testing.T) {
	compat := &RuntimeCompatibility{Capability: "unverified", Runtime: "unverified", RuntimeImage: "sha256:ghi789"}
	findings := compatibilityFindings(compatMaterial(compat, nil))
	if findings[0].Severity != verdict.SeverityWarning {
		t.Errorf("severity = %q, want %q", findings[0].Severity, verdict.SeverityWarning)
	}
}

func TestCompatibilityFindingsMakeNoClaimAboutARunsOwnRuntime(t *testing.T) {
	compat := &RuntimeCompatibility{Capability: "activated", Runtime: "native", RuntimeImage: "sha256:jkl012"}
	snapshot, err := json.Marshal(map[string]any{
		"runtime": map[string]string{"runtime": "claude_agent_sdk", "runtime_version": "1.2.3"},
	})
	if err != nil {
		t.Fatalf("marshal fixture snapshot: %v", err)
	}
	findings := compatibilityFindings(compatMaterial(compat, snapshot))
	if strings.Contains(findings[0].Message, "does not cover") {
		t.Errorf("message = %q, want no claim about coverage of the run's own runtime", findings[0].Message)
	}
	if findings[0].Severity != verdict.SeverityInfo {
		t.Errorf("severity = %q, want %q (only capability decides severity)", findings[0].Severity, verdict.SeverityInfo)
	}
}

func TestAFindingWithNoEvidenceCarriesAnEmptyListNotNull(t *testing.T) {
	findings := compatibilityFindings(compatMaterial(nil, nil))
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1", len(findings))
	}
	encoded, err := json.Marshal(findings[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"evidence":[]`) {
		t.Errorf("finding = %s, want evidence as an empty list", encoded)
	}
}
