package trace

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func agentOutputEventOfSize(t *testing.T, size int) Event {
	t.Helper()
	const head, tail = `{"kind":"final","truncated":false,"text":"`, `"}`
	payload := head + strings.Repeat("x", size-len(head)-len(tail)) + tail
	if len(payload) != size {
		t.Fatalf("built a %d byte payload, want %d", len(payload), size)
	}
	return Event{
		SchemaVersion: "1.0", EventID: "0f0a1e6c-1c9a-4f8e-9a2b-1d5a2c7b3e01",
		RunID: "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20", Attempt: 1, Seq: 1,
		OccurredAt: time.Now(), EmittedBy: SourceSandbox, Type: TypeAgentOutput,
		Payload: json.RawMessage(payload),
	}
}

func TestValidateAcceptsAWellFormedPayloadAtTheSizeCeiling(t *testing.T) {
	event := agentOutputEventOfSize(t, maxPayloadBytes)
	if err := event.Validate(); err != nil {
		t.Fatalf("a %d byte payload was rejected: %v", maxPayloadBytes, err)
	}
}

func TestValidateRefusesAWellFormedPayloadOneByteOverTheSizeCeiling(t *testing.T) {
	event := agentOutputEventOfSize(t, maxPayloadBytes+1)
	err := event.Validate()
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "payload exceeds") {
		t.Fatalf("a %d byte payload: got %v, want the size refusal", maxPayloadBytes+1, err)
	}
}

func TestMaskerRedactsKnownValuesFromSixteenBytesAndLeavesShorterOnes(t *testing.T) {
	fifteen, sixteen := strings.Repeat("k", 15), strings.Repeat("s", 16)
	masker := &Masker{Known: []string{fifteen, sixteen}}

	got := masker.MaskString("a " + fifteen + " b " + sixteen + " c")

	want := "a " + fifteen + " b " + Placeholder + " c"
	if got != want {
		t.Fatalf("MaskString = %q, want %q", got, want)
	}
}
