package credit

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("SKILLHUB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SKILLHUB_TEST_DATABASE_URL not set; skipping credit store integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// savepointTx keeps a failed statement from aborting the surrounding test transaction, as autocommit does in production.
type savepointTx struct{ pgx.Tx }

func (s savepointTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	sp, err := s.Begin(ctx)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	tag, err := sp.Exec(ctx, sql, args...)
	if err != nil {
		_ = sp.Rollback(ctx)
		return tag, err
	}
	return tag, sp.Commit(ctx)
}

func seedSessionStep(t *testing.T, ctx context.Context, tx pgx.Tx, user, session pgtype.UUID, usdMicros int64) {
	t.Helper()
	if _, err := tx.Exec(ctx, `
		INSERT INTO cost_events (kind, model, usd_micros, cost_source, user_id, ref_type, ref_id, idempotency_key)
		VALUES ('creation_step', 'fixture-model', $1, 'gateway', $2, 'creation_session', $3, $4)`,
		usdMicros, user, session, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
}

func TestOneSessionThatCannotBeSummarizedDoesNotStopTheOthers(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var user pgtype.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (email, display_name) VALUES ($1, 'fixture') RETURNING id`,
		uuid.NewString()+"@example.test").Scan(&user); err != nil {
		t.Fatal(err)
	}
	uuidOf := func(s string) pgtype.UUID {
		var id pgtype.UUID
		if err := id.Scan(s); err != nil {
			t.Fatal(err)
		}
		return id
	}
	overflowing := uuidOf("00000000-0000-4000-8000-" + strings.Repeat("0", 11) + "1")
	goodA := uuidOf("ffffffff-0000-4000-8000-" + strings.Repeat("0", 11) + "1")
	goodB := uuidOf("ffffffff-0000-4000-8000-" + strings.Repeat("0", 11) + "2")
	const aboveHalfOfInt64 = 5_000_000_000_000_000_000
	seedSessionStep(t, ctx, tx, user, overflowing, aboveHalfOfInt64)
	seedSessionStep(t, ctx, tx, user, overflowing, aboveHalfOfInt64)
	seedSessionStep(t, ctx, tx, user, goodA, 3_000)
	seedSessionStep(t, ctx, tx, user, goodB, 4_000)

	store := NewPostgresStore(pool)
	written, err := store.summarizeSessions(ctx, gen.New(savepointTx{tx}), []pgtype.UUID{overflowing, goodA, goodB})

	if err == nil || !strings.Contains(err.Error(), uuid.UUID(overflowing.Bytes).String()) {
		t.Fatalf("error = %v, want it to name the session that could not be summarized", err)
	}
	if written != 2 {
		t.Fatalf("summaries written = %d, want the 2 healthy sessions", written)
	}
	for id, want := range map[pgtype.UUID]int{overflowing: 0, goodA: 1, goodB: 1} {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM cost_session_summaries WHERE session_id = $1`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Errorf("summaries for session %s = %d, want %d", fmt.Sprint(uuid.UUID(id.Bytes)), n, want)
		}
	}
}
