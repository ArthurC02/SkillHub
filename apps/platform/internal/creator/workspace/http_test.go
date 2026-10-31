package identity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAccountViewKeepsUnavailableAccountDeletionFieldsExplicit(t *testing.T) {
	encoded, err := json.Marshal(accountView{UserID: "user", Email: "user@example.test", DisplayName: "User", WorkspaceID: "workspace"})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"deletion_requested_at", "purge_after", "deletion_scope"} {
		value, present := body[field]
		if !present || value != nil {
			t.Errorf("%s = %v (present %v), want explicit null", field, value, present)
		}
	}
	if _, present := body["features"]; present {
		t.Errorf("features = %v, want it omitted when no feature is available", body["features"])
	}
}

func TestDevLoginRejectsOverlongNameInChinese(t *testing.T) {
	h := &Handler{DevLogin: true}
	body := strings.NewReader(`{"user":"` + strings.Repeat("a", 65) + `"}`)
	w := httptest.NewRecorder()
	h.devLogin(w, httptest.NewRequest(http.MethodPost, "/auth/dev/login", body))

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "使用者名稱最多 64 個字元") {
		t.Errorf("body = %q, want the Chinese length message", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "too long") {
		t.Errorf("body still carries the English sentence: %q", w.Body.String())
	}
}

func TestDevLoginLetsANameAtTheBoundaryThroughToLogin(t *testing.T) {
	pool, err := pgxpool.New(context.Background(),
		"postgres://nobody@127.0.0.1:1/nothing?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	h := &Handler{DevLogin: true, Service: &Service{Pool: pool}}
	body := strings.NewReader(`{"user":"` + strings.Repeat("a", 64) + `"}`)
	w := httptest.NewRecorder()

	h.devLogin(w, httptest.NewRequest(http.MethodPost, "/auth/dev/login", body))

	if w.Code != 500 || !strings.Contains(w.Body.String(), "login failed") {
		t.Fatalf("status = %d body = %q, want 500 \"login failed\" from the unreachable store: a 64-character name must clear the length gate", w.Code, w.Body.String())
	}
}

func TestNotInvitedAnswersABrowserWithAChinesePage(t *testing.T) {
	browser := httptest.NewRequest(http.MethodGet, "/downloads/x/content", nil)
	browser.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")
	w := httptest.NewRecorder()
	writeNotInvited(w, browser)
	if w.Code != 403 {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html for a browser navigation", ct)
	}
	if !strings.Contains(w.Body.String(), "受邀的測試者") {
		t.Errorf("body = %q, want the Chinese refusal", w.Body.String())
	}

	w = httptest.NewRecorder()
	writeNotInvited(w, httptest.NewRequest(http.MethodGet, "/downloads/x/content", nil))
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want JSON for an API caller", ct)
	}
	if !strings.Contains(w.Body.String(), "封測") {
		t.Errorf("API body = %q; the app prints this sentence inside a Chinese screen", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "/feedback") {
		t.Errorf("API body = %q; it tells the reader to call an endpoint rather than use the page "+
			"they are on", w.Body.String())
	}
}

func TestTheLoginRoutesRefuseCleanlyWhenNoProviderIsConfigured(t *testing.T) {
	h := &Handler{Service: &Service{}}
	for _, tc := range []struct {
		name   string
		serve  func(http.ResponseWriter, *http.Request)
		target string
	}{
		{"start", h.startLogin, "/auth/github/login"},
		{"callback", h.finishLogin, "/auth/github/callback?state=x&code=y"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.serve(w, httptest.NewRequest(http.MethodGet, tc.target, nil))

			if got := loginFailureOf(t, w); got != loginNoProvider {
				t.Fatalf("sent the browser back with %q, want %q when no identity provider is configured", got, loginNoProvider)
			}
		})
	}
}

func TestDevLoginStopsReadingABodyPastFourKilobytes(t *testing.T) {
	pool, err := pgxpool.New(context.Background(),
		"postgres://nobody@127.0.0.1:1/nothing?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	h := &Handler{DevLogin: true, Service: &Service{Pool: pool}}
	overlongNameBehindPadding := strings.Repeat(" ", 4096) + `{"user":"` + strings.Repeat("a", 65) + `"}`
	w := httptest.NewRecorder()

	h.devLogin(w, httptest.NewRequest(http.MethodPost, "/auth/dev/login", strings.NewReader(overlongNameBehindPadding)))

	if w.Code != 500 || !strings.Contains(w.Body.String(), "login failed") {
		t.Fatalf("status = %d body = %q, want the default dev user's login attempt: the name past 4 KiB must never be read", w.Code, w.Body.String())
	}
}

func requestWithSessionUser(user User, workspace *Workspace) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	ctx := context.WithValue(r.Context(), ctxKey{}, user)
	if workspace != nil {
		ctx = context.WithValue(ctx, sessionWorkspaceKey{}, *workspace)
	}
	return r.WithContext(ctx)
}

func unreachableService(t *testing.T) *Service {
	t.Helper()
	pool, err := pgxpool.New(context.Background(),
		"postgres://nobody@127.0.0.1:1/nothing?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return &Service{Pool: pool}
}

func TestSessionWorkspaceAnswers401WithoutASession(t *testing.T) {
	w := httptest.NewRecorder()
	_, ws, ok := SessionWorkspace(&Service{}, w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if ok || ws.ID.Valid {
		t.Fatalf("ok = %v workspace = %+v, want a refusal with no workspace", ok, ws)
	}
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), `"not authenticated"`) {
		t.Fatalf("status = %d body = %q, want 401 \"not authenticated\"", w.Code, w.Body.String())
	}
}

func TestSessionWorkspaceAnswers500WhenTheWorkspaceLookupFails(t *testing.T) {
	w := httptest.NewRecorder()
	user := User{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}}
	_, ws, ok := SessionWorkspace(unreachableService(t), w, requestWithSessionUser(user, nil))
	if ok || ws.ID.Valid {
		t.Fatalf("ok = %v workspace = %+v, want a refusal with no workspace", ok, ws)
	}
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), `"workspace lookup failed"`) {
		t.Fatalf("status = %d body = %q, want 500 \"workspace lookup failed\"", w.Code, w.Body.String())
	}
}

func TestSessionWorkspaceReturnsTheUserAndWorkspaceAndWritesNothing(t *testing.T) {
	user := User{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}}
	want := Workspace{ID: pgtype.UUID{Bytes: [16]byte{2}, Valid: true}, OwnerUserID: user.ID}
	w := httptest.NewRecorder()

	gotUser, gotWorkspace, ok := SessionWorkspace(&Service{}, w, requestWithSessionUser(user, &want))

	if !ok || gotUser.ID != user.ID || gotWorkspace.ID != want.ID {
		t.Fatalf("ok = %v user = %v workspace = %v, want the session user and their workspace", ok, gotUser.ID, gotWorkspace.ID)
	}
	if w.Body.Len() != 0 || w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %q, want nothing written", w.Code, w.Body.String())
	}
}

func TestSessionWorkspaceIDIsZeroWithoutASessionOrOnLookupFailure(t *testing.T) {
	user := User{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}}
	if got := SessionWorkspaceID(&Service{}, httptest.NewRequest(http.MethodGet, "/x", nil)); got.Valid {
		t.Errorf("no session: id = %v, want zero", got)
	}
	if got := SessionWorkspaceID(unreachableService(t), requestWithSessionUser(user, nil)); got.Valid {
		t.Errorf("lookup failure: id = %v, want zero", got)
	}
}

func TestSessionWorkspaceIDReturnsThePersonalWorkspaceID(t *testing.T) {
	user := User{ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}}
	want := Workspace{ID: pgtype.UUID{Bytes: [16]byte{2}, Valid: true}, OwnerUserID: user.ID}
	if got := SessionWorkspaceID(&Service{}, requestWithSessionUser(user, &want)); got != want.ID {
		t.Errorf("id = %v, want %v", got, want.ID)
	}
}

func TestALoginRefusedMidPurgeSaysInChineseThatTheAccountIsGoingAway(t *testing.T) {
	w := httptest.NewRecorder()

	if !writeRefusedLogin(w, ErrAccountPurging) {
		t.Fatal("a purging account was not refused")
	}

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", w.Code)
	}
	if !strings.Contains(w.Body.String(), "這個帳號正在永久刪除") {
		t.Errorf("body = %q, want the plain Chinese reason", w.Body.String())
	}
}
