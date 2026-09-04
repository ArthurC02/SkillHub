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
