package testlab

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestASnapshotCanBeRerunOnlyWhileItsTestCaseAndEveryDatasetAreStillThere(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) pgtype.Timestamptz { return pgtype.Timestamptz{Time: now.Add(d), Valid: true} }
	first := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	second := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	live := func(id pgtype.UUID) gen.ListDatasetLifetimesRow {
		return gen.ListDatasetLifetimesRow{ID: id, ExpiresAt: at(time.Second)}
	}
	for _, tc := range []struct {
		what        string
		testCaseDel pgtype.Timestamptz
		refs        []pgtype.UUID
		lifetimes   []gen.ListDatasetLifetimesRow
		want        bool
	}{
		{"no datasets and a live test case", pgtype.Timestamptz{}, nil, nil, true},
		{"the test case was deleted", at(-time.Hour), nil, nil, false},
		{"every dataset is still live", pgtype.Timestamptz{}, []pgtype.UUID{first, second}, []gen.ListDatasetLifetimesRow{live(first), live(second)}, true},
		{"one dataset row is gone", pgtype.Timestamptz{}, []pgtype.UUID{first, second}, []gen.ListDatasetLifetimesRow{live(first)}, false},
		{"one dataset was deleted", pgtype.Timestamptz{}, []pgtype.UUID{first}, []gen.ListDatasetLifetimesRow{{ID: first, ExpiresAt: at(time.Hour), DeletedAt: at(-time.Minute)}}, false},
		{"one dataset expires this very moment", pgtype.Timestamptz{}, []pgtype.UUID{first}, []gen.ListDatasetLifetimesRow{{ID: first, ExpiresAt: at(0)}}, false},
	} {
		if got := snapshotInputsAvailable(tc.testCaseDel, tc.refs, tc.lifetimes, now); got != tc.want {
			t.Errorf("%s: available = %v, want %v", tc.what, got, tc.want)
		}
	}
}
