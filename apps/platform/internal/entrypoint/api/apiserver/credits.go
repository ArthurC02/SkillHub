// CRED-001 / CRED-007 (負責人 2026-09-08 定案 — see this task's brief; not yet
// recorded in `01` §10 / `04` / root AGENTS.md's prefix table, which are
// outside this file's write scope and are the main agent's to update):
//
//   - CRED-001: GET /me/credits — the account's Credit balance, the estimated
//     cost of one interactive-creation session in credits, and whether a new
//     session may start (gate ①, "開始前：餘額 < 門檻就不讓開新會話").
//   - CRED-007: POST /admin/credits/{workspace_id}/grants — the operator top-up
//     surface (MVP has no payment gateway; a grant is the whole of "充值", and
//     it is also how a gate-test participant's Credit reward is issued — a
//     rewrite of `05` R-2's cash reward into 「點數與加成」).
//
// SCOPE. This subagent may only touch apiserver/ and web/src/; contracts/,
// db/migrations/ and new domain packages belong to the main agent. The real
// ledger — cost_events, credit_entries, cost_statistics and the settleCost
// integration that writes them — is therefore not in this file. CreditLedger
// below is the port a real domain service will satisfy once that lands.
//
// Until Deps.Credits is wired to a live CreditLedger in NewApp, neither route
// is mounted at all (see router.go) — the same "nil = does not exist" shape
// GET /me/quota already uses for an allowance nothing enforces (entitlements
// package, ADR-028 決策 3), so this deployment never shows a number nothing
// backs (04 乙-2's lesson, restated in this task's brief for Credit).
package apiserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

// DebtFloorCredits is the most negative a workspace balance may go (負責人本
// 輪定案：「負債不能超過 -50」). The per-step settlement gate (②) enforces
// it — that lives with settleCost, outside this file's scope — this file only
// displays it so a blocked screen can say how much room is left.
const DebtFloorCredits int64 = -50

// MinCostSampleSize is how many recent cost samples a rolling window needs
// before its p95 is trusted as the real starting-session threshold, rather
// than the fallback constant below (負責人本輪：「樣本數不足時退回設定的保
// 守常數並在畫面標明是估計值」).
const MinCostSampleSize = 20

// FallbackSessionThresholdCredits is that conservative constant. 65 credits is
// the top of the measured range for one interactive-creation session
// including review (報告 §5–§14: session US$0.023–0.030 median / US$0.041–
// 0.050 max, review ≈US$0.019 per session), marked up at the default 1.3x and
// rounded up to a round number — US$0.050 × 1.3 = US$0.065 = 65 credits at
// the 1 credit = US$0.001 face value (負責人本輪定案兩者皆是).
const FallbackSessionThresholdCredits int64 = 65

// CreditSessionEstimate is what one interactive-creation session is expected
// to cost, in credits, and the threshold GET /me/credits blocks a new session
// against (gate ①). A real CreditLedger derives Threshold from
// cost_statistics's rolling p95 × markup, rounded up (負責人本輪：「進位一
// 律無條件進位」) — this file never recomputes it, for the reason
// entitlements.QuotaView never recomputes PDM-010's counters: a display with
// its own arithmetic can disagree with the rule it is showing.
type CreditSessionEstimate struct {
	LowCredits       int64
	HighCredits      int64
	ThresholdCredits int64
	SampleSize       int
	// Estimated is true when SampleSize < MinCostSampleSize and
	// FallbackSessionThresholdCredits was used in place of a measured p95.
	Estimated bool
}

// CreditLedger is the port a real domain service satisfies once the CRED
// contract and its migrations land. Balance must read the materialized
// balance column, never re-sum credit_entries live (負責人本輪：「另有一個
// 物化欄位隨分錄在同一交易更新，讀取用它、對帳用分錄」) — that reconciliation
// is a maintenance-time job, not a request-time one.
type CreditLedger interface {
	// Balance is the workspace's current Credit balance; it may be negative
	// down to DebtFloorCredits.
	Balance(ctx context.Context, workspaceID pgtype.UUID) (int64, error)
	// SessionEstimate reads the current cost_statistics rolling window (or
	// reports the fallback per MinCostSampleSize above).
	SessionEstimate(ctx context.Context) (CreditSessionEstimate, error)
	// Grant records one operator-issued credit_entries row (kind "grant") and
	// returns the resulting balance. amountCredits may be negative for a
	// corrective adjustment; MVP's only caller sends a positive top-up.
	// CRED-007 requires the implementation to also write an audit event
	// (actor, timestamp, target workspace, amount, reason) — this handler
	// validates that reason is non-empty but writes no audit event itself,
	// the same division every other operator surface on this table uses
	// (see discovery/restriction.go).
	Grant(ctx context.Context, workspaceID pgtype.UUID, amountCredits int64, reason string, actorUserID pgtype.UUID) (newBalance int64, err error)
}

// creditsHandler serves both CRED routes. Identity mirrors every other
// handler in this table (Search, Registry, Runs, ...) rather than a new
// cross-context read, so it is not a new dependency this table did not
// already have.
type creditsHandler struct {
	Ledger   CreditLedger
	Identity *identity.Service
}

// creditBalanceView is GET /me/credits's wire shape — a proposal for
// contracts/openapi/public.yaml's CRED-001 schema (not written yet; the
// contract is the main agent's to add). Modelled on RunQuota: display only,
// no arithmetic the enforcement point does not already own.
type creditBalanceView struct {
	BalanceCredits   int64              `json:"balance_credits"`
	DebtFloorCredits int64              `json:"debt_floor_credits"`
	EstimatedSession creditEstimateView `json:"estimated_session"`
	CanStart         bool               `json:"can_start"`
	// BlockReason is empty when CanStart is true, and otherwise names the
	// deficit — never a bare refusal. system.md:100「擋住人的訊息必須說下一
	// 步是什麼，或誠實說目前沒有下一步」.
	BlockReason string `json:"block_reason,omitempty"`
}

type creditEstimateView struct {
	LowCredits  int64 `json:"low_credits"`
	HighCredits int64 `json:"high_credits"`
	SampleSize  int   `json:"sample_size"`
	Estimated   bool  `json:"estimated"`
}

// creditBalanceResponse computes gate ①'s answer from a balance and a session
// estimate. Pure, so the gating rule has a test that needs no database (see
// credits_gate_test.go).
func creditBalanceResponse(balance int64, est CreditSessionEstimate) creditBalanceView {
	view := creditBalanceView{
		BalanceCredits:   balance,
		DebtFloorCredits: DebtFloorCredits,
		EstimatedSession: creditEstimateView{
			LowCredits: est.LowCredits, HighCredits: est.HighCredits,
			SampleSize: est.SampleSize, Estimated: est.Estimated,
		},
		CanStart: balance >= est.ThresholdCredits,
	}
	if !view.CanStart {
		short := est.ThresholdCredits - balance
		view.BlockReason = fmt.Sprintf(
			"餘額不足以開始新的創作會話：目前 %d 點，這一場大約要 %d 點，還差 %d 點。"+
				"請聯絡 operator 授予點數，或等待下次充值。",
			balance, est.ThresholdCredits, short,
		)
	}
	return view
}

// Get handles GET /me/credits (CRED-001). Mounted only when Ledger is
// configured (see router.go).
func (h *creditsHandler) Get(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	ws, err := h.Identity.PersonalWorkspace(r.Context(), user)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "workspace lookup failed")
		return
	}
	balance, err := h.Ledger.Balance(r.Context(), ws.ID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "balance lookup failed")
		return
	}
	est, err := h.Ledger.SessionEstimate(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "session estimate unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, creditBalanceResponse(balance, est))
}

// maxCreditGrantRequestBytes caps the request body the same way
// discovery/restriction.go's maxOperatorNoteBytes does for its own operator
// surface.
const maxCreditGrantRequestBytes = 4096

// creditGrantRequest is POST /admin/credits/{workspace_id}/grants's body.
type creditGrantRequest struct {
	AmountCredits int64  `json:"amount_credits"`
	Reason        string `json:"reason"`
}

// Grant handles POST /admin/credits/{workspace_id}/grants (CRED-007): the
// whole of MVP's top-up path, no payment gateway, and the mechanism a
// gate-test participant's Credit reward is issued through.
func (h *creditsHandler) Grant(w http.ResponseWriter, r *http.Request) {
	var workspaceID pgtype.UUID
	if err := workspaceID.Scan(r.PathValue("workspace_id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "workspace not found")
		return
	}
	// The same 404-not-401 shape discovery/restriction.go's sessionActor uses:
	// the only role check is auth.RequireOperator in router.go, and this
	// exists only so a session RequireOperator let through is never recorded
	// with no actor (02:SEC-011「誰做的」).
	actor, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	var body creditGrantRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxCreditGrantRequestBytes)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	body.Reason = strings.TrimSpace(body.Reason)
	if body.AmountCredits == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "amount_credits must not be zero")
		return
	}
	if body.Reason == "" {
		httpx.WriteError(w, http.StatusBadRequest, "reason is required")
		return
	}
	balance, err := h.Ledger.Grant(r.Context(), workspaceID, body.AmountCredits, body.Reason, actor.ID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "grant failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"workspace_id":    pgconv.UUIDString(workspaceID),
		"balance_credits": balance,
		"amount_credits":  body.AmountCredits,
	})
}
