package ingest

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestASourceCheckKeepsWhenItFirstWentAwayAndWhenItFirstChanged(t *testing.T) {
	now := time.Unix(2_000, 0)
	earlier := pgtype.Timestamptz{Time: time.Unix(1_000, 0), Valid: true}
	stamped := pgtype.Timestamptz{Time: now, Valid: true}
	cases := []struct {
		name                     string
		unavailableSince         pgtype.Timestamptz
		changedAt                pgtype.Timestamptz
		available, changed       bool
		wantUnavailable, wantChg pgtype.Timestamptz
	}{
		{"an available source stays clean", pgtype.Timestamptz{}, pgtype.Timestamptz{}, true, false, pgtype.Timestamptz{}, pgtype.Timestamptz{}},
		{"a source goes away", pgtype.Timestamptz{}, pgtype.Timestamptz{}, false, false, stamped, pgtype.Timestamptz{}},
		{"a source still away keeps its first absence", earlier, pgtype.Timestamptz{}, false, false, earlier, pgtype.Timestamptz{}},
		{"a source comes back", earlier, pgtype.Timestamptz{}, true, false, pgtype.Timestamptz{}, pgtype.Timestamptz{}},
		{"a source changes", pgtype.Timestamptz{}, pgtype.Timestamptz{}, true, true, pgtype.Timestamptz{}, stamped},
		{"a changed source keeps its first change", pgtype.Timestamptz{}, earlier, true, true, pgtype.Timestamptz{}, earlier},
		{"a change already seen stays when the source goes away", pgtype.Timestamptz{}, earlier, false, false, stamped, earlier},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := gen.ListSourcesToCheckRow{UnavailableSince: tc.unavailableSince, ContentChangedAt: tc.changedAt}

			got := checkedSource(row, tc.available, tc.changed, now)

			if got.UnavailableSince != tc.wantUnavailable || got.ContentChangedAt != tc.wantChg {
				t.Fatalf("unavailable since %v changed at %v, want %v and %v",
					got.UnavailableSince, got.ContentChangedAt, tc.wantUnavailable, tc.wantChg)
			}
		})
	}
}
