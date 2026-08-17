package run

// The PDM-010 free run allowance, and the one place it is enforced (ADR-028 決策 2).
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
//	workspace concurrency   how many at once (gateb.go). Two at a time still
//	                        allows three hundred in a month
//
// So the counter has to be the platform's own, and it belongs exactly where gate
// B's concurrency check already is: inside the create-run transaction, under the
// same advisory lock. Not for convenience — that is the only point in the system
// where a run is about to exist and nothing has been spent on it yet.
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

	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/metrics"
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
type QuotaReport struct {
	Daily      int `json:"daily"`
	Window     int `json:"window"`
	WindowDays int `json:"window_days"`
	Concurrent int `json:"concurrent"`
}

// quotaUsage counts what this workspace has already spent, over both spans.
//
// q is whatever handle the caller has: the create-run transaction's for the
// enforcement path, a pool-backed one for the read path. Both run the same query
// with the same predicate, which is the point — a display that computed its own
// number could disagree with the rule, and disagreeing in the generous direction
// is the failure mode of 04 乙-2.
func (s *Service) quotaUsage(
	ctx context.Context, q *gen.Queries, workspaceID pgtype.UUID, now time.Time,
) (state QuotaState, err error) {
	l := s.Quota

	created, err := q.GetWorkspaceCreatedAt(ctx, workspaceID)
	if err != nil {
		return QuotaState{}, err
	}
	windowStart := now.Add(-time.Duration(l.WindowDays) * 24 * time.Hour)

	window, err := q.CountQuotaRuns(ctx, gen.CountQuotaRunsParams{
		WorkspaceID: workspaceID, Since: pgtype.Timestamptz{Time: windowStart, Valid: true},
	})
	if err != nil {
		return QuotaState{}, err
	}
	day, err := q.CountQuotaRuns(ctx, gen.CountQuotaRunsParams{
		WorkspaceID: workspaceID,
		Since:       pgtype.Timestamptz{Time: now.Add(-24 * time.Hour), Valid: true},
	})
	if err != nil {
		return QuotaState{}, err
	}

	// PDM-010: a workspace inside its first window gets the lower allowance. The
	// rolling window handles the rest by itself — before the workspace is 30 days
	// old, "the last 30 days" and "since signup" are the same span, so only the
	// ceiling has to change.
	windowLimit := l.Window
	firstWindowEnds := created.Time.Add(time.Duration(l.WindowDays) * 24 * time.Hour)
	inFirstWindow := now.Before(firstWindowEnds)
	if inFirstWindow {
		windowLimit = l.FirstWindow
	}

	// When more allowance appears. Rolling, so there is no calendar reset to name:
	// it is the moment the oldest counted run drops out of the back of the window.
	// In the first window it is instead the moment the ceiling itself rises, which
	// is the earlier and more useful answer for a new account.
	resets := now
	if window.Oldest.Valid {
		resets = window.Oldest.Time.Add(time.Duration(l.WindowDays) * 24 * time.Hour)
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
			Concurrent: MaxConcurrentRunsPerWorkspace,
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

// requireQuota is gate B's allowance condition (PDM-010, ADR-028 決策 2).
//
// q must be the create-run transaction's handle, and requireRunSlot must already
// have run on it — that call takes the per-workspace advisory lock, and this one
// runs inside it. Two things follow, and only one of them is a race fix:
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
// allows is bounded by MaxConcurrentRunsPerWorkspace, because a workspace cannot
// be holding more uncounted runs than it may have in flight — requireRunSlot,
// under the same lock, is what makes that bound hold.
//
// Closing the gap entirely would mean counting runs PDM-010 says explicitly must
// not be counted: a failure during `provisioning` never took a resource, and
// charging for it is the thing the counting rule exists to prevent.
func (s *Service) requireQuota(ctx context.Context, q *gen.Queries, workspaceID pgtype.UUID) error {
	if !s.Quota.Enforced() {
		return nil
	}
	state, err := s.quotaUsage(ctx, q, workspaceID, time.Now())
	if err != nil {
		// Fail closed, like every other gate B condition (02:SEC-002: a check that
		// could not be performed has not passed). An allowance that could not be
		// counted is not an allowance of zero used.
		metrics.RunRefused.WithLabelValues("quota_unavailable").Inc()
		return fmt.Errorf("%w: the allowance could not be counted, "+
			"and an uncounted allowance is not treated as an unused one: %w", ErrQuotaExceeded, err)
	}
	if state.RemainingToday <= 0 {
		metrics.RunRefused.WithLabelValues("quota_daily").Inc()
		return fmt.Errorf("%w: %d runs a day is the limit; it resets 24 hours after your earliest run today",
			ErrQuotaExceeded, s.Quota.Daily)
	}
	if state.RemainingWindow <= 0 {
		metrics.RunRefused.WithLabelValues("quota_window").Inc()
		return fmt.Errorf("%w: %d runs per %d days is the limit; the next one frees up at %s",
			ErrQuotaExceeded, state.Limits.Window, s.Quota.WindowDays,
			state.WindowResetsAt.UTC().Format(time.RFC3339))
	}
	return nil
}

// QuotaFor is the read path behind GET /me/quota and the pre-run summary's quota
// block. ok is false when this deployment enforces no allowance, and the caller
// must then show nothing rather than zeroes.
func (s *Service) QuotaFor(ctx context.Context, workspaceID pgtype.UUID) (QuotaState, bool, error) {
	if !s.Quota.Enforced() {
		return QuotaState{}, false, nil
	}
	state, err := s.quotaUsage(ctx, s.queries(), workspaceID, time.Now())
	if err != nil {
		return QuotaState{}, false, err
	}
	return state, true, nil
}
