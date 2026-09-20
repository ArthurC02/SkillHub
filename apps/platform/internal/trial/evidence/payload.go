package trace

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type payloadField struct {
	required bool
	kinds    []string
	enum     []string
}

type payloadRule struct {
	open   bool
	fields map[string]payloadField
}

var payloadRules = map[string]payloadRule{
	"skill_activation": {open: false, fields: map[string]payloadField{
		"decision":         {required: true, kinds: []string{"string"}, enum: []string{"activated", "skipped"}},
		"reason":           {kinds: []string{"null", "string"}},
		"skill_id":         {kinds: []string{"string"}},
		"skill_name":       {kinds: []string{"string"}},
		"skill_version_id": {required: true, kinds: []string{"string"}},
	}},
	"resource_read": {open: false, fields: map[string]payloadField{
		"bytes_read":       {kinds: []string{"integer", "null"}},
		"outcome":          {required: true, kinds: []string{"string"}, enum: []string{"denied", "not_found", "read"}},
		"resource_path":    {required: true, kinds: []string{"string"}},
		"skill_version_id": {kinds: []string{"string"}},
		"truncated":        {kinds: []string{"boolean"}},
	}},
	"tool_call": {open: false, fields: map[string]payloadField{
		"arguments":      {kinds: []string{"null", "object"}},
		"duration_ms":    {required: true, kinds: []string{"integer"}},
		"invocation_id":  {kinds: []string{"null", "string"}},
		"outcome":        {required: true, kinds: []string{"string"}, enum: []string{"denied", "failed", "succeeded", "timed_out"}},
		"result_summary": {kinds: []string{"null", "string"}},
		"tool_name":      {required: true, kinds: []string{"string"}},
		"truncated":      {kinds: []string{"boolean"}},
	}},
	"mcp_call": {open: true, fields: map[string]payloadField{
		"server":    {kinds: []string{"string"}},
		"tool_name": {kinds: []string{"string"}},
	}},
	"script_log": {open: false, fields: map[string]payloadField{
		"dropped_bytes": {kinds: []string{"integer", "null"}},
		"message":       {required: true, kinds: []string{"string"}},
		"script_path":   {kinds: []string{"null", "string"}},
		"stream":        {required: true, kinds: []string{"string"}, enum: []string{"stderr", "stdout"}},
		"truncated":     {required: true, kinds: []string{"boolean"}},
	}},
	"agent_output": {open: false, fields: map[string]payloadField{
		"kind":      {required: true, kinds: []string{"string"}, enum: []string{"captured", "final", "intermediate"}},
		"text":      {required: true, kinds: []string{"string"}},
		"truncated": {required: true, kinds: []string{"boolean"}},
	}},
	"error": {open: false, fields: map[string]payloadField{
		"category":             {required: true, kinds: []string{"string"}, enum: []string{"artifact_upload", "cleanup", "evaluation", "event_delivery", "execution", "provision"}},
		"code":                 {required: true, kinds: []string{"string"}},
		"message":              {required: true, kinds: []string{"string"}},
		"provider_diagnostics": {kinds: []string{"null", "object"}},
		"retryable":            {required: true, kinds: []string{"boolean"}},
	}},
	"usage": {open: false, fields: map[string]payloadField{
		"cache_read_input_tokens":  {kinds: []string{"integer", "null"}},
		"cache_write_input_tokens": {kinds: []string{"integer", "null"}},
		"cost_source":              {kinds: []string{"null", "string"}, enum: []string{"estimated", "gateway"}},
		"cost_usd":                 {kinds: []string{"null", "number"}},
		"duration_ms":              {kinds: []string{"integer", "null"}},
		"input_tokens":             {required: true, kinds: []string{"integer"}},
		"model":                    {required: true, kinds: []string{"string"}},
		"output_tokens":            {required: true, kinds: []string{"integer"}},
		"scope":                    {kinds: []string{"string"}, enum: []string{"call", "run_total"}},
		"token_source":             {kinds: []string{"null", "string"}, enum: []string{"accumulated", "result"}},
	}},
	"run_lifecycle": {open: false, fields: map[string]payloadField{
		"from_status": {kinds: []string{"null", "string"}},
		"reason":      {kinds: []string{"null", "string"}},
		"to_status":   {required: true, kinds: []string{"string"}, enum: []string{"cancelled", "evaluating", "failed", "preparing", "provisioning", "queued", "running", "succeeded", "timed_out"}},
	}},
	"evaluation_started": {open: false, fields: map[string]payloadField{
		"evaluation_id":        {required: true, kinds: []string{"string"}},
		"judge_model":          {required: true, kinds: []string{"string"}},
		"judge_prompt_version": {required: true, kinds: []string{"string"}},
		"rubric_version":       {kinds: []string{"null", "string"}},
	}},
	"evaluation_completed": {open: false, fields: map[string]payloadField{
		"cost_usd":              {kinds: []string{"null", "number"}},
		"criteria_failed":       {required: true, kinds: []string{"integer"}},
		"criteria_passed":       {required: true, kinds: []string{"integer"}},
		"criteria_total":        {required: true, kinds: []string{"integer"}},
		"criteria_undetermined": {required: true, kinds: []string{"integer"}},
		"evaluation_id":         {required: true, kinds: []string{"string"}},
		"evidence_complete":     {required: true, kinds: []string{"boolean"}},
		"failure_reason":        {kinds: []string{"null", "string"}},
		"overall":               {required: true, kinds: []string{"string"}, enum: []string{"met", "not_met", "partially_met", "undetermined"}},
	}},
}

func jsonKind(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "null"
	}
	switch trimmed[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	}
	if bytes.ContainsAny(trimmed, ".eE") {
		return "number"
	}
	return "integer"
}

func numeric(kind string) bool { return kind == "integer" || kind == "number" }

func kindAccepted(kind string, accepted []string) bool {
	for _, want := range accepted {
		if kind == want || (numeric(kind) && numeric(want)) {
			return true
		}
	}
	return false
}

func valueAccepted(got string, accepted []string) bool {
	for _, want := range accepted {
		if got == want {
			return true
		}
	}
	return false
}

func validatePayload(eventType string, raw json.RawMessage) error {
	rule, known := payloadRules[eventType]
	if !known {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return fmt.Errorf("%w: payload is not a JSON object", ErrInvalid)
	}
	for name, value := range fields {
		spec, declared := rule.fields[name]
		if !declared {
			if rule.open {
				continue
			}
			return fmt.Errorf("%w: %s carries no field %q", ErrInvalid, eventType, name)
		}
		kind := jsonKind(value)
		if len(spec.kinds) > 0 && !kindAccepted(kind, spec.kinds) {
			return fmt.Errorf("%w: %s.%s is %s, want %v", ErrInvalid, eventType, name, kind, spec.kinds)
		}
		if len(spec.enum) > 0 && kind == "string" {
			var got string
			if err := json.Unmarshal(value, &got); err != nil {
				return fmt.Errorf("%w: %s.%s is not readable", ErrInvalid, eventType, name)
			}
			if !valueAccepted(got, spec.enum) {
				return fmt.Errorf("%w: %s.%s is %q, want one of %v", ErrInvalid, eventType, name, got, spec.enum)
			}
		}
	}
	for name, spec := range rule.fields {
		if spec.required && fields[name] == nil {
			return fmt.Errorf("%w: %s requires %q", ErrInvalid, eventType, name)
		}
	}
	return nil
}
