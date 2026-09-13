package outbox

import (
	"encoding/json"
	"testing"
)

func marshalled(t *testing.T, payload any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("a published payload must marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("a published payload must be a JSON object: %v", err)
	}
	return decoded
}

func TestARunStatusChangeCarriesTheFieldsAConsumerReads(t *testing.T) {
	t.Parallel()
	fields := marshalled(t, RunStatusChanged{
		ToStatus: "succeeded", FromStatus: "evaluating", Reason: "the judge finished",
	})
	for key, want := range map[string]string{
		"to_status": "succeeded", "from_status": "evaluating", "reason": "the judge finished",
	} {
		if fields[key] != want {
			t.Errorf("%s = %v, want %q; consumers read this key by name", key, fields[key], want)
		}
	}
	if len(fields) != 3 {
		t.Errorf("the payload carries %d keys (%v); a key nobody declared is a contract nobody agreed to", len(fields), fields)
	}
}

func TestTheFirstStatusOfARunHasNoPreviousOne(t *testing.T) {
	t.Parallel()
	fields := marshalled(t, RunStatusChanged{ToStatus: "queued"})
	if _, present := fields["from_status"]; present {
		t.Error("a run entering its first status has nothing to report as the previous one")
	}
	if _, present := fields["reason"]; present {
		t.Error("an empty reason must be absent rather than an empty string")
	}
	if fields["to_status"] != "queued" {
		t.Errorf("to_status = %v, want queued", fields["to_status"])
	}
}

func TestACleanupChangeCarriesItsStatusAndFailureCount(t *testing.T) {
	t.Parallel()
	fields := marshalled(t, RunCleanupChanged{CleanupStatus: "failed", FailureCount: 3})
	if fields["cleanup_status"] != "failed" {
		t.Errorf("cleanup_status = %v, want failed", fields["cleanup_status"])
	}
	if fields["failure_count"] != float64(3) {
		t.Errorf("failure_count = %v, want 3", fields["failure_count"])
	}
	if len(fields) != 2 {
		t.Errorf("the payload carries %d keys (%v)", len(fields), fields)
	}
}

func TestACleanCleanupReportsNoFailureCount(t *testing.T) {
	t.Parallel()
	fields := marshalled(t, RunCleanupChanged{CleanupStatus: "cleaned"})
	if _, present := fields["failure_count"]; present {
		t.Error("a cleanup with nothing to report must omit the count rather than publish a zero")
	}
}
