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

	if !strings.Contains(gerr.Error(), "generation") {
		t.Errorf("generation refusal does not say generation: %q", gerr)
	}
	if !strings.Contains(rerr.Error(), "run") {
		t.Errorf("run refusal does not say run: %q", rerr)
	}
}

func TestGenerationAllowanceOffRefusesNothing(t *testing.T) {
	reason, err := EnforceGenerateQuota(context.Background(),
		generateReader(10_000, time.Now()), QuotaLimits{}, pgtype.UUID{})
	if err != nil || reason != "" {
		t.Fatalf("got (%q, %v), want no refusal", reason, err)
	}
}

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

func TestGenerateDefaultsAreTheRatifiedNumbers(t *testing.T) {
	l := DefaultGenerateQuotaLimits()
	if l.Daily != 10 || l.Window != 30 || l.FirstWindow != 20 || l.WindowDays != 30 {
		t.Errorf("generation allowance drifted from the 2026-08-27 ratification "+
			"(m0/pdm-proposals.md §9.1, 05 R-9): %+v", l)
	}

	if l.Daily <= DefaultQuotaLimits().Daily {
		t.Errorf("generation daily = %d, run daily = %d; the ratified numbers put generation higher",
			l.Daily, DefaultQuotaLimits().Daily)
	}
}
