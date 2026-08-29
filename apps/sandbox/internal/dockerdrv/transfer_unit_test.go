package dockerdrv

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchRejectsAnOversizedGrantedObject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "67108865")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := fetch(context.Background(), srv.URL+"/signed?secret=must-not-leak")
	if err == nil {
		t.Fatal("oversized object was accepted")
	}
	if strings.Contains(err.Error(), srv.URL) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("grant URL leaked into error: %v", err)
	}
}

func TestFetchRejectsOversizedChunkedObject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher := w.(http.Flusher)
		_, _ = w.Write([]byte("first"))
		flusher.Flush() // forces chunked transfer with ContentLength == -1
		_, _ = w.Write([]byte("-chunk-exceeds-limit"))
	}))
	defer srv.Close()

	_, err := fetchWithLimit(context.Background(), srv.URL+"/signed?secret=must-not-leak", 5)
	if err == nil {
		t.Fatal("oversized chunked object was accepted")
	}
	if strings.Contains(err.Error(), srv.URL) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("grant URL leaked into error: %v", err)
	}
}

// TestFetchRefusesToFollowARedirect: a grant URL is pre-signed by the control
// plane and names one object, so a 3xx is never a hop to take. Go's default
// client would have taken up to ten of them, and sandboxd holds the node's own
// network reach — enough to be asked to fetch a link-local metadata address on
// the storage endpoint's behalf.
//
// The redirect target is a second httptest server that would answer 200 with a
// body, so following it would succeed and this test would only go red on the
// refusal actually being there.
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
