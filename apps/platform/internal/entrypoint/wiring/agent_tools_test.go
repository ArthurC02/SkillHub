package wiring

import (
	"encoding/json"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/capacity"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/jobruns"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/operations"
)

func TestTheMaintenanceFactsNameTablesAndJobsSoACiteCanReachThem(t *testing.T) {
	days := 12.5
	facts := NewMaintenanceFacts(
		capacity.Report{DatabaseBytes: 100, DaysUntilBudget: &days, Tables: []capacity.TableSize{{Table: "runs", Bytes: 60}}},
		[]jobruns.Status{{Job: "purge-audit", PeriodSeconds: 604800, OverdueRatio: 2.5}, {Job: "report", PeriodSeconds: 86400}},
	)
	encoded, err := json.Marshal(facts)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Capacity struct {
			DatabaseBytes   int64            `json:"database_bytes"`
			DaysUntilBudget float64          `json:"days_until_budget"`
			TableBytes      map[string]int64 `json:"table_bytes"`
		} `json:"capacity"`
		MaintenanceJobs map[string]struct {
			PeriodSeconds int64   `json:"period_seconds"`
			OverdueRatio  float64 `json:"overdue_ratio"`
			Action        string  `json:"action"`
		} `json:"maintenance_jobs"`
	}
	if err := json.Unmarshal(encoded, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Capacity.DatabaseBytes != 100 || doc.Capacity.DaysUntilBudget != 12.5 || doc.Capacity.TableBytes["runs"] != 60 {
		t.Errorf("capacity facts %+v, want the report's size, forecast and the table by name", doc.Capacity)
	}
	if job := doc.MaintenanceJobs["purge-audit"]; job.PeriodSeconds != 604800 || job.OverdueRatio != 2.5 {
		t.Errorf("job facts %+v, want the job by name with its period and overdue ratio", doc.MaintenanceJobs)
	}
	if proposable, reportOnly := doc.MaintenanceJobs["purge-audit"].Action, doc.MaintenanceJobs["report"].Action; proposable != "run-purge-audit" || reportOnly != "" {
		t.Errorf("actions %q and %q, want only the proposable job to name the action that reruns it", proposable, reportOnly)
	}

	report := `{"items":[{"status":"attention","text":"x","cites":[` +
		`"/capacity/table_bytes/runs","/maintenance_jobs/purge-audit/overdue_ratio","/capacity/days_until_budget"]}]}`
	steps := []operations.StepRecord{{ToolCall: operations.ToolCall{Tool: operations.ToolMaintenanceReport}, Result: string(encoded)}}
	if err := operations.CitesOnlyReturnedFacts(json.RawMessage(report), steps); err != nil {
		t.Errorf("a report citing the tool's own names was rejected: %v", err)
	}
}

func TestARerunIsOfferedOnlyOnceAJobHasMissedMoreThanTwoPeriods(t *testing.T) {
	for _, tc := range []struct {
		job   string
		ratio float64
		want  string
	}{
		{"purge-audit", 2, ""},
		{"purge-audit", 2.01, "run-purge-audit"},
		{"purge-audit", 0, ""},
		{"report", 9, ""},
	} {
		facts := NewMaintenanceFacts(capacity.Report{}, []jobruns.Status{{Job: tc.job, OverdueRatio: tc.ratio}})
		if got := facts.MaintenanceJobs[tc.job].Action; got != tc.want {
			t.Errorf("%s at %.2f periods overdue offers %q, want %q", tc.job, tc.ratio, got, tc.want)
		}
	}
}
