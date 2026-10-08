package credit

import (
	"context"
	"errors"
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
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/testschema"
)

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("SKILLHUB_TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("SKILLHUB_REQUIRE_DB") == "1" {
			t.Fatal("SKILLHUB_REQUIRE_DB=1 but SKILLHUB_TEST_DATABASE_URL is unset")
		}
		t.Skip("SKILLHUB_TEST_DATABASE_URL not set; skipping credit store integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	unlock, err := testschema.Lock(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(unlock)
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

func TestAnAdjustmentIsRefusedExactlyWhenItWouldPassTheLowestStoredBalance(t *testing.T) {
	pool := integrationPool(t)
	for _, tc := range []struct {
		name        string
		credits     int64
		wantApplied bool
		wantBalance int64
	}{
		{"down to the lowest stored balance is applied", MinBalanceCredits, true, MinBalanceCredits},
		{"one credit past the lowest stored balance is refused", MinBalanceCredits - 1, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
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

			balance, applied, err := NewPostgresStore(pool).ApplyGrant(ctx, tx, GrantEntry{
				UserID: user, EntryKind: EntryAdjustment, Credits: tc.credits,
				Reason: "fixture", IdempotencyKey: uuid.NewString(),
			})

			if tc.wantApplied && err != nil {
				t.Fatalf("ApplyGrant(%d) = %v, want it applied", tc.credits, err)
			}
			if !tc.wantApplied && !errors.Is(err, ErrBalanceOutOfRange) {
				t.Fatalf("ApplyGrant(%d) error = %v, want ErrBalanceOutOfRange", tc.credits, err)
			}
			if applied != tc.wantApplied || balance != tc.wantBalance {
				t.Fatalf("ApplyGrant(%d) = (balance %d, applied %v), want (%d, %v)", tc.credits, balance, applied, tc.wantBalance, tc.wantApplied)
			}
		})
	}
}

func TestAReplayedChargeKeyRecordsAndDebitsOnce(t *testing.T) {
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
	s := &Service{Store: NewPostgresStore(pool), Config: testConfig()}
	usd := int64(10_000)
	key := "creation:" + uuid.NewString() + ":1"
	in := ChargeInput{
		Kind: KindCreationStep, UserID: user, RefType: RefCreationSession, RefID: user,
		IdempotencyKey: key, UsdMicros: &usd, ReservedUsdMicros: usd,
	}

	first, err := s.Charge(ctx, tx, in)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.Charge(ctx, tx, in)
	if err != nil {
		t.Fatal(err)
	}

	if first.Existed || first.Credits != 13 || first.NewBalance != -13 {
		t.Fatalf("first charge = %+v, want a new 13-credit debit leaving -13", first)
	}
	if !replay.Existed || replay.NewBalance != -13 {
		t.Fatalf("replayed charge = %+v, want it reported as existing with the balance still -13", replay)
	}
	var events, debits int
	var balance int64
	if err := tx.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM cost_events WHERE idempotency_key = $1),
		       (SELECT count(*) FROM credit_entries WHERE idempotency_key = $1),
		       (SELECT balance_credits FROM credit_accounts WHERE user_id = $2)`,
		key, user).Scan(&events, &debits, &balance); err != nil {
		t.Fatal(err)
	}
	if events != 1 || debits != 1 || balance != -13 {
		t.Fatalf("after a replay: %d cost events, %d ledger entries, balance %d; want 1, 1, -13", events, debits, balance)
	}
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
