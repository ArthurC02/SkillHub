package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTransportSendsJSONAndBearerToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization = %q, want bearer token", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content type = %q, want JSON", got)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	status, response, err := (Transport{Token: "token"}).Do(context.Background(), http.MethodPost, srv.URL, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || string(response) != `{"ok":true}` {
		t.Fatalf("response = %d %q, want 200 JSON", status, response)
	}
}

func TestTransportRejectsAResponsePastItsLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("12345"))
	}))
	defer srv.Close()

	status, _, err := (Transport{ResponseLimit: 4}).Do(context.Background(), http.MethodGet, srv.URL, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if err == nil || !strings.Contains(err.Error(), "4") {
		t.Fatalf("error = %v, want the response limit", err)
	}
}

func TestTransportAcceptsAResponseAtItsExactLimit(t *testing.T) {
	const limit = int64(4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("1234"))
	}))
	defer srv.Close()

	status, response, err := (Transport{ResponseLimit: limit}).Do(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || string(response) != "1234" {
		t.Fatalf("response = %d %q, want 200 exact limit", status, response)
	}
}
