package publishing

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestPublishingActivityDecisionTable(t *testing.T) {
	tests := []struct {
		name       string
		status     Status
		hasRelease bool
		want       string
	}{
		{"published with release", StatusPublished, true, "recent"},
		{"published without release", StatusPublished, false, "needs_attention"},
		{"delisted with release", StatusDelisted, true, "recent"},
		{"delisted without release", StatusDelisted, false, "recent"},
		{"future state", Status("future"), true, "needs_attention"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			latestVersionID := pgtype.UUID{Valid: test.hasRelease}
			got, label := classifyPublishingActivity(test.status, latestVersionID)
			if got != test.want || label == "" {
				t.Fatalf("classification = %q, label = %q", got, label)
			}
		})
	}
}

func TestPublishingActivityUsesTheLatestOwnerTimeWithAnExplicitFallback(t *testing.T) {
	statusChanged := time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)
	released := statusChanged.Add(time.Hour)
	for _, test := range []struct {
		name    string
		release pgtype.Timestamptz
		want    time.Time
	}{
		{"no release", pgtype.Timestamptz{}, statusChanged},
		{"newer release", pgtype.Timestamptz{Time: released, Valid: true}, released},
		{"older release", pgtype.Timestamptz{Time: statusChanged.Add(-time.Hour), Valid: true}, statusChanged},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := publicationActivityTime(
				pgtype.Timestamptz{Time: statusChanged, Valid: true}, test.release,
			)
			if !got.Equal(test.want) {
				t.Fatalf("activity time = %s, want %s", got, test.want)
			}
		})
	}
}
