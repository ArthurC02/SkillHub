package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/services/platform/internal/audit"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/httpx"
)

const (
	sessionCookie = "sh_session"
	stateCookie   = "sh_oauth_state"
)

// Handler exposes the auth endpoints declared in contracts/openapi/public.yaml.
type Handler struct {
	Service *Service
	// Secure controls the cookie Secure flag; false only for plain-http local dev.
	Secure bool
	// AppURL is where the callback redirects after login (default "/").
	AppURL string
	// DevLogin mounts the offline dev provider (ADR-020). Local demo and E2E
	// only; must never be set in production.
	DevLogin bool
	// Operators is the 02:SEC-011 platform operator allowlist, keyed by user id
	// and filled from deployment configuration (OPERATOR_USER_IDS in cmd/api).
	//
	// A map and not a database role table because the team is one person: a role
	// table needs its own grant endpoint, its own authorization for that endpoint
	// and its own audit trail for grants, and all three would exist to let one
	// account promote itself. Granting is editing the deployment's environment and
	// restarting, which nobody who cannot already deploy can do.
	//
	// Empty is the correct default and the shipped one: nobody is an operator, and
	// every operator route answers 404 to everybody. The upgrade path when a second
	// person appears is a roles table read here instead, not a change to the
	// routes or the handlers.
	Operators map[string]bool
}

// Mount registers the auth routes on mux.
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

// devLogin signs in as a named dev user without any external network — the
// offline demo path (ADR-020). Reachable only when DevLogin is true.
func (h *Handler) devLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		User string `json:"user"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body) // empty body → default user
	name := body.User
	if name == "" {
		name = "dev"
	}
	if len(name) > 64 {
		httpx.WriteError(w, http.StatusBadRequest, "user name too long")
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
	// One-shot state check (ADR-020): cookie must exist and match the query.
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
			httpx.WriteError(w, http.StatusInternalServerError, "logout failed")
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

// RequireSession resolves the session cookie to a user and stores it in the
// request context; without a valid session the request ends with 401.
// Public reads (DISC-010) simply do not use this wrapper.
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

// LogOperatorRoster records the operator list this process came up with
// (02:SEC-011 「授予或撤銷 operator 角色本身也是 audit event」).
//
// Written on every start, not only when the list changes: comparing against the
// previous event would mean reading the trail to decide whether to append to it,
// and a roster that silently persisted across a restart nobody logged is the
// state this row exists to make impossible. Duplicates are cheap; a gap is not.
//
// Its limits, stated rather than papered over: it records *what* the roster is,
// never who granted it or when — that fact lives in whatever changed the
// deployment's environment. A roster table with its own grant path is the
// upgrade, and 02:SEC-011 describes it.
func (h *Handler) LogOperatorRoster(ctx context.Context) error {
	ids := make([]string, 0, len(h.Operators))
	for id := range h.Operators {
		ids = append(ids, id)
	}
	sort.Strings(ids) // stable across restarts, so two events are comparable
	return audit.Log(ctx, gen.New(h.Service.Pool), audit.Event{
		// No actor: nobody performed this through the platform (audit.Event's
		// zero actor is "platform-initiated", which a start-up is).
		Action:       audit.ActionOperatorRoster,
		ResourceType: audit.ResourceOperatorRoster,
		Metadata: map[string]any{
			"user_ids": ids,
			"count":    len(ids),
			"source":   "OPERATOR_USER_IDS",
		},
	})
}

// RequireOperator is RequireSession for the 02:SEC-011 operator routes, with one
// difference that is the whole point of it: everybody who is not on the
// deployment's operator list gets 404, not 401 and not 403.
//
// 401 would tell an anonymous caller the route exists and that logging in is the
// next step; 403 would tell a signed-in member that there is a privilege they do
// not have. SEC-011 asks for neither to be knowable ("不揭露資源與端點存在", the
// same non-disclosure rule SEC-008 applies to other people's content), so the
// answer here is the answer the mux gives for a path nobody ever wrote.
//
// Being on the list is *not* a widened workspace scope. Nothing downstream may
// read workspace-private data on the strength of it (SEC-011 最小權力原則); the
// session user goes into the context only so the handler can name an actor in
// the audit event.
func (h *Handler) RequireOperator(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil || c.Value == "" {
			httpx.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		user, err := h.Service.UserForToken(r.Context(), c.Value)
		if err != nil || !h.Operators[uuidString(user.ID)] {
			httpx.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, user)))
	}
}

// SessionUser returns the user placed in the context by RequireSession or
// OptionalSession.
func SessionUser(ctx context.Context) (gen.User, bool) {
	u, ok := ctx.Value(ctxKey{}).(gen.User)
	return u, ok
}

// OptionalSession resolves the session cookie when present and valid, storing
// the user in the request context, but never rejects the request — missing or
// invalid sessions just mean SessionUser returns ok=false downstream. For
// public reads (search, skill detail) that personalize when logged in
// (DISC-010, ADR-020).
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
	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"user_id":      uuidString(user.ID),
		"email":        user.Email,
		"display_name": user.DisplayName,
		"workspace_id": uuidString(ws.ID),
	})
}

// deletionScope is the WS-002/PDM-006 §6.1 requirement that the deletion scope
// is stated up front, not discovered afterwards: the user has to know that
// content other people built on keeps existing without their name on it.
const deletionScope = "Your account stays usable until the grace period ends. " +
	"At that point your uploaded datasets, run artifacts, and every skill nobody " +
	"else forked or ran are permanently deleted, files included. Skill versions " +
	"that other users forked or that historical runs used are kept — their content " +
	"is another user's provenance chain — but your identity is removed from them " +
	"and they show as belonging to a deleted user."

func (h *Handler) requestDeletion(w http.ResponseWriter, r *http.Request) {
	user, _ := SessionUser(r.Context())
	updated, err := h.Service.RequestAccountDeletion(r.Context(), user)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "deletion request failed")
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
		httpx.WriteError(w, http.StatusInternalServerError, "deletion cancel failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"deletion_requested_at": nil})
}

func uuidString(u pgtype.UUID) string {
	s, _ := u.Value()
	str, _ := s.(string)
	return str
}

func pgTime(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
