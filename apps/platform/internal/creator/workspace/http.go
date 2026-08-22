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

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
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
	// Invited is the BETA-001 admission list (ADR-028 決策 1), keyed by
	// user_identities.provider_user_id and filled from deployment configuration
	// (BETA_ALLOWLIST in cmd/api).
	//
	// The same shape as Operators above and for the same three reasons SEC-011
	// gave: a table of invitees would need a grant endpoint, authorization for that
	// endpoint, and an audit trail for the grants, and all three would exist so one
	// account could add itself. Inviting somebody is editing the deployment's
	// environment and restarting.
	//
	// Empty is the shipped default and it means the gate is off — every signed-in
	// user is admitted, which is what M0 through M3 have been. It is not a
	// fail-open hole: the gate exists to bound the cost of a public deployment
	// during a closed beta, and a deployment that has not named a beta cohort does
	// not have one.
	//
	// It is not a role and it is not a wider scope. Being on it grants nothing
	// beyond what an ordinary signed-in user already has; being off it removes
	// Fork, run creation and download, and leaves search and skill detail exactly
	// as they were (DISC-010 already serves those to nobody in particular).
	Invited map[string]bool
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
	return audit.Log(ctx, h.Service.Pool, audit.Event{
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
		if err != nil || !h.Operators[pgconv.UUIDString(user.ID)] {
			httpx.WriteError(w, http.StatusNotFound, "not found")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, user)))
	}
}

// betaNotInvited is what somebody outside the cohort is told. Deliberately an
// explanation and an invitation to say what they wanted, not a wall: the catalogue
// has just shown this person the content, so pretending the feature does not exist
// would be contradicted by the next request they make (the same reasoning that put
// 403 rather than 404 on /files in 02:SEC-011).
const betaNotInvited = "Skill Hub is in closed beta: browsing and skill details are open to " +
	"everyone, but forking, trial runs and downloads are limited to the invited testers. " +
	"Tell us what you were trying to do at POST /feedback with kind=need_signal and it goes " +
	"straight into the scope review."

// RequireInvited is the BETA-001 admission gate (ADR-028 決策 1), layered on top
// of RequireSession rather than replacing it: login is still GitHub OAuth and this
// only decides what a logged-in account may reach.
//
// 403 and not 404, which is the opposite of RequireOperator two functions up, and
// the difference is deliberate. An operator route's existence is itself meant to
// be secret. A closed beta's is not — the product says so on its own front page,
// and the visitor was just served the catalogue by the same API. Hiding the
// endpoint would be a fiction their next request disproves.
//
// There is no exemption for the dev provider and no second code path for it.
// DEV_LOGIN's offline demo (ADR-020) is untouched because a development
// deployment sets no BETA_ALLOWLIST and the gate is therefore off — not because
// this function knows about it. An exemption would be a way past the gate that
// exists only in the build where the gate matters least, and the identity table
// keys dev logins the same way it keys GitHub ones, so a dev deployment that does
// want the gate simply lists the names it uses.
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
		ids, err := h.Service.queries().GetIdentityProviderIDs(r.Context(), user.ID)
		if err != nil {
			// Fail closed. The list is a cost ceiling on a publicly reachable
			// deployment, and "we could not read who you are" is not "you are on
			// the list" (the SEC-002 rule, applied to admission).
			httpx.WriteError(w, http.StatusServiceUnavailable, "invite check unavailable")
			return
		}
		for _, id := range ids {
			if h.Invited[id.ProviderUserID] {
				next(w, r)
				return
			}
		}
		httpx.WriteError(w, http.StatusForbidden, betaNotInvited)
	}
}

// LogInviteRoster records the beta cohort this process came up with, in the same
// form and for the same reason as LogOperatorRoster above (ADR-028 決策 1 sends it
// to that precedent explicitly).
//
// It answers "who is on the list now" and deliberately not "who was added when" —
// that fact lives in the deployment configuration's own change history, and
// claiming otherwise would make this look like a grant audit it is not.
//
// Fail-closed, and here that word means something different from the operator
// roster: a start-up that cannot record the cohort recognises nobody as invited,
// which with a configured list closes the gate on everyone rather than opening it.
func (h *Handler) LogInviteRoster(ctx context.Context) error {
	if len(h.Invited) == 0 {
		return nil // no cohort configured, no gate, nothing to record
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

// SessionUser returns the user placed in the context by RequireSession or
// OptionalSession.
func SessionUser(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
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
	// deletion_requested_at is 02:SEC-006's "刪除工作具可追蹤狀態". Until now the
	// only place it appeared was the response to DELETE /me itself, so a user who
	// closed the tab had no way to ask whether the request had been recorded, and
	// no way to find the date the grace period runs out from. Both are null when no
	// deletion is pending, which is the difference between "not requested" and
	// "requested and I cannot tell".
	out := map[string]any{
		"user_id":               pgconv.UUIDString(user.ID),
		"email":                 user.Email,
		"display_name":          user.DisplayName,
		"workspace_id":          pgconv.UUIDString(ws.ID),
		"deletion_requested_at": nil,
		"purge_after":           nil,
		// The scope sentence used to exist only in the response to DELETE /me, so
		// it survived exactly one render: reload the page and the disclosure
		// 02:WS-002 第 3 條 asks for was gone, while the grace period it describes
		// ran on. One constant, both endpoints — a second copy of this wording is
		// a second thing to keep true.
		"deletion_scope": nil,
	}
	if user.DeletionRequestedAt.Valid {
		at := user.DeletionRequestedAt.Time.UTC()
		out["deletion_requested_at"] = at.Format(time.RFC3339)
		out["purge_after"] = at.Add(AccountDeletionGrace).UTC().Format(time.RFC3339)
		out["deletion_scope"] = deletionScope
	}
	httpx.WriteJSON(w, http.StatusOK, out)
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
