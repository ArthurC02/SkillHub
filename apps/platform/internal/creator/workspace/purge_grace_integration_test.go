package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

func TestAPurgeClaimHonoursTheGraceOfTheDeletionRequestStandingAtClaimTime(t *testing.T) {
	pool := signupPool(t)
	ctx := context.Background()
	var userID pgtype.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, display_name, deletion_requested_at) VALUES ($1, 'fixture', now()) RETURNING id`,
		"purge-grace-"+uuid.NewString()+"@example.invalid").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID) })
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	s := &Service{Pool: pool}
	started := func() bool {
		var v bool
		if err := pool.QueryRow(ctx, `SELECT purge_started_at IS NOT NULL FROM users WHERE id = $1`, userID).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}

	sweepCutoff := pgconv.Timestamptz(time.Now().Add(-30 * 24 * time.Hour))
	if err := s.claimAccountPurge(ctx, conn, userID, nil, sweepCutoff); !errors.Is(err, errAccountPurgeDeferred) || started() {
		t.Errorf("a request newer than the sweep's cutoff: err %v, started %v; want deferred and not started", err, started())
	}
	if err := s.claimAccountPurge(ctx, conn, userID, nil, pgconv.Timestamptz(time.Now().Add(time.Minute))); err != nil || !started() {
		t.Errorf("a request past the cutoff: err %v, started %v; want started", err, started())
	}
}
