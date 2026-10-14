package readmodel

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type RunsOnDay struct {
	Day    time.Time
	Status string
	Runs   int64
}

type RunWorkspacesOnDay struct {
	Day        time.Time
	Workspaces int64
}

func (r Reader) DailyRunWorkspaces(ctx context.Context, since time.Time) ([]RunWorkspacesOnDay, error) {
	rows, err := r.queries().CountRunWorkspacesByDay(ctx, pgtype.Timestamptz{Time: since, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]RunWorkspacesOnDay, 0, len(rows))
	for _, r := range rows {
		out = append(out, RunWorkspacesOnDay{Day: r.Day.Time, Workspaces: r.Workspaces})
	}
	return out, nil
}

func (r Reader) DailyRuns(ctx context.Context, since time.Time) ([]RunsOnDay, error) {
	rows, err := r.queries().CountRunsByDay(ctx, pgtype.Timestamptz{Time: since, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]RunsOnDay, 0, len(rows))
	for _, r := range rows {
		out = append(out, RunsOnDay{Day: r.Day.Time, Status: r.Status, Runs: r.Runs})
	}
	return out, nil
}
