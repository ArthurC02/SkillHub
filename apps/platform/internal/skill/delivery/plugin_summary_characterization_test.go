package packaging

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestAPluginDownloadCarriesTheFirstMemberRedistributionThatRefuses(t *testing.T) {
	allowed := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	blocked := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	unknown := pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	summaries := map[pgtype.UUID]VersionSummary{
		allowed: {SkillName: "a", Redistribution: string(RedistributionAllowed)},
		blocked: {SkillName: "b", Redistribution: string(RedistributionBlocked)},
		unknown: {SkillName: "c", Redistribution: "unknown"},
	}

	combined, view, ok := pluginSummary("p", "1.0.0", []pgtype.UUID{allowed, blocked, unknown}, summaries)

	if !ok || combined.Redistribution != string(RedistributionBlocked) || combined.AccessRestricted {
		t.Fatalf("ok=%v combined=%+v, want the first refusing member's %q and no hold", ok, combined, RedistributionBlocked)
	}
	if reason, _ := gate(SkillFacts{Redistribution: combined.Redistribution}); reason != BlockedNotRedistributable {
		t.Fatalf("the plugin download gate answered %q, want %q", reason, BlockedNotRedistributable)
	}
	if len(view.Members) != 3 || view.Members[1].Name != "b" {
		t.Fatalf("members = %+v, want all three in order", view.Members)
	}
}

func TestAPluginDownloadOfReleasingMembersStaysAllowed(t *testing.T) {
	own := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	generated := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	summaries := map[pgtype.UUID]VersionSummary{
		own:       {SkillName: "a", Redistribution: string(RedistributionSelfSupplied)},
		generated: {SkillName: "b", Redistribution: string(RedistributionGenerated)},
	}

	combined, _, ok := pluginSummary("p", "1.0.0", []pgtype.UUID{own, generated}, summaries)

	if !ok || combined.Redistribution != string(RedistributionAllowed) {
		t.Fatalf("ok=%v combined=%+v, want %q", ok, combined, RedistributionAllowed)
	}
}
