package halt

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

var queuedAt = time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)

func minute(n int) time.Time { return queuedAt.Add(time.Duration(n) * time.Minute) }

func haltRow(provider string, declared int, lifted *int) gen.DispatchHalt {
	row := gen.DispatchHalt{
		Provider: provider, Source: string(OrphanThreshold),
		DeclaredAt: pgtype.Timestamptz{Time: minute(declared), Valid: true},
	}
	if lifted != nil {
		row.LiftedAt = pgtype.Timestamptz{Time: minute(*lifted), Valid: true}
	}
	return row
}

func at(n int) *int { return &n }

func TestPausedWithinCountsOnlyTimeWhenNoProviderCouldTakeTheRun(t *testing.T) {
	providers := []string{"a", "b"}
	for _, tc := range []struct {
		name string
		rows []gen.DispatchHalt
		want time.Duration
	}{
		{"no halt", nil, 0},
		{"a pool halt inside the window", []gen.DispatchHalt{haltRow(Pool, 5, at(20))}, 15 * time.Minute},
		{"a pool halt declared before the run queued and still held", []gen.DispatchHalt{haltRow(Pool, -10, nil)}, 40 * time.Minute},
		{"a pool halt lifted exactly when the run queued", []gen.DispatchHalt{haltRow(Pool, -10, at(0))}, 0},
		{"one of two providers halted", []gen.DispatchHalt{haltRow("a", 0, at(30))}, 0},
		{"both providers halted, overlapping for ten minutes", []gen.DispatchHalt{
			haltRow("a", 0, at(20)), haltRow("b", 10, at(30)),
		}, 10 * time.Minute},
		{"a provider halt nested in a pool halt is not counted twice", []gen.DispatchHalt{
			haltRow(Pool, 5, at(25)), haltRow("a", 10, at(15)), haltRow("b", 10, at(15)),
		}, 20 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := pausedWithin(tc.rows, providers, minute(0), minute(40)); got != tc.want {
				t.Errorf("paused = %v, want %v", got, tc.want)
			}
		})
	}
}
