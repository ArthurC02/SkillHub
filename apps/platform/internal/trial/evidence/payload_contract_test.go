package trace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

type schemaProperty struct {
	Type  json.RawMessage  `json:"type"`
	Enum  []any            `json:"enum"`
	Ref   string           `json:"$ref"`
	OneOf []schemaProperty `json:"oneOf"`
	AnyOf []schemaProperty `json:"anyOf"`
}

type schemaObject struct {
	AdditionalProperties *bool                     `json:"additionalProperties"`
	Required             []string                  `json:"required"`
	Properties           map[string]schemaProperty `json:"properties"`
}

type traceEventSchema struct {
	Defs  map[string]json.RawMessage `json:"$defs"`
	AllOf []struct {
		If struct {
			Properties struct {
				Type struct {
					Const string `json:"const"`
				} `json:"type"`
			} `json:"properties"`
		} `json:"if"`
		Then struct {
			Properties struct {
				Payload struct {
					Ref string `json:"$ref"`
				} `json:"payload"`
			} `json:"properties"`
		} `json:"then"`
	} `json:"allOf"`
}

func loadTraceEventSchema(t *testing.T) traceEventSchema {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "..", "contracts", "events", "trace-event.schema.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the contract this package enforces is unreadable: %v", err)
	}
	var schema traceEventSchema
	if err := json.Unmarshal(body, &schema); err != nil {
		t.Fatal(err)
	}
	if len(schema.AllOf) == 0 {
		t.Fatal("the contract declares no per-type payload at all; every comparison below would be vacuous")
	}
	return schema
}

func (s traceEventSchema) resolve(t *testing.T, ref string) schemaObject {
	t.Helper()
	raw, ok := s.Defs[filepath.Base(ref)]
	if !ok {
		t.Fatalf("the contract refers to %q and does not define it", ref)
	}
	var obj schemaObject
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	return obj
}

func (s traceEventSchema) declaredKinds(t *testing.T, p schemaProperty) []string {
	t.Helper()
	if p.Ref != "" {
		return s.declaredKinds(t, s.property(t, p.Ref))
	}
	var kinds []string
	for _, branch := range append(append([]schemaProperty{}, p.OneOf...), p.AnyOf...) {
		kinds = append(kinds, s.declaredKinds(t, branch)...)
	}
	if len(p.Type) > 0 {
		var one string
		if err := json.Unmarshal(p.Type, &one); err == nil {
			kinds = append(kinds, one)
		} else {
			var many []string
			if err := json.Unmarshal(p.Type, &many); err != nil {
				t.Fatal(err)
			}
			kinds = append(kinds, many...)
		}
	}
	slices.Sort(kinds)
	return slices.Compact(kinds)
}

func (s traceEventSchema) property(t *testing.T, ref string) schemaProperty {
	t.Helper()
	raw, ok := s.Defs[filepath.Base(ref)]
	if !ok {
		t.Fatalf("the contract refers to %q and does not define it", ref)
	}
	var p schemaProperty
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func (s traceEventSchema) declaredEnum(t *testing.T, p schemaProperty) []string {
	t.Helper()
	if p.Ref != "" {
		p = s.property(t, p.Ref)
	}
	var named []string
	for _, v := range p.Enum {
		if text, ok := v.(string); ok {
			named = append(named, text)
		}
	}
	slices.Sort(named)
	return named
}

func TestEveryTypeTheContractGivesAPayloadShapeIsEnforcedHere(t *testing.T) {
	schema := loadTraceEventSchema(t)
	for _, branch := range schema.AllOf {
		eventType := branch.If.Properties.Type.Const
		if branch.Then.Properties.Payload.Ref == "" {
			continue
		}
		if _, enforced := payloadRules[eventType]; !enforced {
			t.Errorf("the contract gives %q a payload shape that nothing here checks; a reader "+
				"gets an empty string instead of a refusal", eventType)
		}
	}
}

func TestThisPackageInventsNoRuleTheContractDoesNotState(t *testing.T) {
	schema := loadTraceEventSchema(t)
	stated := map[string]bool{}
	for _, branch := range schema.AllOf {
		if branch.Then.Properties.Payload.Ref != "" {
			stated[branch.If.Properties.Type.Const] = true
		}
	}
	for eventType := range payloadRules {
		if !stated[eventType] {
			t.Errorf("this package refuses payloads for %q on a rule the contract never states", eventType)
		}
	}
}

func TestEveryFieldRuleIsTheOneTheContractStates(t *testing.T) {
	schema := loadTraceEventSchema(t)
	for _, branch := range schema.AllOf {
		eventType := branch.If.Properties.Type.Const
		ref := branch.Then.Properties.Payload.Ref
		if ref == "" {
			continue
		}
		rule, enforced := payloadRules[eventType]
		if !enforced {
			continue
		}
		object := schema.resolve(t, ref)

		open := object.AdditionalProperties == nil || *object.AdditionalProperties
		if rule.open != open {
			t.Errorf("%s: this package treats the payload as open=%v, the contract says open=%v",
				eventType, rule.open, open)
		}

		for name, declared := range object.Properties {
			spec, checked := rule.fields[name]
			if !checked {
				t.Errorf("%s.%s is in the contract and in no rule here", eventType, name)
				continue
			}
			if want := slices.Contains(object.Required, name); spec.required != want {
				t.Errorf("%s.%s required here=%v, in the contract=%v", eventType, name, spec.required, want)
			}
			if want := schema.declaredKinds(t, declared); len(want) > 0 && !slices.Equal(spec.kinds, want) {
				t.Errorf("%s.%s accepts %v here, %v in the contract", eventType, name, spec.kinds, want)
			}
			if want := schema.declaredEnum(t, declared); !slices.Equal(spec.enum, want) {
				t.Errorf("%s.%s allows %v here, %v in the contract", eventType, name, spec.enum, want)
			}
		}
		for name := range rule.fields {
			if _, declared := object.Properties[name]; !declared {
				t.Errorf("%s.%s is checked here and the contract does not declare it", eventType, name)
			}
		}
	}
}

func eventWithPayload(eventType, payload string) *Event {
	return &Event{
		SchemaVersion: "1.0",
		EventID:       "9c0d1e2f-3a4b-4c5d-8e6f-7a8b9c0d1e10",
		RunID:         "9b1d4f2e-77c3-4a2b-8f10-3c9e5a6b7d20",
		Attempt:       1,
		Seq:           2,
		OccurredAt:    time.Now().UTC(),
		EmittedBy:     SourceSandbox,
		Type:          eventType,
		Payload:       json.RawMessage(payload),
	}
}

func TestAPayloadTheReaderWouldMisreadIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name    string
		kind    string
		payload string
		want    string
	}{
		{"a required field left out", "script_log",
			`{"stream":"stdout","truncated":false}`, `requires "message"`},
		{"a number where the reader expects text", "script_log",
			`{"stream":"stdout","message":17,"truncated":false}`, "script_log.message is integer"},
		{"an object where the reader expects a number", "tool_call",
			`{"tool_name":"bash","outcome":"succeeded","duration_ms":{"ms":3}}`, "tool_call.duration_ms is object"},
		{"a value outside the closed set", "agent_output",
			`{"kind":"whispered","text":"x","truncated":false}`, `agent_output.kind is "whispered"`},
		{"a field this type does not have", "agent_output",
			`{"kind":"final","text":"x","truncated":false,"invented":1}`, `carries no field "invented"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := eventWithPayload(tc.kind, tc.payload).Validate()
			if err == nil {
				t.Fatalf("%s was accepted; the reader turns it into an empty string, not an error", tc.name)
			}
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("err = %v, want it to be ErrInvalid so ingestion counts it as rejected", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %q, want it to name %q", err, tc.want)
			}
		})
	}
}

func TestThePayloadsTheProducersActuallySendAreAccepted(t *testing.T) {
	for _, tc := range []struct{ kind, payload string }{
		{"skill_activation", `{"skill_version_id":"4d5e6f7a-8b9c-4d1e-9f2a-3b4c5d6e7f81","skill_name":"x","decision":"activated","reason":null}`},
		{"resource_read", `{"resource_path":"a.md","outcome":"read","bytes_read":12,"truncated":false}`},
		{"script_log", `{"script_path":null,"stream":"stderr","message":"x","truncated":false,"dropped_bytes":null}`},
		{"tool_call", `{"tool_name":"bash","invocation_id":null,"arguments":null,"result_summary":null,"outcome":"timed_out","duration_ms":9,"truncated":true}`},
		{"usage", `{"scope":"run_total","model":"m","input_tokens":1,"output_tokens":2,"cache_read_input_tokens":null,"cost_usd":0.5,"cost_source":"gateway","token_source":"result","duration_ms":3}`},
		{"agent_output", `{"kind":"captured","text":"x","truncated":false}`},
		{"error", `{"category":"execution","code":"c","message":"m","retryable":false}`},
		{"mcp_call", `{"server":"s","tool_name":"t","whatever_the_placeholder_carries":1}`},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			if err := eventWithPayload(tc.kind, tc.payload).Validate(); err != nil {
				t.Errorf("a payload this system really sends was refused: %v", err)
			}
		})
	}
}
