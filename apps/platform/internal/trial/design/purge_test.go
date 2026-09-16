package testlab

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestOnlyTestCasesNoRunFrozeAreErased(t *testing.T) {
	frozen := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	loose := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	for _, tc := range []struct {
		what        string
		testCases   []pgtype.UUID
		snapshotted []pgtype.UUID
		want        []pgtype.UUID
	}{
		{"no test cases", nil, nil, nil},
		{"none snapshotted", []pgtype.UUID{frozen, loose}, nil, []pgtype.UUID{frozen, loose}},
		{"one snapshotted", []pgtype.UUID{frozen, loose}, []pgtype.UUID{frozen}, []pgtype.UUID{loose}},
		{"all snapshotted", []pgtype.UUID{frozen, loose}, []pgtype.UUID{loose, frozen}, nil},
	} {
		if got := testCasesNoRunFroze(tc.testCases, tc.snapshotted); !slices.Equal(got, tc.want) {
			t.Errorf("%s: erasable = %v, want %v", tc.what, got, tc.want)
		}
	}
}
