package trace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const SchemaVersion = "1.0"

const SchemaVersionEvaluation = "1.2"

func schemaVersionFor(eventType string) string {
	switch eventType {
	case TypeEvaluationStarted, TypeEvaluationCompleted:
		return SchemaVersionEvaluation
	default:
		return SchemaVersion
	}
}

const (
	SourceSandbox    = "sandbox"
	SourceOrchestr   = "orchestrator"
	SourceLLMService = "llm_service"
)

const (
	TypeSkillActivation = "skill_activation"
	TypeResourceRead    = "resource_read"
	TypeToolCall        = "tool_call"
	TypeMCPCall         = "mcp_call"
	TypeScriptLog       = "script_log"
	TypeAgentOutput     = "agent_output"
	TypeError           = "error"
	TypeUsage           = "usage"
	TypeRunLifecycle    = "run_lifecycle"

	TypeEvaluationStarted   = "evaluation_started"
	TypeEvaluationCompleted = "evaluation_completed"
)

var eventTypes = map[string]bool{
	TypeSkillActivation: true, TypeResourceRead: true, TypeToolCall: true,
	TypeMCPCall: true, TypeScriptLog: true, TypeAgentOutput: true,
	TypeError: true, TypeUsage: true, TypeRunLifecycle: true,
	TypeEvaluationStarted: true, TypeEvaluationCompleted: true,
}

var sources = map[string]bool{SourceSandbox: true, SourceOrchestr: true, SourceLLMService: true}

var statuses = map[string]bool{"ok": true, "error": true, "skipped": true, "cancelled": true, "timed_out": true}

type Event struct {
	SchemaVersion string          `json:"schema_version"`
	EventID       string          `json:"event_id"`
	RunID         string          `json:"run_id"`
	Attempt       int             `json:"attempt"`
	Seq           int64           `json:"seq"`
	OccurredAt    time.Time       `json:"occurred_at"`
	EmittedBy     string          `json:"emitted_by"`
	Type          string          `json:"type"`
	Status        *string         `json:"status,omitempty"`
	Masked        bool            `json:"masked"`
	MaskedFields  []string        `json:"masked_fields"`
	Payload       json.RawMessage `json:"payload"`

	Late bool `json:"late,omitempty"`
}

var ErrInvalid = errors.New("invalid trace event")

const (
	maxPayloadBytes = 96 << 10
	maxTraceAttempt = 100_000

	maxTraceSeq = 100_000
)

func (e *Event) Validate() error {
	switch {
	case e.SchemaVersion == "":
		return fmt.Errorf("%w: schema_version is required", ErrInvalid)
	case !compatibleVersion(e.SchemaVersion):
		return fmt.Errorf("%w: schema_version %q is not a 1.x version", ErrInvalid, e.SchemaVersion)
	case e.EventID == "":
		return fmt.Errorf("%w: event_id is required", ErrInvalid)
	case e.Attempt < 1:
		return fmt.Errorf("%w: attempt must be 1 or greater", ErrInvalid)
	case e.Attempt > maxTraceAttempt:
		return fmt.Errorf("%w: attempt must not exceed %d", ErrInvalid, maxTraceAttempt)
	case e.Seq < 1:
		return fmt.Errorf("%w: seq must be 1 or greater", ErrInvalid)
	case e.Seq > maxTraceSeq:
		return fmt.Errorf("%w: seq must not exceed %d", ErrInvalid, maxTraceSeq)
	case e.OccurredAt.IsZero():
		return fmt.Errorf("%w: occurred_at is required", ErrInvalid)
	case !sources[e.EmittedBy]:
		return fmt.Errorf("%w: emitted_by %q is not a known producer", ErrInvalid, e.EmittedBy)
	case !eventTypes[e.Type]:

		return fmt.Errorf("%w: type %q is not in this schema version", ErrInvalid, e.Type)
	case (e.Type == TypeEvaluationStarted || e.Type == TypeEvaluationCompleted) && e.EmittedBy != SourceOrchestr:

		return fmt.Errorf("%w: %s may only be emitted by the orchestrator", ErrInvalid, e.Type)
	case e.Status != nil && !statuses[*e.Status]:
		return fmt.Errorf("%w: status %q is not a known outcome", ErrInvalid, *e.Status)
	case len(e.Payload) == 0:
		return fmt.Errorf("%w: payload is required", ErrInvalid)
	case len(e.Payload) > maxPayloadBytes:
		return fmt.Errorf("%w: payload exceeds %d bytes", ErrInvalid, maxPayloadBytes)
	case len(bytes.TrimSpace(e.Payload)) == 0 || bytes.TrimSpace(e.Payload)[0] != '{':
		return fmt.Errorf("%w: payload must be a JSON object", ErrInvalid)
	}
	return nil
}

func compatibleVersion(v string) bool {
	return len(v) > 2 && v[0] == '1' && v[1] == '.'
}
