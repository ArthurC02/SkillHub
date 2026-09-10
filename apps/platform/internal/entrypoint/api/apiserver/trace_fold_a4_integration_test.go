package apiserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func insertTraceEvents(t *testing.T, pool *pgxpool.Pool, workspaceID, runID string, attempt int, seqs []int64, eventType, payload string) {
	t.Helper()
	for _, seq := range seqs {
		if _, err := pool.Exec(context.Background(), `
			INSERT INTO trace_events
			(event_id, workspace_id, run_id, attempt, seq, occurred_at, event_type, source,
			 schema_version, masked, masked_fields, payload, late)
			VALUES (gen_random_uuid(), $1, $2, $3, $4, now() + ($5 * interval '1 millisecond'),
			        $6, 'sandbox', '1.0', true, '[]', $7, false)`,
			workspaceID, runID, attempt, seq, float64(seq), eventType, payload); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTraceStreamHealthCountsEveryHoleAndBoundsTheSample(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "trace-fold-health")

	gappy := seedRun(t, pool, c.workspaceID, seedSkill(t, pool, c.workspaceID, "trace-fold-health-a"))
	insertTraceEvents(t, pool, c.workspaceID, gappy, 1, []int64{1, 2, 4}, "script_log", `{}`)

	status, view := c.advancedTrace(t, gappy)
	if status != 200 {
		t.Fatalf("advanced trace: got %d, want 200", status)
	}
	if view.Complete {
		t.Error("a stream missing seq 3 was reported complete")
	}
	if len(view.Streams) != 1 {
		t.Fatalf("got %d streams, want 1: %+v", len(view.Streams), view.Streams)
	}
	if got := view.Streams[0].MissingSeq; len(got) != 1 || got[0] != 3 {
		t.Errorf("missing_seq = %v, want [3]", got)
	}

	sparse := seedRun(t, pool, c.workspaceID, seedSkill(t, pool, c.workspaceID, "trace-fold-health-b"))
	insertTraceEvents(t, pool, c.workspaceID, sparse, 1, []int64{1, 100_000}, "script_log", `{}`)

	status, raw := c.advancedTraceRaw(t, sparse)
	if status != 200 {
		t.Fatalf("advanced trace: got %d, want 200", status)
	}
	if len(raw.Streams) != 1 {
		t.Fatalf("got %d streams, want 1", len(raw.Streams))
	}
	if raw.Streams[0].MissingCount != 99_998 {
		t.Errorf("missing_count = %d, want 99998 — the count must be exact, not the size of the sample",
			raw.Streams[0].MissingCount)
	}
	if len(raw.Streams[0].MissingSeq) != 1_000 {
		t.Errorf("missing_seq sample has %d entries, want 1000", len(raw.Streams[0].MissingSeq))
	}
}

func TestTraceGeneralFoldReadsUsageTheWayTheContractDoes(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "trace-fold-usage")
	newRun := func(name string) string {
		return seedRun(t, pool, c.workspaceID, seedSkill(t, pool, c.workspaceID, name))
	}

	usage := func(scope, model string, in, out int64, cost string) string {
		return fmt.Sprintf(
			`{"scope":%q,"model":%q,"input_tokens":%d,"output_tokens":%d,"cost_usd":%s,"cost_source":"gateway"}`,
			scope, model, in, out, cost)
	}

	summed := newRun("trace-fold-usage-summed")
	insertTraceEvents(t, pool, c.workspaceID, summed, 1, []int64{1}, "usage", usage("attempt", "gpt-5-mini", 1000, 100, "0.5"))
	insertTraceEvents(t, pool, c.workspaceID, summed, 1, []int64{2}, "usage", usage("attempt", "gpt-5-nano", 200, 20, "0.25"))

	u := c.foldedUsage(t, summed)
	if u == nil {
		t.Fatal("usage events did not reach the summary")
	}
	if u.InputTokens != 1200 || u.OutputTokens != 120 {
		t.Errorf("summed usage = %d in / %d out, want 1200 / 120", u.InputTokens, u.OutputTokens)
	}

	if u.CostCredits == nil || *u.CostCredits != 975 {
		t.Errorf("summed cost = %v credits, want 975 (the $0.75 sum converted)", derefCredits(u.CostCredits))
	}
	if u.Model != "gpt-5-nano" {
		t.Errorf("model = %q, want the last usage event's %q", u.Model, "gpt-5-nano")
	}

	total := newRun("trace-fold-usage-total")
	insertTraceEvents(t, pool, c.workspaceID, total, 1, []int64{1}, "usage", usage("attempt", "gpt-5-mini", 1000, 100, "0.5"))
	insertTraceEvents(t, pool, c.workspaceID, total, 1, []int64{2}, "usage", usage("attempt", "gpt-5-mini", 200, 20, "0.25"))
	insertTraceEvents(t, pool, c.workspaceID, total, 1, []int64{3}, "usage", usage("run_total", "gpt-5-mini", 27042, 1180, "0.9"))

	u = c.foldedUsage(t, total)
	if u == nil {
		t.Fatal("usage events did not reach the summary")
	}
	if u.InputTokens != 27042 || u.OutputTokens != 1180 {
		t.Errorf("usage = %d in / %d out, want the run_total event's 27042 / 1180 and not the sum",
			u.InputTokens, u.OutputTokens)
	}
	if u.CostCredits == nil || *u.CostCredits != 1170 {
		t.Errorf("cost = %v credits, want 1170 (the run_total event's $0.9 converted, not the sum)", derefCredits(u.CostCredits))
	}

	free := newRun("trace-fold-usage-free")
	insertTraceEvents(t, pool, c.workspaceID, free, 1, []int64{1}, "usage", usage("run_total", "gpt-5-mini", 27042, 1180, "null"))

	u = c.foldedUsage(t, free)
	if u == nil {
		t.Fatal("usage events did not reach the summary")
	}
	if u.CostCredits != nil {
		t.Errorf("cost_credits = %v, want null (unreported, not 0 點)", *u.CostCredits)
	}
	if u.InputTokens != 27042 {
		t.Errorf("input tokens = %d, want 27042 — a null cost must not lose the token counts", u.InputTokens)
	}
}

type foldUsage struct {
	Model        string `json:"model"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CostCredits  *int64 `json:"cost_credits"`
	CostSource   string `json:"cost_source"`
}

func (c *client) foldedUsage(t *testing.T, runID string) *foldUsage {
	t.Helper()
	resp, err := c.Get(c.base + "/runs/" + runID + "/trace?mode=general")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("general trace: got %d, want 200", resp.StatusCode)
	}
	var out struct {
		Usage *foldUsage `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Usage
}

type foldStreamHealth struct {
	Attempt      int     `json:"attempt"`
	MissingCount int64   `json:"missing_count"`
	MissingSeq   []int64 `json:"missing_seq"`
}

type foldAdvancedView struct {
	Streams []foldStreamHealth `json:"streams"`
}

func (c *client) advancedTraceRaw(t *testing.T, runID string) (int, foldAdvancedView) {
	t.Helper()
	var out foldAdvancedView
	resp, err := c.Get(c.base + "/runs/" + runID + "/trace?mode=advanced")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func derefCredits(v *int64) any {
	if v == nil {
		return "null"
	}
	return *v
}
