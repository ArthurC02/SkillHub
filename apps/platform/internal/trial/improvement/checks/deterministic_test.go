package checks

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/facts"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/verdict"
)

const (
	eventID     = "0f0a1e6c-1c9a-4f8e-9a2b-1d5a2c7b3e01"
	finalOutput = "結果 ✅：Removed 17 duplicate rows and saved the result to output.xlsx."
)

func fixtureMaterial(complete bool) (facts.Material, map[string]trace.EventView) {
	event := trace.EventView{
		EventID:    eventID,
		Attempt:    1,
		Seq:        3,
		OccurredAt: "2026-08-17T09:12:07Z",
		EmittedBy:  trace.SourceSandbox,
		Type:       trace.TypeToolCall,
		Payload:    json.RawMessage(`{"tool_name":"bash","outcome":"succeeded","duration_ms":3412}`),
	}
	m := facts.Material{
		Criteria: []testlab.Criterion{
			{ID: "c1", Text: "the duplicates are removed"},
			{ID: "c2", Text: "an xlsx file is produced"},
		},
		Advanced: trace.AdvancedView{Complete: complete, Events: []trace.EventView{event}},
		Summary:  trace.Summary{FinalOutput: finalOutput},
		Artifacts: []facts.Artifact{
			{FileName: "output.xlsx", SizeBytes: 4096, ContentType: "application/vnd.ms-excel", ContentHash: "sha256:abc"},
		},
	}
	return m, map[string]trace.EventView{eventID: event}
}

func compatMaterial(compat *facts.RuntimeCompatibility, runtimeSnapshot []byte) facts.Material {
	return facts.Material{
		Run:    facts.Run{RuntimeSnapshot: runtimeSnapshot},
		Compat: compat,
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
	compat := &facts.RuntimeCompatibility{Capability: "activated", Runtime: "native", RuntimeImage: "sha256:abc123"}
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
	compat := &facts.RuntimeCompatibility{Capability: "not_activated", Runtime: "transpiled", RuntimeImage: "sha256:def456"}
	findings := compatibilityFindings(compatMaterial(compat, nil))
	if findings[0].Severity != verdict.SeverityWarning {
		t.Errorf("severity = %q, want %q", findings[0].Severity, verdict.SeverityWarning)
	}
}

func TestCompatibilityFindingsUnverifiedIsWarning(t *testing.T) {
	compat := &facts.RuntimeCompatibility{Capability: "unverified", Runtime: "unverified", RuntimeImage: "sha256:ghi789"}
	findings := compatibilityFindings(compatMaterial(compat, nil))
	if findings[0].Severity != verdict.SeverityWarning {
		t.Errorf("severity = %q, want %q", findings[0].Severity, verdict.SeverityWarning)
	}
}

func TestCompatibilityFindingsMakeNoClaimAboutARunsOwnRuntime(t *testing.T) {
	compat := &facts.RuntimeCompatibility{Capability: "activated", Runtime: "native", RuntimeImage: "sha256:jkl012"}
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

func TestActivationNeverClaimsTheModelChoseNotToUseTheSkill(t *testing.T) {
	m, _ := fixtureMaterial(true)
	m.Run = facts.Run{Status: "succeeded", Succeeded: true}
	findings := activationFindings(m)
	if len(findings) != 1 || findings[0].Category != verdict.CategoryActivation {
		t.Fatalf("one activation finding expected, got %+v", findings)
	}

	for _, forbidden := range []string{"chose", "declined", "ignored", "decided not"} {
		if strings.Contains(strings.ToLower(findings[0].Message), forbidden) {
			t.Errorf("the message claims something the trace cannot show (%q): %s",
				forbidden, findings[0].Message)
		}
	}
}

func TestASuccessfulRunWithNoOutputFilesIsReported(t *testing.T) {
	m, _ := fixtureMaterial(true)
	m.Run = facts.Run{Status: "succeeded", Succeeded: true}
	m.Artifacts = nil
	findings := artifactFindings(m)
	if len(findings) != 1 || findings[0].Severity != verdict.SeverityWarning {
		t.Fatalf("handoff 丙-5's case has to be visible, got %+v", findings)
	}

	m.Run.Status, m.Run.Succeeded = "failed", false
	if got := artifactFindings(m); len(got) != 0 {
		t.Errorf("no finding for a run that did not finish, got %+v", got)
	}
}

func TestCostIsReportedAsALowerBoundAndUnreportedIsNotZero(t *testing.T) {
	m, _ := fixtureMaterial(true)
	m.Summary.Usage = nil
	if got := costFindings(m); !strings.Contains(got[0].Message, "rather than zero") {
		t.Errorf("an absent usage event is unknown, not $0: %s", got[0].Message)
	}
	cost := 0.0142
	m.Summary.Usage = &trace.UsageSummary{
		Model: "gpt-5.4-mini", InputTokens: 100, OutputTokens: 20,
		CostUSD: &cost, CostSource: "gateway",
	}
	if got := costFindings(m); !strings.Contains(got[0].Message, "lower bound") {
		t.Errorf("handoff 丙-3 requires the total to be labelled a lower bound: %s", got[0].Message)
	}
}

func TestAnEmptyManifestNeverReadsAsAnEmptyRun(t *testing.T) {
	said := func(mutate func(*facts.Material)) string {
		m, _ := fixtureMaterial(true)
		m.Run.Status, m.Run.Succeeded = "succeeded", true
		mutate(&m)
		parts := []string{}
		for _, f := range artifactFindings(m) {
			parts = append(parts, f.Message)
		}
		return strings.Join(parts, " | ")
	}
	states := map[string]string{
		"present":        said(func(*facts.Material) {}),
		"never recorded": said(func(m *facts.Material) { m.Artifacts = nil }),
		"deleted":        said(func(m *facts.Material) { m.Artifacts, m.Absent = nil, facts.ArtifactAbsence{Deleted: 2} }),
		"expired":        said(func(m *facts.Material) { m.Artifacts, m.Absent = nil, facts.ArtifactAbsence{Expired: 2} }),
		"one of each":    said(func(m *facts.Material) { m.Absent = facts.ArtifactAbsence{Deleted: 1} }),
	}
	seen := map[string]string{}
	for name, sentence := range states {
		if sentence == "" {
			t.Fatalf("%s: no sentence at all is the silent judgement the clause forbids", name)
		}
		if other, dup := seen[sentence]; dup {
			t.Errorf("%q and %q share one sentence: %s", name, other, sentence)
		}
		seen[sentence] = name
	}

	if strings.Contains(states["never recorded"], "cannot read") {
		t.Errorf("a run that recorded nothing has nothing to have lost: %s", states["never recorded"])
	}
	for _, name := range []string{"deleted", "expired"} {
		if strings.Contains(states[name], "no output files were recorded") {
			t.Errorf("%s reads as a run that produced nothing: %s", name, states[name])
		}
	}
	if !strings.Contains(states["deleted"], "deleted from this workspace") ||
		strings.Contains(states["deleted"], "retention") {
		t.Errorf("deleted has to name deletion and only deletion: %s", states["deleted"])
	}
	if !strings.Contains(states["expired"], "past the retention") ||
		strings.Contains(states["expired"], "deleted") {
		t.Errorf("expired has to name retention and only retention: %s", states["expired"])
	}
	if !strings.Contains(states["one of each"], "output.xlsx") ||
		!strings.Contains(states["one of each"], "cannot read") {
		t.Errorf("a manifest that is part readable says both halves: %s", states["one of each"])
	}
}
