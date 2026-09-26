package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
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

	Statement(ctx context.Context, workspaceID pgtype.UUID, beforeAt time.Time, beforeID pgtype.UUID) ([]credit.StatementEntry, bool, error)

	CostStatistics(ctx context.Context) ([]credit.KindStatistics, error)

	DailyCost(ctx context.Context, since time.Time) ([]credit.DailyAmount, error)

	DailyCredits(ctx context.Context, since time.Time) ([]credit.DailyAmount, int64, error)
}

type creditsHandler struct {
	Ledger   CreditLedger
	Identity *identity.Service

	RunsInWorkspace func(ctx context.Context, workspaceID pgtype.UUID, runIDs []pgtype.UUID) ([]pgtype.UUID, error)
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

type statementEntryView struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Label        string `json:"label"`
	DeltaCredits int64  `json:"delta_credits"`
	Estimated    bool   `json:"estimated"`
	CreatedAt    string `json:"created_at"`
	RunID        string `json:"run_id,omitempty"`
}

type statementResponse struct {
	Entries    []statementEntryView `json:"entries"`
	NextBefore string               `json:"next_before,omitempty"`
	Note       string               `json:"note"`
}

const statementNote = "這裡列的是真正扣掉與入帳的點數，一律以點數計。" +
	"試跑頁上的「用量」是事件逐筆疊出來的下界估計，兩者不會剛好相等；花了多少，以這裡為準。" +
	"標示「估計」的那一筆，是閘道沒有回報實際花費時，平台依統計上界扣的點數。"

var spentOnLabels = map[credit.CostKind]string{
	credit.KindRun:             "試跑",
	credit.KindCreationStep:    "互動創作",
	credit.KindGenerate:        "從描述生成 Skill",
	credit.KindReview:          "評估判定",
	credit.KindSuggestion:      "改善建議",
	credit.KindSuggestCriteria: "建議驗收條件",
	credit.KindIndexEnrich:     "匯入時的索引增強",
	credit.KindMatchReasons:    "搜尋結果的符合原因",
	credit.KindSearchIntent:    "搜尋意圖分析",
	credit.KindSearchEmbedding: "搜尋",
}

var entryKindLabels = map[credit.EntryKind]string{
	credit.EntryGrant:      "營運者授予",
	credit.EntryTopup:      "儲值",
	credit.EntryAdjustment: "調整",
}

func statementLabel(e credit.StatementEntry) string {
	if e.SpentOn != nil {
		if label, ok := spentOnLabels[*e.SpentOn]; ok {
			return label
		}
		return string(*e.SpentOn)
	}
	if label, ok := entryKindLabels[e.Kind]; ok {
		return label
	}
	return string(e.Kind)
}

func parseStatementCursor(raw string) (time.Time, pgtype.UUID, error) {
	var id pgtype.UUID
	if raw == "" {
		return time.Time{}, id, nil
	}
	at, rawID, _ := strings.Cut(raw, "_")
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil || id.Scan(rawID) != nil {
		return time.Time{}, id, errors.New("before must be a cursor this endpoint returned")
	}
	return t, id, nil
}

func statementCursor(e credit.StatementEntry) string {
	return e.CreatedAt.UTC().Format(time.RFC3339Nano) + "_" + pgconv.UUIDString(e.ID)
}

func (h *creditsHandler) Statement(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	beforeAt, beforeID, err := parseStatementCursor(r.URL.Query().Get("before"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	ws, err := h.Identity.PersonalWorkspace(r.Context(), user)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "workspace lookup failed")
		return
	}
	entries, more, err := h.Ledger.Statement(r.Context(), ws.ID, beforeAt, beforeID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "statement lookup failed")
		return
	}
	readable, err := h.readableRuns(r.Context(), ws.ID, entries)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "statement lookup failed")
		return
	}
	body := statementResponse{Entries: make([]statementEntryView, 0, len(entries)), Note: statementNote}
	for _, e := range entries {
		view := statementEntryView{
			ID: pgconv.UUIDString(e.ID), Kind: string(e.Kind), Label: statementLabel(e),
			DeltaCredits: e.DeltaCredits, Estimated: e.Estimated,
			CreatedAt: e.CreatedAt.UTC().Format(time.RFC3339),
		}
		if readable[e.RefID] {
			view.RunID = pgconv.UUIDString(e.RefID)
		}
		body.Entries = append(body.Entries, view)
	}
	if more && len(entries) > 0 {
		body.NextBefore = statementCursor(entries[len(entries)-1])
	}
	httpx.WriteJSON(w, http.StatusOK, body)
}

func (h *creditsHandler) readableRuns(
	ctx context.Context, workspaceID pgtype.UUID, entries []credit.StatementEntry,
) (map[pgtype.UUID]bool, error) {
	var runIDs []pgtype.UUID
	for _, e := range entries {
		if e.RefType != nil && *e.RefType == "run" && e.RefID.Valid {
			runIDs = append(runIDs, e.RefID)
		}
	}
	readable := map[pgtype.UUID]bool{}
	if len(runIDs) == 0 {
		return readable, nil
	}
	if h.RunsInWorkspace == nil {
		return nil, errors.New("credits: run reader is not configured")
	}
	found, err := h.RunsInWorkspace(ctx, workspaceID, runIDs)
	if err != nil {
		return nil, err
	}
	for _, id := range found {
		readable[id] = true
	}
	return readable, nil
}

const maxCreditGrantRequestBytes = 4096

type creditGrantRequest struct {
	AmountCredits int64  `json:"amount_credits"`
	Reason        string `json:"reason"`
}

type creditGrantResponse struct {
	WorkspaceID    string `json:"workspace_id"`
	BalanceCredits int64  `json:"balance_credits"`
	AmountCredits  int64  `json:"amount_credits"`
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
	httpx.WriteJSON(w, http.StatusOK, creditGrantResponse{
		WorkspaceID: pgconv.UUIDString(workspaceID), BalanceCredits: balance,
		AmountCredits: body.AmountCredits,
	})
}

type creditEntryView struct {
	Kind         string  `json:"kind"`
	DeltaCredits int64   `json:"delta_credits"`
	RefType      *string `json:"ref_type"`
	Estimated    bool    `json:"estimated"`
	CreatedAt    string  `json:"created_at"`
}

type creditAccountResponse struct {
	WorkspaceID    string            `json:"workspace_id"`
	BalanceCredits int64             `json:"balance_credits"`
	Entries        []creditEntryView `json:"entries"`
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
			Kind: string(e.Kind), DeltaCredits: e.DeltaCredits, RefType: e.RefType,
			Estimated: e.Estimated, CreatedAt: e.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, creditAccountResponse{
		WorkspaceID: pgconv.UUIDString(workspaceID), BalanceCredits: ledger.Balance,
		Entries: entries,
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

type costStatisticsResponse struct {
	Statistics []costStatisticsView `json:"statistics"`
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
			Kind: string(s.Kind), WindowStart: s.WindowStart.UTC().Format(time.RFC3339),
			WindowEnd: s.WindowEnd.UTC().Format(time.RFC3339), SampleCount: s.SampleCount,
			P50UsdMicros: s.P50UsdMicros, P90UsdMicros: s.P90UsdMicros,
			P95UsdMicros: s.P95UsdMicros, MaxUsdMicros: s.MaxUsdMicros,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, costStatisticsResponse{Statistics: views})
}
