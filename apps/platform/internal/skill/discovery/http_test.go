package catalog

import (
	"net/http"
	"net/http/httptest"
	"reflect"
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

func jsonFieldsOf(structType reflect.Type) map[string]bool {
	fields := map[string]bool{}
	for i := 0; i < structType.NumField(); i++ {
		name, _, _ := strings.Cut(structType.Field(i).Tag.Get("json"), ",")
		if name != "" && name != "-" {
			fields[name] = true
		}
	}
	return fields
}

func TestTheCatalogResponseDropsEveryFieldABrowseCouldOnlyFillWithAConstant(t *testing.T) {
	searchOnly := []string{"query", "degraded", "no_results", "query_suggestion"}

	catalog := jsonFieldsOf(reflect.TypeOf(catalogResponse{}))
	search := jsonFieldsOf(reflect.TypeOf(searchResponse{}))

	if len(catalog) == 0 {
		t.Fatal("the catalog response declares no json field at all, so this check would pass on an empty struct")
	}
	for _, field := range searchOnly {
		if !search[field] {
			t.Errorf("the search response no longer carries %q, so this check is comparing the catalog "+
				"against nothing", field)
			continue
		}
		if catalog[field] {
			t.Errorf("the catalog response carries %q, and a browse can only ever fill it with one value; "+
				"a field that never varies is not a field this response has", field)
		}
	}
}
