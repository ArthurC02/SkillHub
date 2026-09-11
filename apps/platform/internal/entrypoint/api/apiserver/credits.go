package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

type CreditSessionEstimate struct {
	LowCredits       int64
	HighCredits      int64
	ThresholdCredits int64
	SampleSize       int
	DebtFloorCredits int64

	Estimated bool
}

type CreditLedger interface {
	Standing(ctx context.Context, workspaceID pgtype.UUID) (balance int64, canStart bool, err error)

	SessionEstimate(ctx context.Context) (CreditSessionEstimate, error)

	Grant(ctx context.Context, workspaceID pgtype.UUID, amountCredits int64, reason string, actorUserID pgtype.UUID) (newBalance int64, err error)

	Ledger(ctx context.Context, workspaceID, operatorID pgtype.UUID) (credit.Ledger, error)

	CostStatistics(ctx context.Context) ([]credit.KindStatistics, error)
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

func creditBalanceResponse(balance int64, canStart bool, est CreditSessionEstimate) creditBalanceView {
	view := creditBalanceView{
		BalanceCredits:   balance,
		DebtFloorCredits: est.DebtFloorCredits,
		EstimatedSession: creditEstimateView{
			LowCredits: est.LowCredits, HighCredits: est.HighCredits,
			SampleSize: est.SampleSize, Estimated: est.Estimated,
		},
		CanStart: canStart,
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
	balance, canStart, err := h.Ledger.Standing(r.Context(), ws.ID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "balance lookup failed")
		return
	}
	est, err := h.Ledger.SessionEstimate(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "session estimate unavailable")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, creditBalanceResponse(balance, canStart, est))
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
	balance, err := h.Ledger.Grant(r.Context(), workspaceID, body.AmountCredits, body.Reason, actor.ID)
	switch {
	case errors.Is(err, identity.ErrWorkspaceNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	case errors.Is(err, credit.ErrZeroAmount):
		httpx.WriteError(w, http.StatusBadRequest, "amount_credits must not be zero")
		return
	case errors.Is(err, credit.ErrReasonRequired):
		httpx.WriteError(w, http.StatusBadRequest, "reason is required")
		return
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "grant failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"workspace_id":    pgconv.UUIDString(workspaceID),
		"balance_credits": balance,
		"amount_credits":  body.AmountCredits,
	})
}

type creditEntryView struct {
	Kind         string  `json:"kind"`
	DeltaCredits int64   `json:"delta_credits"`
	RefType      *string `json:"ref_type"`
	Estimated    bool    `json:"estimated"`
	CreatedAt    string  `json:"created_at"`
}

func (h *creditsHandler) Account(w http.ResponseWriter, r *http.Request) {
	var workspaceID pgtype.UUID
	if err := workspaceID.Scan(r.PathValue("workspace_id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	operator, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	ledger, err := h.Ledger.Ledger(r.Context(), workspaceID, operator.ID)
	switch {
	case errors.Is(err, identity.ErrWorkspaceNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "ledger lookup failed")
		return
	}
	entries := make([]creditEntryView, 0, len(ledger.Entries))
	for _, e := range ledger.Entries {
		entries = append(entries, creditEntryView{
			Kind: e.Kind, DeltaCredits: e.DeltaCredits, RefType: e.RefType,
			Estimated: e.Estimated, CreatedAt: e.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"workspace_id":    pgconv.UUIDString(workspaceID),
		"balance_credits": ledger.Balance,
		"entries":         entries,
	})
}

type costStatisticsView struct {
	Kind         string `json:"kind"`
	WindowStart  string `json:"window_start"`
	WindowEnd    string `json:"window_end"`
	SampleCount  int64  `json:"sample_count"`
	P50UsdMicros *int64 `json:"p50_usd_micros"`
	P90UsdMicros *int64 `json:"p90_usd_micros"`
	P95UsdMicros *int64 `json:"p95_usd_micros"`
	MaxUsdMicros *int64 `json:"max_usd_micros"`
}

func (h *creditsHandler) CostStatistics(w http.ResponseWriter, r *http.Request) {
	stats, err := h.Ledger.CostStatistics(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "statistics lookup failed")
		return
	}
	views := make([]costStatisticsView, 0, len(stats))
	for _, s := range stats {
		views = append(views, costStatisticsView{
			Kind: s.Kind, WindowStart: s.WindowStart.UTC().Format(time.RFC3339),
			WindowEnd: s.WindowEnd.UTC().Format(time.RFC3339), SampleCount: s.SampleCount,
			P50UsdMicros: s.P50UsdMicros, P90UsdMicros: s.P90UsdMicros,
			P95UsdMicros: s.P95UsdMicros, MaxUsdMicros: s.MaxUsdMicros,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"statistics": views})
}
