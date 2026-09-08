package creation

import (
	"context"
	"errors"
	"testing"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/jackc/pgx/v5/pgtype"
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
