package apiserver

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/jackc/pgx/v5/pgtype"
	"io"
	"net/http"
	"time"
)

type creationHandler struct {
	Svc       *creation.Service
	Identity  *identity.Service
	Transient func(context.Context, creation.JobArgs, *llmclient.GenerateDiagram) error
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
func creationError(w http.ResponseWriter, err error) {
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
		creationError(w, err)
		return
	}
	httpx.WriteJSON(w, 200, v)
}
func (h *creationHandler) Get(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.scope(w, r)
	if !ok {
		return
	}
	id, err := creation.ParseID(r.PathValue("session_id"))
	if err != nil {
		creationError(w, creation.ErrNotFound)
		return
	}
	v, err := h.Svc.Get(r.Context(), ws, id)
	if err != nil {
		creationError(w, err)
		return
	}
	httpx.WriteJSON(w, 200, v)
}
func (h *creationHandler) Create(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.scope(w, r)
	if !ok {
		return
	}
	var in struct {
		ID        pgtype.UUID `json:"id"`
		Message   string      `json:"message"`
		BudgetUSD float64     `json:"budget_usd"`
	}
	if !creationDecode(w, r, &in) {
		return
	}
	v, err := h.Svc.Create(r.Context(), ws, in.ID, in.Message, in.BudgetUSD)
	if err != nil {
		creationError(w, err)
		return
	}
	httpx.WriteJSON(w, 200, v)
}
func (h *creationHandler) Act(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.scope(w, r)
	if !ok {
		return
	}
	id, err := creation.ParseID(r.PathValue("session_id"))
	if err != nil {
		creationError(w, creation.ErrNotFound)
		return
	}
	var in creation.Command
	if !creationDecode(w, r, &in) {
		return
	}
	v, job, err := h.Svc.Act(r.Context(), ws, id, in)
	if err != nil {
		creationError(w, err)
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
			creationError(w, err)
			return
		}
	}
	httpx.WriteJSON(w, 200, v)
}
