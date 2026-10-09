package wiring

import (
	"slices"
	"testing"

	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

func TestTheJudgePanelIsOnOnlyWhenAskedAndAnythingElseRefusesToStart(t *testing.T) {
	cases := []struct {
		value   string
		panel   []string
		refused bool
	}{
		{value: "", panel: nil},
		{value: "off", panel: nil},
		{value: "on", panel: eval.PanelRoles},
		{value: "ON", refused: true},
		{value: "yes", refused: true},
		{value: "3", refused: true},
	}
	for _, tc := range cases {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv(JudgePanelEnv, tc.value)
			panel, err := JudgePanelFromEnv()
			if (err != nil) != tc.refused || !slices.Equal(panel, tc.panel) {
				t.Fatalf("JudgePanelFromEnv(%q) = %v, %v; want %v, refused=%v", tc.value, panel, err, tc.panel, tc.refused)
			}
		})
	}
}
