package operations

import "testing"

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
