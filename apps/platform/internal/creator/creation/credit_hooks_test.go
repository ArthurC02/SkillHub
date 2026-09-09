package creation

import (
	"context"
	"errors"
	"testing"
	"time"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestCreditCanStartBlocksBeforeAnyDatabaseWork proves ADR-068 gate ① runs
// before Create touches anything else: s.Pool is left nil here, so if the
// gate were checked even one line later this would panic on a nil pointer
// instead of returning ErrCreditThreshold.
func TestCreditCanStartBlocksBeforeAnyDatabaseWork(t *testing.T) {
	s := &Service{CreditCanStart: func(ctx context.Context, workspaceID pgtype.UUID) (bool, error) {
		return false, nil
	}}
	_, err := s.Create(context.Background(), identity.Workspace{}, pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, "hello", 1)
	if !errors.Is(err, ErrCreditThreshold) {
		t.Fatalf("Create() error = %v, want ErrCreditThreshold", err)
	}
}

// TestCreditCanStartPropagatesItsError proves a lookup failure (e.g. the
// balance could not be read) is surfaced as-is, not swallowed into a
// generic refusal that would look identical to "balance too low".
func TestCreditCanStartPropagatesItsError(t *testing.T) {
	wantErr := errors.New("boom")
	s := &Service{CreditCanStart: func(ctx context.Context, workspaceID pgtype.UUID) (bool, error) {
		return false, wantErr
	}}
	_, err := s.Create(context.Background(), identity.Workspace{}, pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, "hello", 1)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Create() error = %v, want %v", err, wantErr)
	}
}

// TestCreditCanStartNilSkipsTheGate proves an unwired hook changes nothing:
// with CreditCanStart nil, Create runs its existing checks unaffected (here,
// the existing ErrUnavailable from invalid Limits — same assertion
// TestLimitsFailClosed already makes without this field ever being set).
func TestCreditCanStartNilSkipsTheGate(t *testing.T) {
	s := &Service{} // no CreditCanStart, no valid Limits
	_, err := s.Create(context.Background(), identity.Workspace{}, pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, "hello", 1)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Create() error = %v, want ErrUnavailable (the pre-existing check, unaffected by the unset credit hook)", err)
	}
}

func TestUSDMicrosRounding(t *testing.T) {
	cases := []struct {
		usd  float64
		want int64
	}{
		{0.1, 100_000},
		{0, 0},
		{-1, 0},
	}
	for _, c := range cases {
		if got := usdMicros(c.usd); got != c.want {
			t.Errorf("usdMicros(%v) = %d, want %d", c.usd, got, c.want)
		}
	}
}

// TestCatalogCheckRunsBeforeTheTransaction proves the fix for a self-deadlock
// that had no symptom other than 「按下送出後，沒有任何反應」.
//
// CatalogCheck is a POOL USER: `CreationKnowledgeIDs` is a pgvector retrieval
// and `ResolveReference` reads a version per hit (apiserver/creation_wiring.go).
// It used to be called with a transaction already open, so the request held one
// connection and then asked for a second. On a deployment with a large pool that
// is invisible. Clean mode pins pgxpool to MaxConns=1 (ADR-060 決策 6, because
// the PGlite carrier serves one client at a time), and there it is a certain
// deadlock: POST /creation-sessions never answers, River's elector and producer
// starve on the same connection, and the only thing that ever unblocks it is the
// browser giving up and cancelling the request context. Reproduced 2026-09-09.
//
// The pool here is unreachable on purpose, and that is the whole fixture: Begin
// cannot succeed, so Create must fail — the question this test asks is whether
// the catalogue check already ran by then. Before the fix it never did, because
// Begin came first and returned the error.
func TestCatalogCheckRunsBeforeTheTransaction(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://skillhub@127.0.0.1:1/skillhub")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	checked := false
	s := &Service{
		Pool:   pool,
		Limits: Limits{MaxCostUSD: 1, MaxCallCostUSD: 0.1, MaxSteps: 24, MaxToolCalls: 8, CallTimeout: time.Second, SessionTimeout: time.Hour, Retention: time.Hour, MaxOutputTokens: 16000},
		CatalogCheck: func(context.Context, identity.Workspace, string) ([]Reference, float64, error) {
			checked = true
			return nil, 0, errors.New("catalogue unreachable in this test")
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := s.Create(ctx, identity.Workspace{}, pgtype.UUID{Bytes: [16]byte{2}, Valid: true}, "摘出文件中所有數字", 0.2); err == nil {
		t.Fatal("Create() succeeded against an unreachable pool")
	}
	if !checked {
		t.Fatal("the catalogue check never ran: it is still inside the transaction, and with MaxConns=1 that is a deadlock")
	}
}
