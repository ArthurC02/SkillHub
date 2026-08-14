package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
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
}

// Mount registers the auth routes on mux.
func (h *Handler) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/github/login", h.startLogin)
	mux.HandleFunc("GET /auth/github/callback", h.finishLogin)
	mux.HandleFunc("POST /auth/logout", h.logout)
	mux.HandleFunc("GET /me", h.RequireSession(h.me))
}

func (h *Handler) startLogin(w http.ResponseWriter, r *http.Request) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		httpError(w, http.StatusInternalServerError, "state generation failed")
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
		httpError(w, http.StatusUnauthorized, "oauth state mismatch")
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
		httpError(w, http.StatusUnauthorized, "code exchange failed")
		return
	}
	ghUser, err := h.Service.OAuth.FetchUser(ctx, accessToken)
	if err != nil {
		slog.Warn("github user fetch failed", "error", err)
		httpError(w, http.StatusUnauthorized, "user fetch failed")
		return
	}
	token, err := h.Service.LoginOrSignup(ctx, ghUser)
	if err != nil {
		slog.Error("login failed", "error", err)
		httpError(w, http.StatusInternalServerError, "login failed")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/",
		MaxAge: int(SessionTTL / time.Second),
		HttpOnly: true, Secure: h.Secure, SameSite: http.SameSiteLaxMode,
	})
	appURL := h.AppURL
	if appURL == "" {
		appURL = "/"
	}
	http.Redirect(w, r, appURL, http.StatusFound)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		if err := h.Service.Logout(r.Context(), c.Value); err != nil {
			httpError(w, http.StatusInternalServerError, "logout failed")
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
			httpError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		user, err := h.Service.UserForToken(r.Context(), c.Value)
		if err != nil {
			httpError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, user)))
	}
}

// SessionUser returns the user placed in the context by RequireSession.
func SessionUser(ctx context.Context) (gen.User, bool) {
	u, ok := ctx.Value(ctxKey{}).(gen.User)
	return u, ok
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	user, _ := SessionUser(r.Context())
	ws, err := h.Service.PersonalWorkspace(r.Context(), user)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "workspace lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"user_id":      uuidString(user.ID),
		"email":        user.Email,
		"display_name": user.DisplayName,
		"workspace_id": uuidString(ws.ID),
	})
}

func httpError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func uuidString(u pgtype.UUID) string {
	s, _ := u.Value()
	str, _ := s.(string)
	return str
}

func pgTime(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
