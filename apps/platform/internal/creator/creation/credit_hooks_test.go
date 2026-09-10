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

func TestCreditCanStartBlocksBeforeAnyDatabaseWork(t *testing.T) {
	s := &Service{CreditCanStart: func(ctx context.Context, workspaceID pgtype.UUID) (bool, error) {
		return false, nil
	}}
	_, err := s.Create(context.Background(), identity.Workspace{}, pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, "hello", 1)
	if !errors.Is(err, ErrCreditThreshold) {
		t.Fatalf("Create() error = %v, want ErrCreditThreshold", err)
	}
}

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

func TestCreditCanStartNilSkipsTheGate(t *testing.T) {
	s := &Service{}
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
