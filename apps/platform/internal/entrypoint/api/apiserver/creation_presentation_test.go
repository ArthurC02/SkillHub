package apiserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
)

func TestCreationSnapshotProjectionKeepsThePublicSnapshotInCredits(t *testing.T) {
	spent := .25
	projection := creationSnapshotProjection{
		Snapshot: creation.Snapshot{Brief: "kept", BudgetUSD: .5, ReservedUSD: 0, SpentUSD: &spent},
		CreditsForUSD: func(usd float64) (int64, bool) {
			return int64(usd * 1000), true
		},
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "usd") {
		t.Errorf("public snapshot exposes USD: %s", encoded)
	}
	var body struct {
		Brief           string `json:"brief"`
		BudgetCredits   int64  `json:"budget_credits"`
		ReservedCredits int64  `json:"reserved_credits"`
		SpentCredits    int64  `json:"spent_credits"`
	}
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatal(err)
	}
	if body.Brief != "kept" || body.BudgetCredits != 500 || body.ReservedCredits != 0 || body.SpentCredits != 250 {
		t.Errorf("projection = %#v, want preserved brief and 500/0/250 credits", body)
	}
}

func TestCreationSessionResponseUsesTheProjectedSnapshot(t *testing.T) {
	response := creationSessionResponse{
		View: creation.View{Snapshot: creation.Snapshot{BudgetUSD: .5}},
		Snapshot: creationSnapshotProjection{
			Snapshot:      creation.Snapshot{BudgetUSD: .5},
			CreditsForUSD: func(float64) (int64, bool) { return 500, true },
		},
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "budget_usd") || !strings.Contains(string(encoded), `"budget_credits":500`) {
		t.Errorf("response did not replace the embedded snapshot: %s", encoded)
	}
}
