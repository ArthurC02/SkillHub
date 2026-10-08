package operations

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestRunViewLastStepTime(t *testing.T) {
	stepAt := pgtype.Timestamptz{Time: time.Date(2026, 10, 8, 8, 9, 10, 0, time.UTC), Valid: true}
	for _, tc := range []struct {
		name string
		last pgtype.Timestamptz
		want string
	}{
		{name: "recorded step", last: stepAt, want: "2026-10-08T08:09:10Z"},
		{name: "no steps yet", want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			view := runViewFromRow(gen.ListPlatformAgentRunsRow{LastStepAt: tc.last})
			if view.LastStepAt != tc.want {
				t.Errorf("LastStepAt = %q, want %q", view.LastStepAt, tc.want)
			}
			payload, err := json.Marshal(view)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(string(payload), `"last_step_at"`); got != (tc.want != "") {
				t.Errorf("last_step_at presence = %v, want %v in %s", got, tc.want != "", payload)
			}
		})
	}
}

func TestHaltReasonNamesTheBrakeBeforeTheDisable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled bool
		braked  bool
		want    string
	}{
		{name: "enabled and no brake runs on", enabled: true, braked: false, want: ""},
		{name: "disabled stops", enabled: false, braked: false, want: stoppedByDisable},
		{name: "brake stops an enabled agent", enabled: true, braked: true, want: stoppedByBrake},
		{name: "brake is named when both hold", enabled: false, braked: true, want: stoppedByBrake},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := haltReason(tc.enabled, tc.braked); got != tc.want {
				t.Errorf("haltReason(enabled=%v, braked=%v) = %q, want %q", tc.enabled, tc.braked, got, tc.want)
			}
		})
	}
}
