package packaging

import (
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestPackagingActivityTreatsUnfinishedScanStatesDefensively(t *testing.T) {
	for status, want := range map[ScanStatus]string{
		ScanAvailable:        "recent",
		ScanQuarantined:      "needs_attention",
		ScanRejected:         "needs_attention",
		ScanStatus("future"): "needs_attention",
	} {
		got, label := classifyPackagingActivity(status)
		if got != want || label == "" {
			t.Errorf("status %q: classification = %q, label = %q", status, got, label)
		}
	}
}

func TestPackagingActivityIncludesSkillAndPluginArtifacts(t *testing.T) {
	pluginName := "release-helper"
	rows := []gen.ListDownloadArtifactsRow{
		{FileName: "skill.zip", ScanStatus: string(ScanAvailable)},
		{FileName: "plugin.zip", PluginName: &pluginName, ScanStatus: string(ScanAvailable)},
	}

	facts := packagingActivityFacts(rows)

	if len(facts) != 2 {
		t.Fatalf("facts = %d, want both Skill and Plugin artifacts", len(facts))
	}
	if facts[1].FileName != "plugin.zip" || facts[1].Classification != "recent" {
		t.Fatalf("plugin fact = %+v", facts[1])
	}
}
