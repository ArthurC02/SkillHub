package apiserver_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestTraceRejectsFractionalCountsWithoutDroppingWholeNeighbours(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "trace-numeric-owner")
	runID := seedRun(t, pool, owner.workspaceID, seedSkill(t, pool, owner.workspaceID, "trace-numeric-skill"))
	bad := event(runID, 1, 1, "tool_call", `{"tool_name":"bad","outcome":"succeeded","duration_ms":1.0000000000000001}`)
	good := event(runID, 1, 2, "tool_call", `{"tool_name":"whole","outcome":"succeeded","duration_ms":100.0e-1}`)
	for attempt := 0; attempt < 2; attempt++ {
		code, report := a.ingest(t, runID, 1, bad, good)
		if code != http.StatusAccepted || report.Rejected != 1 || report.Stored != 1-attempt || report.Duplicate != attempt {
			t.Fatalf("fractional count batch %d: code=%d report=%+v", attempt, code, report)
		}
	}
	code, view := owner.advancedTrace(t, runID)
	if code != http.StatusOK || len(view.Events) != 1 || view.Events[0].Seq != 2 {
		t.Fatalf("fractional count survived or whole neighbour was lost: code=%d view=%+v", code, view)
	}
	var payload struct {
		ToolName string `json:"tool_name"`
		Duration int    `json:"duration_ms"`
	}
	if err := json.Unmarshal(view.Events[0].Payload, &payload); err != nil || payload.ToolName != "whole" || payload.Duration != 10 {
		t.Fatalf("whole exponent was not retained: payload=%s err=%v", view.Events[0].Payload, err)
	}
	code, summary := owner.generalTrace(t, runID)
	if code != http.StatusOK || summary.ToolCalls.Total != 1 || summary.ToolCalls.TotalMS != 10 {
		t.Fatalf("whole duration was misreported: code=%d calls=%+v", code, summary.ToolCalls)
	}
}
