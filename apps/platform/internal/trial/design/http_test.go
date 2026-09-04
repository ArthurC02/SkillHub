package testlab

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFailWritesChineseLimitBody covers 丙-149: a limit refusal must reach the
// client as a plain Chinese sentence, with no English sentinel prefix such as
// "超過上限:" ahead of it (04 丙-138 boundary-strip precedent).
func TestFailWritesChineseLimitBody(t *testing.T) {
	err := fmt.Errorf("%w: 一個 Test Case 最多 %d 條驗收條件", ErrLimitExceeded, MaxCriteria)

	rec := httptest.NewRecorder()
	fail(rec, err, "should not be used")

	var body map[string]string
	if decodeErr := json.NewDecoder(rec.Body).Decode(&body); decodeErr != nil {
		t.Fatalf("decode response body: %v", decodeErr)
	}
	msg := body["error"]
	want := fmt.Sprintf("一個 Test Case 最多 %d 條驗收條件", MaxCriteria)
	if msg != want {
		t.Fatalf("error body = %q, want %q", msg, want)
	}
	if strings.HasPrefix(msg, ErrLimitExceeded.Error()) {
		t.Fatalf("error body %q still carries the English sentinel prefix %q", msg, ErrLimitExceeded.Error())
	}
}
