package apiserver_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
)

func TestTheFeedbackMessageLimitCountsCharactersNotBytes(t *testing.T) {
	pool := requireDB(t)
	a := betaAPI(t, pool, policy.QuotaLimits{}, nil, 0)
	f := newFixture(t, a, pool, "feedback-rune-limit")

	atCap := strings.Repeat("字", 2000)
	if code, _ := f.doJSON(t, http.MethodPost, "/feedback", `{"kind":"need_signal","message":"`+atCap+`"}`); code != http.StatusNoContent {
		t.Fatalf("2000 Chinese characters: got %d, want 204", code)
	}
	if n := betaCount(t, pool, `SELECT count(*) FROM feedback_reports WHERE message = $1`, atCap); n != 1 {
		t.Errorf("2000 Chinese characters were stored %d times, want 1", n)
	}
	if code, _ := f.doJSON(t, http.MethodPost, "/feedback", `{"kind":"need_signal","message":"`+atCap+`字"}`); code != http.StatusBadRequest {
		t.Errorf("2001 Chinese characters: got %d, want 400", code)
	}
}

func TestAFeedbackBodyOverEightKiBIsRefused(t *testing.T) {
	pool := requireDB(t)
	a := betaAPI(t, pool, policy.QuotaLimits{}, nil, 0)
	f := newFixture(t, a, pool, "feedback-body-cap")

	padded := `{"kind":"need_signal","message":"padded past the body cap","page_path":"/` + strings.Repeat("p", 9000) + `"}`
	if code, _ := f.doJSON(t, http.MethodPost, "/feedback", padded); code != http.StatusBadRequest {
		t.Fatalf("a body over 8 KiB: got %d, want 400", code)
	}
	if n := betaCount(t, pool, `SELECT count(*) FROM feedback_reports WHERE message = 'padded past the body cap'`); n != 0 {
		t.Errorf("a body over 8 KiB was stored %d times", n)
	}
}
