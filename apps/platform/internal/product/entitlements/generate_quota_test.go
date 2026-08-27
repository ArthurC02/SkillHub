package policy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func generateReader(used int64, created time.Time) UsageReader {
	return UsageReader{
		WorkspaceCreatedAt: func(context.Context, pgtype.UUID) (time.Time, error) { return created, nil },
		CountRuns: func(context.Context, pgtype.UUID, time.Time) (RunUsage, error) {
			return RunUsage{Used: used}, nil
		},
	}
}

// ADR-047 決策 5. The two allowances share their arithmetic and must not share
// anything else: a caller that treats one exhaustion as the other's would let a
// generation refusal read as "you have used your free run allowance", and a
// deployment that turned one off would turn off both.
func TestTheTwoAllowancesAreNotTheSameAllowance(t *testing.T) {
	ctx := context.Background()
	old := time.Now().Add(-365 * 24 * time.Hour)
	exhausted := generateReader(1000, old)

	_, gerr := EnforceGenerateQuota(ctx, exhausted, DefaultGenerateQuotaLimits(), pgtype.UUID{})
	if !errors.Is(gerr, ErrGenerateQuotaExceeded) {
		t.Fatalf("generation refusal = %v, want ErrGenerateQuotaExceeded", gerr)
	}
	if errors.Is(gerr, ErrQuotaExceeded) {
		t.Error("a generation refusal also matched the run allowance's sentinel")
	}

	_, rerr := EnforceQuota(ctx, exhausted, DefaultQuotaLimits(), pgtype.UUID{})
	if !errors.Is(rerr, ErrQuotaExceeded) {
		t.Fatalf("run refusal = %v, want ErrQuotaExceeded", rerr)
	}
	if errors.Is(rerr, ErrGenerateQuotaExceeded) {
		t.Error("a run refusal also matched the generation allowance's sentinel")
	}

	// The sentence a user reads has to name what they ran out of. Sharing the
	// arithmetic is the point; sharing the noun would be the bug.
	if !strings.Contains(gerr.Error(), "generation") {
		t.Errorf("generation refusal does not say generation: %q", gerr)
	}
	if !strings.Contains(rerr.Error(), "run") {
		t.Errorf("run refusal does not say run: %q", rerr)
	}
}

// Zero limits enforce nothing and claim nothing — what GENERATE_QUOTA=off
// produces. Without this the "off" switch is one typo away from refusing
// everything, since a zero ceiling and an absent one look alike.
func TestGenerationAllowanceOffRefusesNothing(t *testing.T) {
	reason, err := EnforceGenerateQuota(context.Background(),
		generateReader(10_000, time.Now()), QuotaLimits{}, pgtype.UUID{})
	if err != nil || reason != "" {
		t.Fatalf("got (%q, %v), want no refusal", reason, err)
	}
}

// Fail closed: an allowance that could not be counted is not an allowance of
// zero used (02:SEC-002). The generation path calls this before spending money,
// so the failure direction here is the one that decides whether an unreadable
// database becomes a free gateway.
// Both allowances, because enforce() is one function and this is its behaviour,
// not either caller's.
//
// It must refuse, and it must NOT say the workspace ran out. Both handlers turn
// their exceeded sentinel into a 422 carrying err.Error(), so wrapping that
// sentinel here told a user with a healthy account that they had used up an
// allowance — and appended the pgx error, connection string included, to a
// response body (NFR-002, iron rule 11).
func TestAnUncountableAllowanceRefusesWithoutClaimingItRanOut(t *testing.T) {
	broken := UsageReader{
		WorkspaceCreatedAt: func(context.Context, pgtype.UUID) (time.Time, error) {
			return time.Time{}, errors.New("password=hunter2 host=db.internal: connection refused")
		},
		CountRuns: func(context.Context, pgtype.UUID, time.Time) (RunUsage, error) {
			return RunUsage{}, nil
		},
	}
	for _, tc := range []struct {
		name     string
		call     func() (string, error)
		exceeded error
		reason   string
	}{
		{"generations", func() (string, error) {
			return EnforceGenerateQuota(context.Background(), broken, DefaultGenerateQuotaLimits(), pgtype.UUID{})
		}, ErrGenerateQuotaExceeded, "generate_quota_unavailable"},
		{"runs", func() (string, error) {
			return EnforceQuota(context.Background(), broken, DefaultQuotaLimits(), pgtype.UUID{})
		}, ErrQuotaExceeded, "quota_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reason, err := tc.call()
			if !errors.Is(err, ErrAllowanceUnavailable) {
				t.Fatalf("err = %v, want ErrAllowanceUnavailable", err)
			}
			if errors.Is(err, tc.exceeded) {
				t.Errorf("an uncountable allowance must not read as an exhausted one: %v", err)
			}
			if reason != tc.reason {
				t.Errorf("reason = %q, want %q", reason, tc.reason)
			}
		})
	}
}

// The generation allowance's four numbers, asserted the way the run allowance's
// four are next door.
//
// It is here because until 2026-08-27 they were the only ratified numbers in this
// package with nothing holding them: quota_test.go has guarded PDM-010's four
// since they were a proposal, and these had no counterpart — a signature with no
// teeth. That asymmetry was found while ratifying them, not by a review.
//
// The literals are repeated rather than read back through the constants on
// purpose. A test that reaches the value through the symbol it guards passes at
// every value, which is exactly how a 32 MiB import ceiling survived for months
// against three documents saying 10 MB (04 乙-23).
func TestGenerateDefaultsAreTheRatifiedNumbers(t *testing.T) {
	l := DefaultGenerateQuotaLimits()
	if l.Daily != 10 || l.Window != 30 || l.FirstWindow != 20 || l.WindowDays != 30 {
		t.Errorf("generation allowance drifted from the 2026-08-27 ratification "+
			"(m0/pdm-proposals.md §9.1, 05 R-9): %+v", l)
	}
	// Higher than the run allowance's daily five, and that ordering is the
	// reasoning ratified with the numbers: a user who does not like what came
	// back rewrites the task description and goes again, and a generation costs
	// about a seventh of a run at the gateway.
	if l.Daily <= DefaultQuotaLimits().Daily {
		t.Errorf("generation daily = %d, run daily = %d; the ratified numbers put generation higher",
			l.Daily, DefaultQuotaLimits().Daily)
	}
}
