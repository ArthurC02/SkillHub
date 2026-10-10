package trace

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const (
	kindNull    = "null"
	kindObject  = "object"
	kindArray   = "array"
	kindString  = "string"
	kindBoolean = "boolean"
	kindNumber  = "number"
	kindInteger = "integer"
)

const (
	PayloadEvaluationID = "evaluation_id"
	payloadTruncated    = "truncated"
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
		"decision":         {required: true, kinds: []string{kindString}, enum: []string{"activated", "skipped"}},
		"reason":           {kinds: []string{kindNull, kindString}},
		"skill_id":         {kinds: []string{kindString}},
		"skill_name":       {kinds: []string{kindString}},
		"skill_version_id": {required: true, kinds: []string{kindString}},
	}},
	"resource_read": {open: false, fields: map[string]payloadField{
		"bytes_read":       {kinds: []string{kindInteger, kindNull}},
		"outcome":          {required: true, kinds: []string{kindString}, enum: []string{"denied", "not_found", "read"}},
		"resource_path":    {required: true, kinds: []string{kindString}},
		"skill_version_id": {kinds: []string{kindString}},
		payloadTruncated:   {kinds: []string{kindBoolean}},
	}},
	"tool_call": {open: false, fields: map[string]payloadField{
		"arguments":      {kinds: []string{kindNull, kindObject}},
		"duration_ms":    {required: true, kinds: []string{kindInteger}},
		"invocation_id":  {kinds: []string{kindNull, kindString}},
		"outcome":        {required: true, kinds: []string{kindString}, enum: []string{"denied", "failed", "succeeded", outcomeTimedOut}},
		"result_summary": {kinds: []string{kindNull, kindString}},
		"tool_name":      {required: true, kinds: []string{kindString}},
		payloadTruncated: {kinds: []string{kindBoolean}},
	}},
	"mcp_call": {open: true, fields: map[string]payloadField{
		"server":    {kinds: []string{kindString}},
		"tool_name": {kinds: []string{kindString}},
	}},
	"script_log": {open: false, fields: map[string]payloadField{
		"dropped_bytes":  {kinds: []string{kindInteger, kindNull}},
		"message":        {required: true, kinds: []string{kindString}},
		"script_path":    {kinds: []string{kindNull, kindString}},
		"stream":         {required: true, kinds: []string{kindString}, enum: []string{"stderr", "stdout"}},
		payloadTruncated: {required: true, kinds: []string{kindBoolean}},
	}},
	"agent_output": {open: false, fields: map[string]payloadField{
		"kind":           {required: true, kinds: []string{kindString}, enum: []string{"captured", "final", "intermediate", "question"}},
		"questions":      {kinds: []string{kindArray}},
		"text":           {required: true, kinds: []string{kindString}},
		payloadTruncated: {required: true, kinds: []string{kindBoolean}},
	}},
	"error": {open: false, fields: map[string]payloadField{
		"category":             {required: true, kinds: []string{kindString}, enum: []string{"artifact_upload", "cleanup", "evaluation", "event_delivery", "execution", "provision"}},
		"code":                 {required: true, kinds: []string{kindString}},
		"message":              {required: true, kinds: []string{kindString}},
		"provider_diagnostics": {kinds: []string{kindNull, kindObject}},
		"retryable":            {required: true, kinds: []string{kindBoolean}},
	}},
	"usage": {open: false, fields: map[string]payloadField{
		"cache_read_input_tokens":  {kinds: []string{kindInteger, kindNull}},
		"cache_write_input_tokens": {kinds: []string{kindInteger, kindNull}},
		"cost_source":              {kinds: []string{kindNull, kindString}, enum: []string{"estimated", "gateway"}},
		"cost_usd":                 {kinds: []string{kindNull, kindNumber}},
		"duration_ms":              {kinds: []string{kindInteger, kindNull}},
		"input_tokens":             {required: true, kinds: []string{kindInteger}},
		"model":                    {required: true, kinds: []string{kindString}},
		"output_tokens":            {required: true, kinds: []string{kindInteger}},
		"scope":                    {kinds: []string{kindString}, enum: []string{"call", "run_total"}},
		"token_source":             {kinds: []string{kindNull, kindString}, enum: []string{"accumulated", "result"}},
	}},
	"run_lifecycle": {open: false, fields: map[string]payloadField{
		"from_status": {kinds: []string{kindNull, kindString}},
		"reason":      {kinds: []string{kindNull, kindString}},
		"to_status":   {required: true, kinds: []string{kindString}, enum: []string{"cancelled", "evaluating", "failed", "preparing", "provisioning", "queued", "running", "succeeded", "timed_out"}},
	}},
	"evaluation_started": {open: false, fields: map[string]payloadField{
		PayloadEvaluationID:    {required: true, kinds: []string{kindString}},
		"judge_model":          {required: true, kinds: []string{kindString}},
		"judge_prompt_version": {required: true, kinds: []string{kindString}},
		"rubric_version":       {kinds: []string{kindNull, kindString}},
	}},
	"evaluation_completed": {open: false, fields: map[string]payloadField{
		"cost_usd":              {kinds: []string{kindNull, kindNumber}},
		"criteria_failed":       {required: true, kinds: []string{kindInteger}},
		"criteria_passed":       {required: true, kinds: []string{kindInteger}},
		"criteria_total":        {required: true, kinds: []string{kindInteger}},
		"criteria_undetermined": {required: true, kinds: []string{kindInteger}},
		PayloadEvaluationID:     {required: true, kinds: []string{kindString}},
		"evidence_complete":     {required: true, kinds: []string{kindBoolean}},
		"failure_reason":        {kinds: []string{kindNull, kindString}},
		"overall":               {required: true, kinds: []string{kindString}, enum: []string{"met", "not_met", "partially_met", "undetermined"}},
	}},
}

func jsonKind(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return kindNull
	}
	switch trimmed[0] {
	case '{':
		return kindObject
	case '[':
		return kindArray
	case '"':
		return kindString
	case 't', 'f':
		return kindBoolean
	case 'n':
		return kindNull
	}
	if bytes.ContainsAny(trimmed, ".eE") {
		return kindNumber
	}
	return kindInteger
}

func numeric(kind string) bool { return kind == kindInteger || kind == kindNumber }

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

func validatePayloadField(eventType, name string, spec payloadField, value json.RawMessage) error {
	kind := jsonKind(value)
	if len(spec.kinds) > 0 && !kindAccepted(kind, spec.kinds) {
		return fmt.Errorf("%w: %s.%s is %s, want %v", ErrInvalid, eventType, name, kind, spec.kinds)
	}
	if len(spec.enum) == 0 || kind != kindString {
		return nil
	}
	var got string
	if err := json.Unmarshal(value, &got); err != nil {
		return fmt.Errorf("%w: %s.%s is not readable", ErrInvalid, eventType, name)
	}
	if !valueAccepted(got, spec.enum) {
		return fmt.Errorf("%w: %s.%s is %q, want one of %v", ErrInvalid, eventType, name, got, spec.enum)
	}
	return nil
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
		if err := validatePayloadField(eventType, name, spec, value); err != nil {
			return err
		}
	}
	for name, spec := range rule.fields {
		if spec.required && fields[name] == nil {
			return fmt.Errorf("%w: %s requires %q", ErrInvalid, eventType, name)
		}
	}
	return nil
}
