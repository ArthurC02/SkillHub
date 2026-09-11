package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/jackc/pgx/v5/pgtype"
	"io"
	"net/http"
	"strconv"
	"time"
)

type creationHandler struct {
	Svc       *creation.Service
	Identity  *identity.Service
	Transient func(context.Context, creation.JobArgs, *llmclient.GenerateDiagram) error
	Credit    *credit.Service
}

var errNoCreditRate = errors.New("creation: credit conversion unavailable")

var snapshotCreditFields = map[string]string{
	"budget_usd":   "budget_credits",
	"reserved_usd": "reserved_credits",
	"spent_usd":    "spent_credits",
}

func (h *creationHandler) present(v creation.View) (map[string]any, error) {
	if h.Credit == nil {
		return nil, errNoCreditRate
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	snap, _ := out["snapshot"].(map[string]any)
	for usdKey, creditKey := range snapshotCreditFields {
		usd, present := snap[usdKey].(float64)
		delete(snap, usdKey)
		if !present {
			continue
		}
		credits := int64(0)
		if usd > 0 {
			var ok bool
			if credits, ok = h.Credit.CreditsForUSD(usd); !ok {
				return nil, errNoCreditRate
			}
		}
		snap[creditKey] = credits
	}
	return out, nil
}

func (h *creationHandler) writeView(w http.ResponseWriter, v creation.View) {
	out, err := h.present(v)
	if err != nil {
		httpx.WriteError(w, 503, "創作服務暫時無法完成這個動作，進度已保留。")
		return
	}
	httpx.WriteJSON(w, 200, out)
}

func (h *creationHandler) budgetBand() (minCredits, maxCredits int64, ok bool) {
	if h.Credit == nil {
		return 0, 0, false
	}
	minCredits, okMin := h.Credit.CreditsForUSD(h.Svc.Limits.MaxCallCostUSD)
	maxCredits, okMax := h.Credit.CreditsWithinUSD(h.Svc.Limits.MaxCostUSD)
	return minCredits, maxCredits, okMin && okMax
}

func (h *creationHandler) usdForCredits(credits int64) (float64, bool) {
	if h.Credit == nil {
		return 0, false
	}
	return h.Credit.USDForCredits(credits)
}

func (h *creationHandler) scope(w http.ResponseWriter, r *http.Request) (identity.Workspace, bool) {
	u, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, 401, "請先登入。")
		return identity.Workspace{}, false
	}
	ws, err := h.Identity.PersonalWorkspace(r.Context(), u)
	if err != nil {
		httpx.WriteError(w, 503, "工作區暫時無法讀取。")
		return ws, false
	}
	return ws, true
}
func (h *creationHandler) creationError(w http.ResponseWriter, err error) {
	code := 503
	text := "創作服務暫時無法完成這個動作，進度已保留。"
	switch {
	case errors.Is(err, creation.ErrNotFound):
		code = 404
		text = "找不到可讀取的創作或參考內容。"
	case errors.Is(err, creation.ErrConflict), errors.Is(err, creation.ErrReplayMismatch):
		code = 409
		text = "創作進度已改變，請重新讀取後確認。"
	case errors.Is(err, creation.ErrInvalidCommand):
		code = 422
		text = "請確認目前步驟需要的輸入與草稿。"
	case errors.Is(err, creation.ErrLimit):
		code = 422
		text = "已達這次核准的創作限制。"
	case errors.Is(err, ingest.ErrGeneratedNameCollision):
		code = 422
		text = "你的工作區已經有一個同名的 Skill。請先刪除或改名它，或在對話中請 Agent 把草稿改名後再保存。"
	case errors.Is(err, creation.ErrDeadline):
		code = 422
		minutes := int64(h.Svc.Limits.SessionTimeout / time.Minute)
		text = "這次創作已超過時間上限（自開始起 " + strconv.FormatInt(minutes, 10) + " 分鐘）；進度已保留，但不能再繼續，請開始新的創作。"
	case errors.Is(err, creation.ErrCreditThreshold):

		code = 422
		text = "點數不足，無法開始新的創作。請聯絡管理者為這個帳號加點；已經開始的創作不受影響。"
	case errors.Is(err, creation.ErrBudgetOutOfBand):
		code = 422
		if minCredits, maxCredits, ok := h.budgetBand(); ok {
			text = "這次預算必須介於 " + strconv.FormatInt(minCredits, 10) + " 點與 " + strconv.FormatInt(maxCredits, 10) + " 點之間。"
		} else {
			text = "這次預算超出允許的範圍。"
		}
	}
	httpx.WriteError(w, code, text)
}
func creationDecode(w http.ResponseWriter, r *http.Request, v any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		httpx.WriteError(w, 400, "輸入格式不正確或檔案太大。")
		return false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		httpx.WriteError(w, 400, "輸入格式不正確。")
		return false
	}
	return true
}
func (h *creationHandler) List(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.scope(w, r)
	if !ok {
		return
	}
	v, err := h.Svc.List(r.Context(), ws)
	if err != nil {
		h.creationError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(v))
	for _, one := range v {
		presented, err := h.present(one)
		if err != nil {
			httpx.WriteError(w, 503, "創作服務暫時無法完成這個動作，進度已保留。")
			return
		}
		out = append(out, presented)
	}
	httpx.WriteJSON(w, 200, out)
}
func (h *creationHandler) Get(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.scope(w, r)
	if !ok {
		return
	}
	id, err := creation.ParseID(r.PathValue("session_id"))
	if err != nil {
		h.creationError(w, creation.ErrNotFound)
		return
	}
	v, err := h.Svc.Get(r.Context(), ws, id)
	if err != nil {
		h.creationError(w, err)
		return
	}
	h.writeView(w, v)
}
func (h *creationHandler) Create(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.scope(w, r)
	if !ok {
		return
	}
	var in struct {
		ID            pgtype.UUID `json:"id"`
		Message       string      `json:"message"`
		BudgetCredits int64       `json:"budget_credits"`
	}
	if !creationDecode(w, r, &in) {
		return
	}
	budget, ok := h.usdForCredits(in.BudgetCredits)
	if !ok {
		h.creationError(w, creation.ErrBudgetOutOfBand)
		return
	}
	v, err := h.Svc.Create(r.Context(), ws, in.ID, in.Message, budget)
	if err != nil {
		h.creationError(w, err)
		return
	}
	h.writeView(w, v)
}
func (h *creationHandler) Act(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.scope(w, r)
	if !ok {
		return
	}
	id, err := creation.ParseID(r.PathValue("session_id"))
	if err != nil {
		h.creationError(w, creation.ErrNotFound)
		return
	}
	var body struct {
		creation.Command
		BudgetCredits *int64 `json:"budget_credits"`
	}
	if !creationDecode(w, r, &body) {
		return
	}
	in := body.Command
	in.BudgetUSD = 0
	if body.BudgetCredits != nil {
		budget, ok := h.usdForCredits(*body.BudgetCredits)
		if !ok {
			h.creationError(w, creation.ErrBudgetOutOfBand)
			return
		}
		in.BudgetUSD = budget
	}
	v, job, err := h.Svc.Act(r.Context(), ws, id, in)
	if err != nil {
		h.creationError(w, err)
		return
	}
	if job != nil {
		if h.Transient == nil {
			err = creation.ErrUnavailable
		} else {
			err = h.Transient(r.Context(), *job, in.Diagram)
		}
		if err != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = h.Svc.InterruptedTransient(ctx, *job)
			cancel()
		}
		v, err = h.Svc.Get(r.Context(), ws, id)
		if err != nil {
			h.creationError(w, err)
			return
		}
	}
	h.writeView(w, v)
}
func (h *creationHandler) Limits(w http.ResponseWriter, r *http.Request) {
	l := h.Svc.Limits
	if !l.Valid() {
		httpx.WriteError(w, 503, "創作服務暫時無法完成這個動作，進度已保留。")
		return
	}
	minCredits, maxCredits, ok := h.budgetBand()
	if !ok {
		httpx.WriteError(w, 503, "創作服務暫時無法完成這個動作，進度已保留。")
		return
	}
	httpx.WriteJSON(w, 200, struct {
		MinBudgetCredits      int64 `json:"min_budget_credits"`
		MaxBudgetCredits      int64 `json:"max_budget_credits"`
		MaxSteps              int   `json:"max_steps"`
		MaxToolCalls          int   `json:"max_tool_calls"`
		CallTimeoutSeconds    int64 `json:"call_timeout_seconds"`
		SessionTimeoutSeconds int64 `json:"session_timeout_seconds"`
		RetentionSeconds      int64 `json:"retention_seconds"`
	}{
		MinBudgetCredits:      minCredits,
		MaxBudgetCredits:      maxCredits,
		MaxSteps:              l.MaxSteps,
		MaxToolCalls:          l.MaxToolCalls,
		CallTimeoutSeconds:    int64(l.CallTimeout / time.Second),
		SessionTimeoutSeconds: int64(l.SessionTimeout / time.Second),
		RetentionSeconds:      int64(l.Retention / time.Second),
	})
}
