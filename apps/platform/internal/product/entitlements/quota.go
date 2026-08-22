package policy

// The PDM-010 free run allowance, and the rule the one enforcement point applies
// (ADR-028 決策 2).
//
// This file exists because of a mistake the project already made once. PDM-005's
// token budget was written into policy_snapshot, shown on the pre-run permission
// summary, and asked the user to confirm it — while neither the platform nor the
// sandbox enforced any of it (04 乙-2). That was judged the worst of the available
// states, because it puts a number on the screen that NFR-001 then makes a lie.
//
// The allowance has the same trap in a different shape. Three mechanisms on the
// model gateway look like brakes and none of them is a quota:
//
//	Virtual Key max_budget  one run's money. A new key per attempt (SBX-008), so
//	                        it never accumulates: thirty $0.30 runs all pass
//	tpm_limit               rate. Not total — it cannot stop 300 runs in a month
//	workspace concurrency   how many at once (run/gateb.go). Two at a time still
//	                        allows three hundred in a month
//
// So the counter has to be the platform's own, and it belongs exactly where gate
// B's concurrency check already is: inside the create-run transaction, under the
// same advisory lock. Not for convenience — that is the only point in the system
// where a run is about to exist and nothing has been spent on it yet. Owning the
// rule here (DDD-014) did not move that point: EnforceQuota is called from inside
// that transaction and nowhere else.
//
// Two rules follow, and both are load bearing:
//
//   - No balance column. The count is a time-window query over the runs
//     themselves, exactly as the concurrency limit counts non-terminal runs rather
//     than keeping a counter. A balance drifts, and refunding one would be a write
//     that a crash, a restart or a failed cleanup can miss. Here a refund is a
//     predicate in CountQuotaRuns, so the paths nobody wrote get it right too.
//   - Enforcement ships before display. GET /me/quota and the quota block on the
//     pre-run summary read this file; they do not compute anything of their own.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ErrQuotaExceeded is the allowance refusal. Like the other gate B conditions it
// becomes a 422: the request is well formed and the platform is working, the
// account simply has nothing left in this window.
var ErrQuotaExceeded = errors.New("this workspace has used its free run allowance")

// PDM-010 §8.1's proposed numbers.
//
// 待追認 — every one of them. PDM-010 is a proposal, and it names one place where
// the owner must choose explicitly rather than let an implementation infer:
// whether the first window is min(20,30) = 20 or 20+30 = 50 ("請負責人明確擇一,
// 不要留給實作推斷"). The value below is the proposal's own recommendation, 20.
//
// ADR-028 決策 4 permits exactly this split: the enforcement point may be built
// before the numbers are ratified, because it does not depend on them — what may
// not happen before ratification is putting an unratified number on a screen. A
// deployment that does not want to show one leaves the allowance unconfigured,
// and then nothing is enforced and nothing is displayed (see QuotaLimits.Enforced).
const (
	quotaFirstWindowRuns = 20 // first 30 days, min(20,30) per PDM-010 — 待追認
	quotaWindowRuns      = 30 // per rolling window thereafter — 待追認
	quotaDailyRuns       = 5  // all the time, first window included — 待追認
	quotaWindowDays      = 30 // the rolling window's length — 待追認
)

// QuotaLimits is the allowance as a deployment configures it. The zero value
// enforces nothing, which is what a build with no allowance is: the API never
// refuses on quota grounds and never claims one exists.
type QuotaLimits struct {
	// Daily is runs per rolling 24 hours.
	Daily int
	// Window is runs per rolling window once the workspace is older than one.
	Window int
	// FirstWindow is the allowance of a workspace's first window. PDM-010 gives a
	// new account less than an established one, because a fresh account is the
	// main surface for burning allowance across many signups.
	FirstWindow int
	// WindowDays is the rolling window's length. Zero disables the whole thing.
	WindowDays int
}

// DefaultQuotaLimits is PDM-010 §8.1 as proposed. See the constants above for why
// every number in it is marked 待追認.
func DefaultQuotaLimits() QuotaLimits {
	return QuotaLimits{
		Daily:       quotaDailyRuns,
		Window:      quotaWindowRuns,
		FirstWindow: quotaFirstWindowRuns,
		WindowDays:  quotaWindowDays,
	}
}

// Enforced reports whether this deployment applies an allowance at all. A window
// length of zero means no: nothing is counted, nothing is refused, and no quota
// appears on the pre-run summary. Absent rather than zeroed — a number on that
// screen is a claim that it is applied (04 乙-2).
func (l QuotaLimits) Enforced() bool { return l.WindowDays > 0 && (l.Daily > 0 || l.Window > 0) }

// QuotaState is the allowance as it stands for one workspace: the RunQuota schema
// of contracts/openapi/public.yaml.
type QuotaState struct {
	RemainingToday  int
	RemainingWindow int
	WindowResetsAt  time.Time
	Limits          QuotaReport
}

// QuotaView is QuotaState on the wire: the RunQuota schema of
// contracts/openapi/public.yaml. Separate from QuotaState only because
// window_resets_at is an RFC3339 string there and a time.Time here.
type QuotaView struct {
	RemainingToday  int         `json:"remaining_today"`
	RemainingWindow int         `json:"remaining_window"`
	WindowResetsAt  string      `json:"window_resets_at"`
	Limits          QuotaReport `json:"limits"`
}

// View renders the state for GET /me/quota and for the pre-run summary's quota
// block — the same object in both places, because they are the same fact.
func (s QuotaState) View() QuotaView {
	return QuotaView{
		RemainingToday:  s.RemainingToday,
		RemainingWindow: s.RemainingWindow,
		WindowResetsAt:  s.WindowResetsAt.UTC().Format(time.RFC3339),
		Limits:          s.Limits,
	}
}

// QuotaReport is the four limits in force, together, so a user sees all of them
// in one place instead of discovering the concurrency one by hitting it.
//
// Concurrent is the one field Usage does not fill: the workspace concurrency
// ceiling belongs to Run Orchestration (run.MaxConcurrentRunsPerWorkspace), and
// run sets it on the way to the screen. Putting all four here is a display
// decision; owning three of them is this context's.
type QuotaReport struct {
	Daily      int `json:"daily"`
	Window     int `json:"window"`
	WindowDays int `json:"window_days"`
	Concurrent int `json:"concurrent"`
}

// RunUsage is the run-owned fact the quota rule consumes, without exposing a
// generated query row across the context boundary.
type RunUsage struct {
	Used   int64
	Oldest *time.Time
}

// UsageReader is the two-owner read port needed by the quota calculation.
// Callers choose transaction-backed or pool-backed functions without policy
// knowing either persistence type.
type UsageReader struct {
	WorkspaceCreatedAt func(context.Context, pgtype.UUID) (time.Time, error)
	CountRuns          func(context.Context, pgtype.UUID, time.Time) (RunUsage, error)
}

// Usage counts what this workspace has already spent, over both spans.
//
// reader is transaction-backed for enforcement and pool-backed for display.
// Both use the same owner reads and predicates, so the display cannot compute a
// more generous answer than the rule.
func Usage(
	ctx context.Context, reader UsageReader, l QuotaLimits, workspaceID pgtype.UUID, now time.Time,
) (QuotaState, error) {
	if reader.WorkspaceCreatedAt == nil || reader.CountRuns == nil {
		return QuotaState{}, errors.New("policy: quota usage readers not injected")
	}
	created, err := reader.WorkspaceCreatedAt(ctx, workspaceID)
	if err != nil {
		return QuotaState{}, err
	}
	windowStart := now.Add(-time.Duration(l.WindowDays) * 24 * time.Hour)

	window, err := reader.CountRuns(ctx, workspaceID, windowStart)
	if err != nil {
		return QuotaState{}, err
	}
	day, err := reader.CountRuns(ctx, workspaceID, now.Add(-24*time.Hour))
	if err != nil {
		return QuotaState{}, err
	}

	// PDM-010: a workspace inside its first window gets the lower allowance. The
	// rolling window handles the rest by itself — before the workspace is 30 days
	// old, "the last 30 days" and "since signup" are the same span, so only the
	// ceiling has to change.
	windowLimit := l.Window
	firstWindowEnds := created.Add(time.Duration(l.WindowDays) * 24 * time.Hour)
	inFirstWindow := now.Before(firstWindowEnds)
	if inFirstWindow {
		windowLimit = l.FirstWindow
	}

	// When more allowance appears. Rolling, so there is no calendar reset to name:
	// it is the moment the oldest counted run drops out of the back of the window.
	// In the first window it is instead the moment the ceiling itself rises, which
	// is the earlier and more useful answer for a new account.
	resets := now
	if window.Oldest != nil {
		resets = window.Oldest.Add(time.Duration(l.WindowDays) * 24 * time.Hour)
	}
	if inFirstWindow && firstWindowEnds.Before(resets) {
		resets = firstWindowEnds
	}

	return QuotaState{
		RemainingToday:  remaining(l.Daily, day.Used),
		RemainingWindow: remaining(windowLimit, window.Used),
		WindowResetsAt:  resets,
		Limits: QuotaReport{
			Daily:      l.Daily,
			Window:     windowLimit,
			WindowDays: l.WindowDays,
		},
	}, nil
}

func remaining(limit int, used int64) int {
	left := limit - int(used)
	if left < 0 {
		return 0
	}
	return left
}

// EnforceQuota is gate B's allowance condition (PDM-010, ADR-028 決策 2). It
// returns the refusal's reason label and the error, or ("", nil) when this
// workspace may start another run.
//
// The two-value shape is what keeps the enforcement point where ADR-028 put it.
// This function decides; the caller refuses, and the caller is `run`, which counts
// the refusal (metrics.RunRefused) and labels it for the audit trail with the
// reason returned here — one reason string, written once, so the Prometheus label
// and the audit reason cannot drift apart.
//
// The reader must be backed by the create-run transaction, and run's
// requireRunSlot must already have run on it — that call takes the per-workspace
// advisory lock, and this one runs inside it. Two things follow, and only one of
// them is a race fix:
//
//   - A refusal leaves nothing behind. The count is taken in the transaction that
//     would insert the run, so a rejected request rolls back the whole thing: no
//     run row, no queue job, no snapshot. A check in the handler could refuse
//     after another path had already written something.
//   - The two ceilings cannot drift apart. Concurrency and allowance are one
//     function apart in one transaction; changing where either is applied puts
//     the other in the diff.
//
// What the lock does *not* buy, stated rather than implied: the run being created
// is `queued`, and PDM-010 counts from `preparing`, so two simultaneous requests
// read the same usage whether they are serialised or not. The overshoot that
// allows is bounded by run.MaxConcurrentRunsPerWorkspace, because a workspace
// cannot be holding more uncounted runs than it may have in flight —
// requireRunSlot, under the same lock, is what makes that bound hold.
//
// Closing the gap entirely would mean counting runs PDM-010 says explicitly must
// not be counted: a failure during `provisioning` never took a resource, and
// charging for it is the thing the counting rule exists to prevent.
func EnforceQuota(
	ctx context.Context, reader UsageReader, l QuotaLimits, workspaceID pgtype.UUID,
) (string, error) {
	if !l.Enforced() {
		return "", nil
	}
	state, err := Usage(ctx, reader, l, workspaceID, time.Now())
	if err != nil {
		// Fail closed, like every other gate B condition (02:SEC-002: a check that
		// could not be performed has not passed). An allowance that could not be
		// counted is not an allowance of zero used.
		return "quota_unavailable", fmt.Errorf("%w: the allowance could not be counted, "+
			"and an uncounted allowance is not treated as an unused one: %w", ErrQuotaExceeded, err)
	}
	if state.RemainingToday <= 0 {
		return "quota_daily",
			fmt.Errorf("%w: %d runs a day is the limit; it resets 24 hours after your earliest run today",
				ErrQuotaExceeded, l.Daily)
	}
	if state.RemainingWindow <= 0 {
		return "quota_window",
			fmt.Errorf("%w: %d runs per %d days is the limit; the next one frees up at %s",
				ErrQuotaExceeded, state.Limits.Window, l.WindowDays,
				state.WindowResetsAt.UTC().Format(time.RFC3339))
	}
	return "", nil
}
