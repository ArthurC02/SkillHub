package localdrv

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFetchRefusesToFollowARedirect is the localdrv half of the same rule
// dockerdrv's transfer_unit_test.go states: a pre-signed grant URL names one
// object, so a 3xx is never a hop to take, and both drivers share
// sandbox.GrantHTTPClient so that neither can quietly go back to
// http.DefaultClient's ten-redirect default.
func TestFetchRefusesToFollowARedirect(t *testing.T) {
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("this must never be fetched"))
	}))
	defer elsewhere.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL, http.StatusFound)
	}))
	defer srv.Close()

	body, err := fetch(context.Background(), srv.URL+"/signed?secret=must-not-leak")
	if err == nil {
		t.Fatalf("a redirected grant URL was followed and returned %q", body)
	}
	if !strings.Contains(err.Error(), "302") {
		t.Errorf("the refusal should report the redirect status it saw; got %v", err)
	}
	if strings.Contains(err.Error(), srv.URL) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("grant URL leaked into error: %v", err)
	}
}
