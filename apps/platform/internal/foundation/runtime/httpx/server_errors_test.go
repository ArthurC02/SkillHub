package httpx

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOnlyAServerErrorIsLoggedWithItsRoute(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		wantLog bool
	}{
		{"server error", http.StatusInternalServerError, true},
		{"unavailable", http.StatusServiceUnavailable, true},
		{"last client error", 499, false},
		{"refusal", http.StatusForbidden, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logged bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
			t.Cleanup(func() { slog.SetDefault(previous) })

			h := LogServerErrors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				WriteError(w, tc.status, "message")
			}))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/skills/abc", nil))

			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d passed through", w.Code, tc.status)
			}
			if got := strings.Contains(logged.String(), "path=/skills/abc"); got != tc.wantLog {
				t.Errorf("logged = %v, want %v: %q", got, tc.wantLog, logged.String())
			}
		})
	}
}

func TestAStreamBehindTheServerErrorLogStillFlushes(t *testing.T) {
	h := LogServerErrors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("the wrapped writer can no longer flush")
		}
		_, _ = w.Write([]byte("data: 1\n\n"))
		f.Flush()
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/stream", nil))
	if !w.Flushed {
		t.Error("a flush through the wrapper never reached the client")
	}
}
