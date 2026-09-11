package catalog

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQueryTooLongAcceptsExactlyTheRuneCap(t *testing.T) {
	if got := queryTooLong(strings.Repeat("x", maxQueryRunes)); got != "" {
		t.Errorf("queryTooLong at exactly the rune cap = %q, want accepted", got)
	}
	if got := queryTooLong(strings.Repeat("x", maxQueryRunes+1)); got == "" {
		t.Error("queryTooLong one over the rune cap was accepted")
	}
}

func TestPublicSearchRejectsAnUnknownPurpose(t *testing.T) {
	h := &Handler{Svc: &Service{}}
	req := httptest.NewRequest(http.MethodGet, "/api/skills/search?q=abc&purpose=other", nil)
	rec := httptest.NewRecorder()
	h.PublicSearch(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestPublicSearchAcceptsPurposeReference(t *testing.T) {
	h := &Handler{Svc: &Service{}}
	req := httptest.NewRequest(http.MethodGet, "/api/skills/search?q=&purpose=reference", nil)
	rec := httptest.NewRecorder()
	h.PublicSearch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (blank query, valid purpose)", rec.Code)
	}
}
