package halt

import (
	"context"
	"slices"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

func (l Ledger) PausedWithin(ctx context.Context, providers []string, from, to time.Time) (time.Duration, error) {
	rows, err := l.queries().ListDispatchHaltsOverlapping(ctx, gen.ListDispatchHaltsOverlappingParams{
		Since: pgconv.Timestamptz(from), Until: pgconv.Timestamptz(to),
	})
	if err != nil {
		return 0, err
	}
	return pausedWithin(rows, providers, from, to), nil
}

func pausedWithin(rows []gen.DispatchHalt, providers []string, from, to time.Time) time.Duration {
	// Between two consecutive declare or lift instants the set of active halts
	// is constant, so each segment is paused or not as a whole.
	cuts := []time.Time{from, to}
	for _, row := range rows {
		cuts = appendWithin(cuts, row.DeclaredAt.Time, from, to)
		if row.LiftedAt.Valid {
			cuts = appendWithin(cuts, row.LiftedAt.Time, from, to)
		}
	}
	slices.SortFunc(cuts, func(a, b time.Time) int { return a.Compare(b) })
	var paused time.Duration
	for i := 0; i+1 < len(cuts); i++ {
		start, end := cuts[i], cuts[i+1]
		if end.After(start) && StateOf(activeAt(rows, start)...).DispatchPaused(providers) {
			paused += end.Sub(start)
		}
	}
	return paused
}

func appendWithin(cuts []time.Time, at, from, to time.Time) []time.Time {
	if at.After(from) && at.Before(to) {
		return append(cuts, at)
	}
	return cuts
}

func activeAt(rows []gen.DispatchHalt, at time.Time) []gen.DispatchHalt {
	var active []gen.DispatchHalt
	for _, row := range rows {
		if !row.DeclaredAt.Time.After(at) && (!row.LiftedAt.Valid || row.LiftedAt.Time.After(at)) {
			active = append(active, row)
		}
	}
	return active
}
