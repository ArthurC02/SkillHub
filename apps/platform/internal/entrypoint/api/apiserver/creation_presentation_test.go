package apiserver

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
)

func TestCreationSnapshotProjectionKeepsThePublicSnapshotInCredits(t *testing.T) {
	spent := .25
	interpretation := &creation.DiagramInterpretation{
		Nodes: []string{"驗證輸入"},
		Uncertainties: []creation.DiagramUncertainty{{
			ID: "11111111-1111-4111-8111-111111111111", Question: "誰核准？", Answer: "主管",
		}},
	}
	projection, err := publicSnapshot(
		creation.Snapshot{Brief: "kept", BudgetUSD: .5, ReservedUSD: 0, SpentUSD: &spent, DiagramDescription: "先驗證輸入。", DiagramDescriptionConfirmed: true, DiagramInterpretation: interpretation},
		func(usd float64) (int64, bool) { return int64(usd * 1000), true },
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "usd") {
		t.Errorf("public snapshot exposes USD: %s", encoded)
	}
	var body struct {
		Brief                       string                          `json:"brief"`
		BudgetCredits               int64                           `json:"budget_credits"`
		ReservedCredits             int64                           `json:"reserved_credits"`
		SpentCredits                int64                           `json:"spent_credits"`
		DiagramDescription          string                          `json:"diagram_description"`
		DiagramDescriptionConfirmed bool                            `json:"diagram_description_confirmed"`
		DiagramInterpretation       *creation.DiagramInterpretation `json:"diagram_interpretation"`
	}
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatal(err)
	}
	if body.Brief != "kept" || body.BudgetCredits != 500 || body.ReservedCredits != 0 || body.SpentCredits != 250 {
		t.Errorf("projection = %#v, want preserved brief and 500/0/250 credits", body)
	}
	if body.DiagramDescription != "先驗證輸入。" || !body.DiagramDescriptionConfirmed || body.DiagramInterpretation == nil || body.DiagramInterpretation.Uncertainties[0].Answer != "主管" {
		t.Errorf("projection omitted diagram checkpoints: %#v", body)
	}
}

func TestCreationSessionResponseUsesTheProjectedSnapshot(t *testing.T) {
	snapshot, err := publicSnapshot(creation.Snapshot{BudgetUSD: .5}, func(float64) (int64, bool) { return 500, true })
	if err != nil {
		t.Fatal(err)
	}
	response := creationSessionResponse{View: creation.View{Snapshot: creation.Snapshot{BudgetUSD: .5}}, Snapshot: snapshot}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "budget_usd") || !strings.Contains(string(encoded), `"budget_credits":500`) {
		t.Errorf("response did not replace the embedded snapshot: %s", encoded)
	}
}

func TestAPublicSnapshotNamesSpentCreditsOnlyOnceSpendingIsKnown(t *testing.T) {
	encoded, err := json.Marshal(mustPublicSnapshot(t, creation.Snapshot{BudgetUSD: .5}))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "spent") {
		t.Errorf("a session with unknown spending names it: %s", encoded)
	}
}

func TestAPublicSnapshotConvertsOnlyAmountsAboveZero(t *testing.T) {
	var converted []float64
	nothingSpent := 0.0
	snapshot, err := publicSnapshot(creation.Snapshot{BudgetUSD: .5, ReservedUSD: 0, SpentUSD: &nothingSpent},
		func(usd float64) (int64, bool) { converted = append(converted, usd); return 7, true })
	if err != nil {
		t.Fatal(err)
	}
	if len(converted) != 1 || converted[0] != .5 {
		t.Errorf("converted %v, want only the 0.5 budget", converted)
	}
	if snapshot.BudgetCredits != 7 || snapshot.ReservedCredits != 0 || snapshot.SpentCredits == nil || *snapshot.SpentCredits != 0 {
		t.Errorf("credits = %d/%d/%v, want 7/0/0", snapshot.BudgetCredits, snapshot.ReservedCredits, snapshot.SpentCredits)
	}
}

func TestAPublicSnapshotRefusesWhenAnAmountCannotBeConverted(t *testing.T) {
	spent := .1
	for name, snapshot := range map[string]creation.Snapshot{
		"budget":   {BudgetUSD: .5},
		"reserved": {ReservedUSD: .2},
		"spent":    {SpentUSD: &spent},
	} {
		if _, err := publicSnapshot(snapshot, func(float64) (int64, bool) { return 0, false }); !errors.Is(err, errNoCreditRate) {
			t.Errorf("%s: err = %v, want errNoCreditRate", name, err)
		}
	}
}

func mustPublicSnapshot(t *testing.T, snapshot creation.Snapshot) publicCreationSnapshot {
	t.Helper()
	out, err := publicSnapshot(snapshot, func(usd float64) (int64, bool) { return int64(usd * 1000), true })
	if err != nil {
		t.Fatal(err)
	}
	return out
}
