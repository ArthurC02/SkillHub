package trace

import (
	"errors"
	"fmt"
	"testing"
)

func TestTraceIntegerFieldsRequireExactWholeNumbers(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		valid       bool
	}{
		{"zero", "0", true},
		{"negative zero", "-0.0", true},
		{"integer", "1", true},
		{"whole decimal", "1.0", true},
		{"shifted decimal", "0.0100e2", true},
		{"whole negative exponent", "100.0e-2", true},
		{"decimal before boundary", "1.1e0", false},
		{"decimal at boundary", "1.1e1", true},
		{"decimal after boundary", "1.1e2", true},
		{"positive exponent", "1e2", true},
		{"capital exponent", "1E+2", true},
		{"zero exponent", "1e-0", true},
		{"fraction", "1.5", false},
		{"negative exponent", "1e-1", false},
		{"precision beyond float64", "1.0000000000000001", false},
		{"negative precision beyond float64", "-1.0000000000000001", false},
		{"large exponent", "1e100", true},
		{"positive exponent overflow", "1e9223372036854775808", true},
		{"negative exponent overflow", "1e-9223372036854775809", false},
		{"zero with negative overflow", "0.000e-9223372036854775809", true},
		{"zero with positive overflow", "-0e9223372036854775808", true},
		{"maximum negative exponent", "1e-9223372036854775808", false},
		{"maximum positive exponent", "1e9223372036854775807", true},
		{"text", `"1"`, false},
		{"null", "null", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := eventWithPayload(TypeToolCall, fmt.Sprintf(`{"tool_name":"bash","outcome":"succeeded","duration_ms":%s}`, tc.value))
			err := event.Validate()
			if tc.valid && err != nil || !tc.valid && !errors.Is(err, ErrInvalid) {
				t.Fatalf("duration_ms=%s: err=%v, want valid=%v", tc.value, err, tc.valid)
			}
		})
	}
}

func TestTraceNumericKindsRemainDistinctAcrossPayloads(t *testing.T) {
	for _, tc := range []struct {
		name, kind, payload string
		valid               bool
	}{
		{"fractional tokens", TypeUsage, `{"model":"m","input_tokens":1.5,"output_tokens":2}`, false},
		{"fractional cached tokens", TypeUsage, `{"model":"m","input_tokens":1,"output_tokens":2,"cache_read_input_tokens":0.1}`, false},
		{"fractional bytes", TypeResourceRead, `{"resource_path":"x","outcome":"read","bytes_read":0.1}`, false},
		{"fractional dropped bytes", TypeScriptLog, `{"stream":"stdout","message":"x","truncated":true,"dropped_bytes":0.1}`, false},
		{"integer cost", TypeUsage, `{"model":"m","input_tokens":1,"output_tokens":2,"cost_usd":1}`, true},
		{"fractional cost", TypeUsage, `{"model":"m","input_tokens":1,"output_tokens":2,"cost_usd":0.001}`, true},
		{"unknown cost", TypeUsage, `{"model":"m","input_tokens":1,"output_tokens":2,"cost_usd":null}`, true},
		{"unknown cached tokens", TypeUsage, `{"model":"m","input_tokens":1,"output_tokens":2,"cache_read_input_tokens":null}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := eventWithPayload(tc.kind, tc.payload).Validate()
			if tc.valid && err != nil || !tc.valid && !errors.Is(err, ErrInvalid) {
				t.Fatalf("payload=%s: err=%v, want valid=%v", tc.payload, err, tc.valid)
			}
		})
	}
}
