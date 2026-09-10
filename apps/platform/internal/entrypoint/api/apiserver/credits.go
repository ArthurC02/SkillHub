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

const DebtFloorCredits int64 = -50

const MinCostSampleSize = 20

const FallbackSessionThresholdCredits int64 = 65

type CreditSessionEstimate struct {
	LowCredits       int64
	HighCredits      int64
	ThresholdCredits int64
	SampleSize       int

	Estimated bool
}

type CreditLedger interface {
	Balance(ctx context.Context, workspaceID pgtype.UUID) (int64, error)

	SessionEstimate(ctx context.Context) (CreditSessionEstimate, error)

	Grant(ctx context.Context, workspaceID pgtype.UUID, amountCredits int64, reason string, actorUserID pgtype.UUID) (newBalance int64, err error)
}

type creditsHandler struct {
	Ledger   CreditLedger
	Identity *identity.Service
}

type creditBalanceView struct {
	BalanceCredits   int64              `json:"balance_credits"`
	DebtFloorCredits int64              `json:"debt_floor_credits"`
	EstimatedSession creditEstimateView `json:"estimated_session"`
	CanStart         bool               `json:"can_start"`

	BlockReason string `json:"block_reason,omitempty"`
}

type creditEstimateView struct {
	LowCredits  int64 `json:"low_credits"`
	HighCredits int64 `json:"high_credits"`
	SampleSize  int   `json:"sample_size"`
	Estimated   bool  `json:"estimated"`
}

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

const maxCreditGrantRequestBytes = 4096

type creditGrantRequest struct {
	AmountCredits int64  `json:"amount_credits"`
	Reason        string `json:"reason"`
}

func (h *creditsHandler) Grant(w http.ResponseWriter, r *http.Request) {
	var workspaceID pgtype.UUID
	if err := workspaceID.Scan(r.PathValue("workspace_id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "workspace not found")
		return
	}

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
