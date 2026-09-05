package catalog

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ADR-066's `purpose=reference` is the only value the contract accepts besides
// absent; anything else is refused the same way an unrecognised DISC-003
// filter is, before retrieval is ever touched (no Pool is configured on this
// Handler, so reaching Service.Search would panic rather than 500).
func TestPublicSearchRejectsAnUnknownPurpose(t *testing.T) {
	h := &Handler{Svc: &Service{}}
	req := httptest.NewRequest(http.MethodGet, "/api/skills/search?q=abc&purpose=other", nil)
	rec := httptest.NewRecorder()
	h.PublicSearch(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// purpose is optional: its absence must not be treated as an unrecognised
// value. This case reaches Service.Search (q is blank/incomprehensible-free
// here would still hit retrieval), so it is exercised only far enough to prove
// the 400 above is about the VALUE and not about parsing the parameter at all.
func TestPublicSearchAcceptsPurposeReference(t *testing.T) {
	h := &Handler{Svc: &Service{}}
	req := httptest.NewRequest(http.MethodGet, "/api/skills/search?q=&purpose=reference", nil)
	rec := httptest.NewRecorder()
	h.PublicSearch(rec, req)
	// q="" takes the blank-query branch, which never reaches retrieval and so
	// needs no Pool — this proves purpose=reference does not 400 by itself.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (blank query, valid purpose)", rec.Code)
	}
}
