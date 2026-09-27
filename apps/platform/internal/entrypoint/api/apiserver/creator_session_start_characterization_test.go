package apiserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
)

func TestAStartThatLosesTheRaceToTheSameStartResumesTheWinner(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	removeCreationWorkspace(t, pool, ws)
	svc := &creation.Service{Pool: pool, Limits: creationLimits()}
	ctx := context.Background()
	template := creationID(t)
	if _, err := svc.Create(ctx, ws, template, "", .5); err != nil {
		t.Fatal(err)
	}

	id := creationID(t)
	winner, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = winner.Rollback(ctx) }()
	if _, err := winner.Exec(ctx, `INSERT INTO creation_sessions (id, workspace_id, revision, state, snapshot, expires_at)
		SELECT $1, workspace_id, revision, state, snapshot, expires_at FROM creation_sessions WHERE id = $2`, id, template); err != nil {
		t.Fatal(err)
	}
	var winnerPID int32
	if err := winner.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&winnerPID); err != nil {
		t.Fatal(err)
	}

	type started struct {
		view creation.View
		err  error
	}
	loser := make(chan started, 1)
	go func() {
		v, err := svc.Create(ctx, ws, id, "", .5)
		loser <- started{v, err}
	}()

	deadline := time.Now().Add(10 * time.Second)
	for {
		var blocked bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE $1::int = ANY(pg_blocking_pids(pid)))`, winnerPID).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the second start never waited on the first")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := winner.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	got := <-loser
	if got.err != nil {
		t.Fatalf("losing start err = %v, want it to resume the winner", got.err)
	}
	if got.view.ID != creation.UUID(id) || got.view.State != "waiting_input" {
		t.Errorf("losing start view = %s %s, want %s waiting_input", got.view.ID, got.view.State, creation.UUID(id))
	}
}
