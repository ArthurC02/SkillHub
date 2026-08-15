package trace

// The Go side of contracts/events/trace-event.schema.json. Hand-written against
// that file, like internal/run/provider.go is against the sandbox contract: no
// codegen is wired for contracts/ yet, and the validator that keeps the two
// honest is tools/contracts/validate_trace_events.py plus the golden sample this
// package writes for it.

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// SchemaVersion is the contract version this code writes and understands. Minor
// bumps are additive, so an event arriving with a higher 1.x is accepted; a
// different major is refused rather than guessed at.
const SchemaVersion = "1.0"

// Producers (schema `emitted_by`).
const (
	SourceSandbox    = "sandbox"
	SourceOrchestr   = "orchestrator"
	SourceLLMService = "llm_service"
)

// Event types (schema `type`).
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
)

var eventTypes = map[string]bool{
	TypeSkillActivation: true, TypeResourceRead: true, TypeToolCall: true,
	TypeMCPCall: true, TypeScriptLog: true, TypeAgentOutput: true,
	TypeError: true, TypeUsage: true, TypeRunLifecycle: true,
}

var sources = map[string]bool{SourceSandbox: true, SourceOrchestr: true, SourceLLMService: true}

var statuses = map[string]bool{"ok": true, "error": true, "skipped": true, "cancelled": true, "timed_out": true}

// Event is one trace event envelope. Payload stays raw: the platform stores it
// as jsonb and the per-type payload shapes are the schema's business, not
// something the control plane needs Go structs for.
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
	// Late is platform-assigned, not producer-assigned: it means "arrived after
	// this run had already finished". It is not part of the wire contract, so it
	// is only ever written on the way out.
	Late bool `json:"late,omitempty"`
}

// ErrInvalid is any event the ingestion endpoint refuses. Refusal is per event,
// not per batch: one malformed event from an untrusted sandbox must not discard
// the well-formed ones next to it.
var ErrInvalid = errors.New("invalid trace event")

// maxPayloadBytes bounds one event's payload after masking. The schema's own
// maxLength values (16 KB for a script_log message, 64 KB for agent text) are
// the producer's obligation; this is the control plane's independent ceiling,
// because a producer inside the trust boundary is not the one to trust with it.
const maxPayloadBytes = 96 << 10

// Validate checks an event from an untrusted producer against the contract, as
// far as the envelope goes. Payload shape is not checked here: the per-type
// schemas are enforced by the producer and by the contract test, and rejecting
// on payload shape at the boundary would let one schema change silently start
// dropping a whole run's trace.
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
	case e.Seq < 1:
		return fmt.Errorf("%w: seq must be 1 or greater", ErrInvalid)
	case e.OccurredAt.IsZero():
		return fmt.Errorf("%w: occurred_at is required", ErrInvalid)
	case !sources[e.EmittedBy]:
		return fmt.Errorf("%w: emitted_by %q is not a known producer", ErrInvalid, e.EmittedBy)
	case !eventTypes[e.Type]:
		// An unknown type is dropped rather than stored: the schema's own rule is
		// that a consumer ignores types it does not know (README §9), and storing
		// one would put an unrenderable row in a user-visible timeline.
		return fmt.Errorf("%w: type %q is not in this schema version", ErrInvalid, e.Type)
	case e.Status != nil && !statuses[*e.Status]:
		return fmt.Errorf("%w: status %q is not a known outcome", ErrInvalid, *e.Status)
	case len(e.Payload) == 0:
		return fmt.Errorf("%w: payload is required", ErrInvalid)
	case len(e.Payload) > maxPayloadBytes:
		return fmt.Errorf("%w: payload exceeds %d bytes", ErrInvalid, maxPayloadBytes)
	}
	return nil
}

// compatibleVersion accepts any 1.x, because minor bumps are additive by the
// contract's own versioning rule and an older consumer is required to tolerate
// fields it does not know.
func compatibleVersion(v string) bool {
	return len(v) > 2 && v[0] == '1' && v[1] == '.'
}
