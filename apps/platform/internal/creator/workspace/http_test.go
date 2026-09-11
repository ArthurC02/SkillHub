package identity

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDevLoginRejectsOverlongNameInChinese(t *testing.T) {
	h := &Handler{DevLogin: true}
	body := strings.NewReader(`{"user":"` + strings.Repeat("a", 65) + `"}`)
	w := httptest.NewRecorder()
	h.devLogin(w, httptest.NewRequest("POST", "/auth/dev/login", body))

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

	h.devLogin(w, httptest.NewRequest("POST", "/auth/dev/login", body))

	if w.Code != 500 || !strings.Contains(w.Body.String(), "login failed") {
		t.Fatalf("status = %d body = %q, want 500 \"login failed\" from the unreachable store: a 64-character name must clear the length gate", w.Code, w.Body.String())
	}
}

func TestNotInvitedAnswersABrowserWithAChinesePage(t *testing.T) {
	browser := httptest.NewRequest("GET", "/downloads/x/content", nil)
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
	writeNotInvited(w, httptest.NewRequest("GET", "/downloads/x/content", nil))
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want JSON for an API caller", ct)
	}
	if !strings.Contains(w.Body.String(), "closed beta") {
		t.Errorf("API body = %q, want the unchanged betaNotInvited sentence", w.Body.String())
	}
}
