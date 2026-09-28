package testlab

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseListLimitWithoutALimitParameterReturnsFiftyOne(t *testing.T) {
	got, err := parseListLimit(httptest.NewRequest(http.MethodGet, "/test-cases", nil))
	if err != nil {
		t.Fatalf("parseListLimit without limit: unexpected error %v", err)
	}
	if got != 51 {
		t.Fatalf("parseListLimit without limit = %d, want 51", got)
	}
}
