package identity

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// The dev-login body is a Chinese sentence a user can read, not the English
// error the handler used to write straight onto the wire (04 丙-149). The
// over-length name is rejected before Service is touched, so this needs no
// database.
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

// The download link is an <a href>, so a browser that follows it while not
// invited lands on the gate's answer with no page to catch it. That answer is a
// Chinese HTML page for a browser and stays the JSON body for an API caller
// (04 丙-149).
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
