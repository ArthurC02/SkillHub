package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

const (
	sessionCookie = "sh_session"
	stateCookie   = "sh_oauth_state"
)

type Handler struct {
	Service *Service

	Secure bool

	AppURL string

	DevLogin bool

	Operators map[string]bool

	Invited map[string]bool

	Features map[string]bool

	Disclosures map[string]bool
}

func (h *Handler) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/github/login", h.startLogin)
	mux.HandleFunc("GET /auth/github/callback", h.finishLogin)
	mux.HandleFunc("POST /auth/logout", h.logout)
	mux.HandleFunc("GET /me", h.RequireSession(h.me))
	mux.HandleFunc("DELETE /me", h.RequireSession(h.requestDeletion))
	mux.HandleFunc("POST /me/deletion/cancel", h.RequireSession(h.cancelDeletion))
	if h.DevLogin {
		mux.HandleFunc("POST /auth/dev/login", h.devLogin)
	}
}

func (h *Handler) devLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		User string `json:"user"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	name := body.User
	if name == "" {
		name = "dev"
	}
	if len(name) > 64 {
		httpx.WriteError(w, http.StatusBadRequest, "使用者名稱最多 64 個字元")
		return
	}
	token, err := h.Service.LoginOrSignup(r.Context(), ExternalIdentity{
		Provider:       "dev",
		ProviderUserID: name,
		Email:          name + "@dev.local",
		Name:           name,
		Login:          name,
	})
	if err != nil {
		if errors.Is(err, ErrAccountPurging) {
			httpx.WriteError(w, http.StatusConflict, "account deletion is in progress")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "login failed")
		return
	}
	h.setSessionCookie(w, token)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) startLogin(w http.ResponseWriter, r *http.Request) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "state generation failed")
		return
	}
	state := hex.EncodeToString(raw)
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: state, Path: "/auth/github",
		MaxAge: 600, HttpOnly: true, Secure: h.Secure, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, h.Service.OAuth.AuthURL(state), http.StatusFound)
}

func (h *Handler) finishLogin(w http.ResponseWriter, r *http.Request) {

	sc, err := r.Cookie(stateCookie)
	if err != nil || sc.Value == "" || r.URL.Query().Get("state") != sc.Value {
		httpx.WriteError(w, http.StatusUnauthorized, "oauth state mismatch")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: "", Path: "/auth/github", MaxAge: -1,
		HttpOnly: true, Secure: h.Secure, SameSite: http.SameSiteLaxMode,
	})

	ctx := r.Context()
	accessToken, err := h.Service.OAuth.Exchange(ctx, r.URL.Query().Get("code"))
	if err != nil {
		slog.Warn("github code exchange failed", "error", err)
		httpx.WriteError(w, http.StatusUnauthorized, "code exchange failed")
		return
	}
	ghUser, err := h.Service.OAuth.FetchUser(ctx, accessToken)
	if err != nil {
		slog.Warn("github user fetch failed", "error", err)
		httpx.WriteError(w, http.StatusUnauthorized, "user fetch failed")
		return
	}
	token, err := h.Service.LoginOrSignup(ctx, ghUser.External())
	if err != nil {
		if errors.Is(err, ErrAccountPurging) {
			httpx.WriteError(w, http.StatusConflict, "account deletion is in progress")
			return
		}
		slog.Error("login failed", "error", err)
		httpx.WriteError(w, http.StatusInternalServerError, "login failed")
		return
	}
	h.setSessionCookie(w, token)
	appURL := h.AppURL
	if appURL == "" {
		appURL = "/"
	}
	http.Redirect(w, r, appURL, http.StatusFound)
}

func (h *Handler) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/",
		MaxAge:   int(SessionTTL / time.Second),
		HttpOnly: true, Secure: h.Secure, SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		if err := h.Service.Logout(r.Context(), c.Value); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "登出沒有完成，可以再試一次")
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: h.Secure, SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

type ctxKey struct{}

func (h *Handler) RequireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		user, err := h.Service.UserForToken(r.Context(), c.Value)
		if err != nil {
			httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, user)))
	}
}

func (h *Handler) LogOperatorRoster(ctx context.Context) error {
	ids := make([]string, 0, len(h.Operators))
	for id := range h.Operators {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return audit.Log(ctx, h.Service.Pool, audit.Event{

		Action:       audit.ActionOperatorRoster,
		ResourceType: audit.ResourceOperatorRoster,
		Metadata: map[string]any{
			"user_ids": ids,
			"count":    len(ids),
			"source":   "OPERATOR_USER_IDS",
		},
	})
}

func (h *Handler) RequireOperator(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			httpx.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		user, err := h.Service.UserForToken(r.Context(), c.Value)
		if err != nil {

			httpx.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		if !h.Operators[pgconv.UUIDString(user.ID)] {
			h.logOperatorRefusal(r, user)
			httpx.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, user)))
	}
}

func (h *Handler) logOperatorRefusal(r *http.Request, user User) {
	pattern := r.Pattern
	if pattern == "" {
		pattern = r.Method + " (unrouted)"
	}
	if err := audit.Log(r.Context(), h.Service.Pool, audit.Event{
		Actor:        user.ID,
		Action:       audit.ActionOperatorRefused,
		ResourceType: audit.ResourceOperatorRoute,
		Metadata:     map[string]any{"route": pattern},
	}); err != nil {
		slog.Error("operator refusal not audited", "error", err)
	}
}

const betaNotInvited = "Skill Hub is in closed beta: browsing and skill details are open to " +
	"everyone, but forking, trial runs and downloads are limited to the invited testers. " +
	"Tell us what you were trying to do at POST /feedback with kind=need_signal and it goes " +
	"straight into the scope review."

func (h *Handler) RequireInvited(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if len(h.Invited) == 0 {
			next(w, r)
			return
		}
		user, ok := SessionUser(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		invited, err := h.invited(r.Context(), user)
		if err != nil {

			httpx.WriteError(w, http.StatusServiceUnavailable, "invite check unavailable")
			return
		}
		if !invited {
			writeNotInvited(w, r)
			return
		}
		next(w, r)
	}
}

const notInvitedHTML = `<!doctype html><html lang="zh-Hant"><meta charset="utf-8"><title>需要封測邀請</title>` +
	`<p>Skill Hub 還在封測：瀏覽與 Skill 詳情對所有人開放，但 Fork、試跑與下載只開放給受邀的測試者。` +
	`回到上一頁，用頁尾的「回報問題」告訴我們你想做什麼。</p>`

func writeNotInvited(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.Header.Get("Accept"), "text/html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(notInvitedHTML))
		return
	}
	httpx.WriteError(w, http.StatusForbidden, betaNotInvited)
}

func (h *Handler) invited(ctx context.Context, user User) (bool, error) {
	if len(h.Invited) == 0 {
		return true, nil
	}
	ids, err := h.Service.queries().GetIdentityProviderIDs(ctx, user.ID)
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		if h.Invited[id.ProviderUserID] {
			return true, nil
		}
	}
	return false, nil
}

func (h *Handler) LogInviteRoster(ctx context.Context) error {
	if len(h.Invited) == 0 {
		return nil
	}
	ids := make([]string, 0, len(h.Invited))
	for id := range h.Invited {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return audit.Log(ctx, h.Service.Pool, audit.Event{
		Action:       audit.ActionBetaRoster,
		ResourceType: audit.ResourceBetaRoster,
		Metadata: map[string]any{
			"provider_user_ids": ids,
			"count":             len(ids),
			"source":            "BETA_ALLOWLIST",
		},
	})
}

func SessionUser(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
}

func (h *Handler) OptionalSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			next(w, r)
			return
		}
		user, err := h.Service.UserForToken(r.Context(), c.Value)
		if err != nil {
			next(w, r)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, user)))
	}
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	user, _ := SessionUser(r.Context())
	ws, err := h.Service.PersonalWorkspace(r.Context(), user)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "workspace lookup failed")
		return
	}

	out := map[string]any{
		"user_id":               pgconv.UUIDString(user.ID),
		"email":                 user.Email,
		"display_name":          user.DisplayName,
		"workspace_id":          pgconv.UUIDString(ws.ID),
		"deletion_requested_at": nil,
		"purge_after":           nil,

		"deletion_scope": nil,
	}

	merged := map[string]bool{}
	for name, on := range h.Disclosures {
		merged[name] = on
	}
	if len(h.Features) > 0 {
		if invited, err := h.invited(r.Context(), user); err == nil && invited {
			for name, on := range h.Features {
				merged[name] = on
			}
		}
	}
	if len(merged) > 0 {
		out["features"] = merged
	}
	if user.DeletionRequestedAt.Valid {
		at := user.DeletionRequestedAt.Time.UTC()
		out["deletion_requested_at"] = at.Format(time.RFC3339)
		out["purge_after"] = at.Add(AccountDeletionGrace).UTC().Format(time.RFC3339)
		out["deletion_scope"] = deletionScope
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

const deletionScope = "寬限期結束前，你的帳號照常可用。到期後，你上傳的資料集、Run 產出，" +
	"以及沒有任何人 Fork 或執行過的 Skill 會連同檔案永久刪除。被其他使用者 Fork 過、" +
	"或歷史 Run 使用過的 Skill 版本會保留（它們的內容是別人的來源鏈），" +
	"但你的身分會從上面移除，顯示為已刪除的使用者所有。"

func (h *Handler) requestDeletion(w http.ResponseWriter, r *http.Request) {
	user, _ := SessionUser(r.Context())
	updated, err := h.Service.RequestAccountDeletion(r.Context(), user)
	if err != nil {
		if errors.Is(err, ErrAccountPurging) {
			httpx.WriteError(w, http.StatusConflict, "刪除已經不可逆，無法再變更")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "刪除要求沒有記錄成功，可以再試一次")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"deletion_requested_at": updated.DeletionRequestedAt.Time.UTC().Format(time.RFC3339),
		"purge_after":           updated.DeletionRequestedAt.Time.Add(AccountDeletionGrace).UTC().Format(time.RFC3339),
		"cancellable":           true,
		"scope":                 deletionScope,
	})
}

func (h *Handler) cancelDeletion(w http.ResponseWriter, r *http.Request) {
	user, _ := SessionUser(r.Context())
	if _, err := h.Service.CancelAccountDeletion(r.Context(), user); err != nil {
		if errors.Is(err, ErrAccountPurging) {
			httpx.WriteError(w, http.StatusConflict, "刪除已經不可逆，無法再變更")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "取消刪除沒有記錄成功，可以再試一次")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"deletion_requested_at": nil})
}
