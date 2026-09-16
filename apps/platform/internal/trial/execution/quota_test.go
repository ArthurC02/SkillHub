package run

import (
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestRequireQuotaRequiresTransaction(t *testing.T) {
	err := (&Service{}).requireQuota(t.Context(), nil, pgtype.UUID{})
	if !errors.Is(err, errQuotaTransactionRequired) {
		t.Fatalf("requireQuota() error = %v, want %v", err, errQuotaTransactionRequired)
	}
}

func TestOnlyFailuresOfTheRunItselfCountAgainstTheQuota(t *testing.T) {
	for _, tc := range []struct {
		class  FailureClass
		counts bool
	}{
		{failureWorkload, true},
		{failureTimeout, true},
		{failureCancelled, true},
		{failureProvider, false},
		{failurePlatform, false},
		{failureNoProvider, false},
	} {
		if got := tc.class.CountsAgainstQuota(); got != tc.counts {
			t.Errorf("%s counts against the quota = %v, want %v", tc.class, got, tc.counts)
		}
	}
	if got, want := quotaExemptFailureClasses(), []string{"provider_error", "capability_mismatch", "platform_error"}; !slices.Equal(got, want) {
		t.Errorf("exempt failure classes = %v, want %v", got, want)
	}
	if quotaCountsFromStatus != "preparing" {
		t.Errorf("a run counts from %q, want preparing", quotaCountsFromStatus)
	}
}
