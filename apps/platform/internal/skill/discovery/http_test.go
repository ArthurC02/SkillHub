package catalog

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
