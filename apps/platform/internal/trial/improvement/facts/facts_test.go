package facts

import (
	"reflect"
	"testing"

	trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

func aView(events ...trace.EventView) trace.AdvancedView {
	return trace.AdvancedView{Complete: true, Events: events}
}

func toolCallEvent(payload string) trace.EventView {
	return trace.EventView{Type: trace.TypeToolCall, Payload: []byte(payload)}
}

func TestToolCallsKeepTheRunsOrderAndOnlyToolCallEvents(t *testing.T) {
	m := Material{Advanced: aView(
		toolCallEvent(`{"tool_name":"Bash","outcome":"failed","duration_ms":40,"arguments":{"command":"python scripts/rate.py"}}`),
		trace.EventView{Type: trace.TypeResourceRead, Payload: []byte(`{"resource_path":"SKILL.md","outcome":"read"}`)},
		toolCallEvent(`{"tool_name":"Read","outcome":"succeeded","duration_ms":3,"arguments":null}`),
	)}

	got := m.ToolCalls()

	want := ToolCallLog{Complete: true, Calls: []ToolCall{
		{Name: "Bash", Outcome: "failed", DurationMS: 40, Arguments: `{"command":"python scripts/rate.py"}`},
		{Name: "Read", Outcome: "succeeded", DurationMS: 3},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tool calls = %+v, want %+v", got, want)
	}
}

func TestToolCallsAreIncompleteWheneverTheTraceCannotShowEveryCall(t *testing.T) {
	for name, cut := range map[string]func(*trace.AdvancedView){
		"a trace with known gaps":        func(v *trace.AdvancedView) { v.Complete = false },
		"a trace with more pages":        func(v *trace.AdvancedView) { v.HasMore = true },
		"a trace cut for the evaluation": func(v *trace.AdvancedView) { v.EvaluationTruncated = true },
		"a call whose payload cannot be read": func(v *trace.AdvancedView) {
			v.Events = append(v.Events, toolCallEvent(`{"tool_name":`))
		},
	} {
		t.Run(name, func(t *testing.T) {
			view := aView(toolCallEvent(`{"tool_name":"Bash","outcome":"succeeded","duration_ms":1}`))
			cut(&view)
			got := Material{Advanced: view}.ToolCalls()
			if got.Complete {
				t.Fatal("complete = true, want false")
			}
			if len(got.Calls) != 1 || got.Calls[0].Name != "Bash" {
				t.Fatalf("calls = %+v, want the one readable Bash call kept", got.Calls)
			}
		})
	}
}
