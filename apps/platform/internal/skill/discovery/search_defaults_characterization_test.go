package catalog

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestASearchWithoutALimitReturnsTwentyResults(t *testing.T) {
	limit, err := parseLimit(httptest.NewRequest(http.MethodGet, "/search?q=ledger", nil))
	if err != nil || limit != 20 {
		t.Fatalf("parseLimit without limit = %d, %v; want 20", limit, err)
	}
}

func TestAnUnmeasuredSkillShowsEveryCompatibilityAxisAsUnverified(t *testing.T) {
	c := unverifiedCompat()
	for axis, got := range map[string]labelled{"spec": c.SpecValidation, "capability": c.Capability, "runtime": c.Runtime} {
		if got.Value != "unverified" || got.Label != "未驗證" || got.Note != "" {
			t.Errorf("%s axis = %+v, want unverified / 未驗證 with no note", axis, got)
		}
	}
}
