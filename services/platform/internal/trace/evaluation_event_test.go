package trace

// The 1.2 half of contracts/events/trace-event.schema.json: two new types, one
// producer, and a version declaration that follows the type rather than a single
// global constant (contract README §9).

import (
	"encoding/json"
	"testing"
	"time"
)

func evaluationEvent(eventType, emittedBy string) *Event {
	return &Event{
		SchemaVersion: SchemaVersionEvaluation,
		EventID:       "9c0d1e2f-3a4b-4c5d-8e6f-7a8b9c0d1e10",
		RunID:         "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20",
		Attempt:       1,
		Seq:           2,
		OccurredAt:    time.Now().UTC(),
		EmittedBy:     emittedBy,
		Type:          eventType,
		Masked:        true,
		Payload:       json.RawMessage(`{"evaluation_id":"ad1e2f3a-4b5c-4d6e-9f7a-8b9c0d1e2f11"}`),
	}
}

func TestEvaluationEventsAreAcceptedFromTheOrchestrator(t *testing.T) {
	for _, eventType := range []string{TypeEvaluationStarted, TypeEvaluationCompleted} {
		if err := evaluationEvent(eventType, SourceOrchestr).Validate(); err != nil {
			t.Errorf("%s from the orchestrator should validate: %v", eventType, err)
		}
	}
}

// The schema pins both types to `orchestrator`. Evaluation happens in the control
// plane after the sandbox is gone, so one arriving from the execution plane is a
// forged verdict rather than a late event (README §4.1).
func TestAnEvaluationEventFromTheSandboxIsRefused(t *testing.T) {
	for _, source := range []string{SourceSandbox, SourceLLMService} {
		for _, eventType := range []string{TypeEvaluationStarted, TypeEvaluationCompleted} {
			if err := evaluationEvent(eventType, source).Validate(); err == nil {
				t.Errorf("%s from %s must be refused: only the orchestrator evaluates", eventType, source)
			}
		}
	}
}

// A producer declares the contract revision it wrote to. The evaluation types do
// not exist in 1.0, and `error` / `run_lifecycle` were never written against 1.2.
func TestDeclaredVersionFollowsTheEventType(t *testing.T) {
	cases := map[string]string{
		TypeEvaluationStarted:   SchemaVersionEvaluation,
		TypeEvaluationCompleted: SchemaVersionEvaluation,
		TypeError:               SchemaVersion,
		TypeRunLifecycle:        SchemaVersion,
	}
	for eventType, want := range cases {
		if got := schemaVersionFor(eventType); got != want {
			t.Errorf("schemaVersionFor(%q) = %q, want %q", eventType, got, want)
		}
	}
	if SchemaVersionEvaluation != "1.2" {
		t.Errorf("the evaluation types landed in 1.2, got %q", SchemaVersionEvaluation)
	}
}
