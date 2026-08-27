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
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ErrQuotaExceeded is the allowance refusal. Like the other gate B conditions it
// becomes a 422: the request is well formed and the platform is working, the
// account simply has nothing left in this window.
var ErrQuotaExceeded = errors.New("this workspace has used its free run allowance")

// ErrAllowanceUnavailable is the other way enforce can refuse: the allowance
// could not be counted at all. It fails closed exactly as an exhausted one does
// (02:SEC-002 — a check that could not be performed has not passed), but it is
// NOT the same answer and must not wrap the same sentinel.
//
// It used to. Both handlers map their exceeded sentinel to a 422 carrying
// err.Error(), so a database outage reached the user as "this workspace has used
// its free allowance ... " with the wrapped pgx error appended — a sentence that
// is false, unactionable, and carries a connection string into a response body
// (NFR-002, iron rule 11). Not wrapping it is what makes the two distinguishable
// at the handler: the generate handler maps it to a 503 with a static sentence,
// the run handler lets it fall through to its generic 500 — either way a 5xx,
// which is what an outage is, and never err.Error().
var ErrAllowanceUnavailable = errors.New("the allowance could not be counted")

// PDM-010 §8.1's numbers.
//
// Ratified 2026-08-27, all four, exactly as proposed — the signature is recorded
// in docs/plans/mvp/m0/pdm-proposals.md §9.1 and in 05-pending-rulings.md R-1b.
// Nothing here changed when it was signed, and that is the point: what was
// missing was a signature, not a behaviour.
//
// The one thing that was genuinely undecided is not: PDM-010 named a choice the
// implementation must not infer — whether the first window is min(20,30) = 20 or
// 20+30 = 50 ("請負責人明確擇一,不要留給實作推斷") — and 04 乙-15 ruled it on
// 2026-08-22: **20, not cumulative**. Three reasons, the load-bearing one being
// the second: the proposal's own wording reads as the lower number; cumulating
// introduces a second clock (does it roll over? expire at month end?) and a quota
// is a §2.2 object, where display and enforcement must point at one line; and the
// beta's 14 days sit inside one window anyway, where the gate is completion rate
// and not run count. Recorded there with its dissent (most products make the first
// month larger, not smaller — the ruling rests on beta testers not being organic
// signups).
//
// ADR-028 決策 4 permitted building the enforcement point before the numbers were
// ratified, because it does not depend on them; what it forbade until then was
// putting an unratified number on a screen. The 2026-08-27 ratification lifts
// that ban for these four, and lifts nothing else — RUN_QUOTA=off (ADR-055) is a
// separate switch and is still off, so this deployment still displays no
// allowance. A ban and a switch, and only the ban moved.
//
// A deployment that does not want one passes the zero QuotaLimits, and then
// nothing is enforced and nothing is displayed (see QuotaLimits.Enforced).
// "Unconfigured" is deliberately not the way to say that: an unset RUN_QUOTA
// means enforced with the numbers above, not unenforced. That asymmetry is
// stated where it is decided (quotaFromEnv / generateQuotaFromEnv in
// cmd/api/main.go) and it survives ratification unchanged — an unset retention
// means data is not collected, which is safe; an unset allowance would mean the
// only real cost ceiling is off, which is not. 05 R-1a recorded it backwards
// once and ADR-055 had to correct it.
const (
	quotaFirstWindowRuns = 20 // first 30 days, min(20,30) per PDM-010 — ratified 2026-08-27
	quotaWindowRuns      = 30 // per rolling window thereafter — ratified 2026-08-27
	quotaDailyRuns       = 5  // all the time, first window included — ratified 2026-08-27
	quotaWindowDays      = 30 // the rolling window's length — ratified 2026-08-27
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

// DefaultQuotaLimits is PDM-010 §8.1 as proposed and, since 2026-08-27, as
// ratified. See the constants above for where that signature is recorded.
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
	return enforce(ctx, reader, l, workspaceID, allowance{
		sentinel: ErrQuotaExceeded, noun: "runs", prefix: "quota",
	})
}

// allowance is what differs between the two things this file can refuse. The
// arithmetic does not differ, and duplicating it would be the way the two
// silently stop agreeing on what "the window resets at" means.
type allowance struct {
	sentinel error
	noun     string // "runs" or "generations"
	prefix   string // metric/audit reason prefix
}

func enforce(
	ctx context.Context, reader UsageReader, l QuotaLimits, workspaceID pgtype.UUID, a allowance,
) (string, error) {
	if !l.Enforced() {
		return "", nil
	}
	state, err := Usage(ctx, reader, l, workspaceID, time.Now())
	if err != nil {
		// Fail closed, like every other gate B condition (02:SEC-002: a check that
		// could not be performed has not passed). An allowance that could not be
		// counted is not an allowance of zero used.
		return a.prefix + "_unavailable", fmt.Errorf(
			"%w: an uncounted allowance is not treated as an unused one: %w",
			ErrAllowanceUnavailable, err)
	}
	if state.RemainingToday <= 0 {
		return a.prefix + "_daily",
			fmt.Errorf("%w: %d %s a day is the limit; it resets 24 hours after your earliest %s today",
				a.sentinel, l.Daily, a.noun, strings.TrimSuffix(a.noun, "s"))
	}
	if state.RemainingWindow <= 0 {
		return a.prefix + "_window",
			fmt.Errorf("%w: %d %s per %d days is the limit; the next one frees up at %s",
				a.sentinel, state.Limits.Window, a.noun, l.WindowDays,
				state.WindowResetsAt.UTC().Format(time.RFC3339))
	}
	return "", nil
}
