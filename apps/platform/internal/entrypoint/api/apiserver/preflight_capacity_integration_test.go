package apiserver_test

import (
	"net/http"
	"testing"
	"time"

	policy "github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func preflightBlocked(t *testing.T, f fixture) string {
	t.Helper()
	code, view := f.preflight(t)
	if code != http.StatusOK {
		t.Fatalf("preflight: got %d, want 200", code)
	}
	return view.Blocked
}

func TestPreflightNamesAWorkspaceThatHasNoRunSlotLeft(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-preflight-concurrency")

	if got := preflightBlocked(t, f); got != "" {
		t.Fatalf("preflight with every slot free says blocked=%q, want nothing", got)
	}
	hash := f.confirmPermissions(t)
	for i := 1; i <= run.MaxConcurrentRunsPerWorkspace; i++ {
		if code, view := f.startWithHash(t, hash); code != http.StatusCreated {
			t.Fatalf("filling slot %d: got %d (%s)", i, code, view.Error)
		}
	}

	if got := preflightBlocked(t, f); got != "workspace_concurrency" {
		t.Errorf("preflight with every slot taken says blocked=%q, want workspace_concurrency", got)
	}
}

func TestPreflightNamesASpentRunAllowanceBeforeTheUserConfirms(t *testing.T) {
	for _, tc := range []struct {
		name    string
		limits  policy.QuotaLimits
		counted int
		want    string
	}{
		{"one run left today", policy.QuotaLimits{Daily: 2, Window: 30, FirstWindow: 30, WindowDays: 30}, 1, ""},
		{"daily allowance spent", policy.QuotaLimits{Daily: 2, Window: 30, FirstWindow: 30, WindowDays: 30}, 2, "quota_daily"},
		{"window allowance spent", policy.QuotaLimits{Daily: 50, Window: 2, FirstWindow: 2, WindowDays: 30}, 2, "quota_window"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := requireDB(t)
			a := betaAPI(t, pool, tc.limits, nil, 0)
			f := newFixture(t, a, pool, "alice-preflight-quota-"+tc.want)
			for range tc.counted {
				seedCountedRun(t, pool, f, "", time.Now().Add(-time.Hour))
			}

			if got := preflightBlocked(t, f); got != tc.want {
				t.Errorf("preflight after %d counted runs says blocked=%q, want %q", tc.counted, got, tc.want)
			}
		})
	}
}
