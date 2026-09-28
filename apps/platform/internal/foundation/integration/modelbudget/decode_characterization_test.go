package modelbudget

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func budgetBodyOfLength(t *testing.T, n int) string {
	t.Helper()
	head, tail := `{"seconds":30,"reason":"`, `"}`
	return head + strings.Repeat("r", n-len(head)-len(tail)) + tail
}

func TestABudgetRequestOfExactlyFourKilobytesIsRead(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/admin/model-budgets/judge", strings.NewReader(budgetBodyOfLength(t, 4096)))
	body, ok := decode(httptest.NewRecorder(), req)
	if !ok || body.Seconds != 30 {
		t.Errorf("a 4096-byte request decoded = %v, seconds %d; want accepted with 30", ok, body.Seconds)
	}
}

func TestABudgetRequestOneByteOverFourKilobytesIsRefused(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/admin/model-budgets/judge", strings.NewReader(budgetBodyOfLength(t, 4097)))
	rec := httptest.NewRecorder()
	if _, ok := decode(rec, req); ok || rec.Code != http.StatusBadRequest {
		t.Errorf("a 4097-byte request decoded = %v with status %d; want refused with 400", ok, rec.Code)
	}
}
