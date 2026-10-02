package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAnUnreadableInviteListRefusesRatherThanAdmitting(t *testing.T) {
	pool, err := pgxpool.New(context.Background(),
		"postgres://nobody@127.0.0.1:1/nothing?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	h := &Handler{
		Service: &Service{Pool: pool},
		Invited: map[string]bool{"github-user-on-the-list": true},
	}

	admitted := false
	guarded := h.RequireInvited(func(http.ResponseWriter, *http.Request) { admitted = true })

	r := httptest.NewRequest(http.MethodGet, "/api/skills", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxKey{},
		User{ID: pgtype.UUID{Bytes: [16]byte{9}, Valid: true}, Email: "someone@example.com"}))
	w := httptest.NewRecorder()
	guarded(w, r)

	if admitted {
		t.Fatal("a user was admitted to the closed beta on a lookup that never answered")
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503: an unanswerable check is not a 403 either — "+
			"403 tells the user they are not invited, which is a statement nobody made",
			w.Code)
	}
}

func TestAnUnreadableInviteListIsAnErrorAndNotAnAdmission(t *testing.T) {
	pool, err := pgxpool.New(context.Background(),
		"postgres://nobody@127.0.0.1:1/nothing?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	h := &Handler{
		Service: &Service{Pool: pool},
		Invited: map[string]bool{"github-user-on-the-list": true},
	}
	user := User{ID: pgtype.UUID{Bytes: [16]byte{9}, Valid: true}, Email: "someone@example.com"}

	invited, err := h.invited(context.Background(), user)
	if err == nil {
		t.Fatal("a lookup that never answered returned no error, so both callers would read it as a decision")
	}
	if invited {
		t.Fatal("a lookup that never answered returned true")
	}
}

func TestAClosedBetaGateRefusesEveryoneWithoutAskingTheDatabase(t *testing.T) {
	pool, err := pgxpool.New(context.Background(),
		"postgres://nobody@127.0.0.1:1/nothing?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	h := &Handler{
		Service: &Service{Pool: pool},
		Invited: map[string]bool{"github-user-on-the-list": true},
	}
	h.CloseBetaGate()

	admitted := false
	guarded := h.RequireInvited(func(http.ResponseWriter, *http.Request) { admitted = true })
	r := httptest.NewRequest(http.MethodGet, "/api/skills", nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxKey{},
		User{ID: pgtype.UUID{Bytes: [16]byte{9}, Valid: true}, Email: "someone@example.com"}))
	w := httptest.NewRecorder()
	guarded(w, r)

	if admitted || w.Code != http.StatusForbidden {
		t.Errorf("admitted = %v status = %d, want a 403 refusal from a closed gate", admitted, w.Code)
	}
	if !h.BetaGateActive() {
		t.Error("a closed gate reported itself inactive, so the invite-only surfaces open to everyone")
	}
	if h.allowlisted([]string{"github-user-on-the-list"}) {
		t.Error("a user once on the list still counts as allowlisted behind a closed gate")
	}
}

func TestASessionThatCannotBeCheckedIsUnavailableRatherThanSignedOut(t *testing.T) {
	pool, err := pgxpool.New(context.Background(),
		"postgres://nobody@127.0.0.1:1/nothing?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	h := &Handler{Service: &Service{Pool: pool}}

	for name, guard := range map[string]func(http.HandlerFunc) http.HandlerFunc{
		"required": h.RequireSession, "operator": h.RequireOperator, "optional": h.OptionalSession,
	} {
		t.Run(name, func(t *testing.T) {
			reached := false
			r := httptest.NewRequest(http.MethodGet, "/me", nil)
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "a-session-token"})
			w := httptest.NewRecorder()

			guard(func(http.ResponseWriter, *http.Request) { reached = true })(w, r)

			if reached || w.Code != http.StatusServiceUnavailable {
				t.Errorf("reached = %v status = %d, want 503: an outage is not a signed-out user", reached, w.Code)
			}
		})
	}
}

func TestAccountFeaturesAreUnavailableWhenTheInviteCannotBeChecked(t *testing.T) {
	pool, err := pgxpool.New(context.Background(),
		"postgres://nobody@127.0.0.1:1/nothing?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	user := User{ID: pgtype.UUID{Bytes: [16]byte{7}, Valid: true}, Email: "someone@example.com"}

	for _, tc := range []struct {
		name       string
		invited    map[string]bool
		wantStatus int
	}{
		{"no beta gate, nothing to check", nil, http.StatusOK},
		{"beta gate, invite unreadable", map[string]bool{"someone-else": true}, http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{Service: &Service{Pool: pool}, Invited: tc.invited, Features: map[string]bool{"creation": true}}
			r := httptest.NewRequest(http.MethodGet, "/me", nil)
			ctx := context.WithValue(r.Context(), ctxKey{}, user)
			ctx = context.WithValue(ctx, sessionWorkspaceKey{}, Workspace{OwnerUserID: user.ID})
			w := httptest.NewRecorder()

			h.me(w, r.WithContext(ctx))

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d body = %s, want %d", w.Code, w.Body, tc.wantStatus)
			}
			if tc.wantStatus == http.StatusOK && !strings.Contains(w.Body.String(), `"creation":true`) {
				t.Errorf("body = %s, want the features of an ungated account", w.Body)
			}
		})
	}
}
