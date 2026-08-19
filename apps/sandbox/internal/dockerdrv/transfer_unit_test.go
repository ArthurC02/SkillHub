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
