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
func TestAnUncountableGenerationAllowanceRefuses(t *testing.T) {
	broken := UsageReader{
		WorkspaceCreatedAt: func(context.Context, pgtype.UUID) (time.Time, error) {
			return time.Time{}, errors.New("database is down")
		},
		CountRuns: func(context.Context, pgtype.UUID, time.Time) (RunUsage, error) {
			return RunUsage{}, nil
		},
	}
	reason, err := EnforceGenerateQuota(context.Background(), broken, DefaultGenerateQuotaLimits(), pgtype.UUID{})
	if !errors.Is(err, ErrGenerateQuotaExceeded) {
		t.Fatalf("err = %v, want ErrGenerateQuotaExceeded", err)
	}
	if reason != "generate_quota_unavailable" {
		t.Errorf("reason = %q", reason)
	}
}
