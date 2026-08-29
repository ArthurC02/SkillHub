// The trace read-side folds, tested against the queries that actually serve
// them.
//
// TRACE-006 and TRACE-008 used to have unit tests in trial/evidence over a Go
// fold — (*Summary).fold and streamHealth — that the read path had stopped
// calling when both moved into db/queries/trace.sql. Breaking either query left
// those tests green, which is the fourth instance of the failure AGENTS.md
// 開發自動化 §9 exists for: the tests existed, and nothing could make them red.
//
// These are the same cases against GetTraceStreamHealth and GetTraceGeneralFold.
// They need a database because the logic being tested IS the database.
package apiserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// insertTraceEvents writes events straight to the table. Ingestion has its own
// tests; what these need is a stored shape, including ones the wire cannot
// produce cheaply (a stream whose highest seq is 100,000).
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

// TRACE-008: a hole in a producer's sequence is a lost event, and the read side
// has to say so rather than present a shorter timeline as complete. seq is
// gapless from 1 by contract, so "1..max minus what arrived" is exactly the set
// of lost events — no heuristic and no guessing, which is why missing_count is
// exact even when the reported sample is not.
func TestTraceStreamHealthCountsEveryHoleAndBoundsTheSample(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "trace-fold-health")

	// One skill per run: seedSkillVersion's content hash is derived from the
	// skill, and skill_versions_content_key makes two runs on one skill collide.
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

	// A stream whose highest seq is the contract's ceiling: the count is exact
	// (99,998) while the reported ordinals stop at 1,000. Both halves matter —
	// the count is what the UI says, the sample is what stops the response from
	// carrying a hundred thousand integers.
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

// TRACE-006's usage fold, all three branches the SQL distinguishes and the Go
// fold it replaced also had:
//
//   - a `run_total` usage event is the run's own total and REPLACES the sum,
//     rather than being added to the per-call events it summarises;
//   - model and cost_source come from the LAST usage event whichever branch
//     supplied the numbers, because they describe what was being used and not
//     how much;
//   - a cost the gateway never reported stays null. Showing 0 would tell the
//     user their run was free (contract README §5), which is a different claim
//     from "nobody said".
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

	// (1) No run_total: the per-call events are summed.
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
	if u.CostUSD == nil || *u.CostUSD != 0.75 {
		t.Errorf("summed cost = %v, want 0.75", u.CostUSD)
	}
	if u.Model != "gpt-5-nano" {
		t.Errorf("model = %q, want the last usage event's %q", u.Model, "gpt-5-nano")
	}

	// (2) A run_total event: it replaces the sum rather than joining it. Its
	// numbers are deliberately NOT the sum of the two per-call events above, so a
	// fold that added them together cannot pass by arithmetic coincidence.
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
	if u.CostUSD == nil || *u.CostUSD != 0.9 {
		t.Errorf("cost = %v, want the run_total event's 0.9", u.CostUSD)
	}

	// (3) An unreported cost stays null, on the run_total branch — the branch that
	// carries the authoritative numbers, so it is the one where a 0 would be read
	// as "this run was free".
	free := newRun("trace-fold-usage-free")
	insertTraceEvents(t, pool, c.workspaceID, free, 1, []int64{1}, "usage", usage("run_total", "gpt-5-mini", 27042, 1180, "null"))

	u = c.foldedUsage(t, free)
	if u == nil {
		t.Fatal("usage events did not reach the summary")
	}
	if u.CostUSD != nil {
		t.Errorf("cost_usd = %v, want null (unreported, not $0)", *u.CostUSD)
	}
	if u.InputTokens != 27042 {
		t.Errorf("input tokens = %d, want 27042 — a null cost must not lose the token counts", u.InputTokens)
	}
}

// --- readers -----------------------------------------------------------------

type foldUsage struct {
	Model        string   `json:"model"`
	InputTokens  int64    `json:"input_tokens"`
	OutputTokens int64    `json:"output_tokens"`
	CostUSD      *float64 `json:"cost_usd"`
	CostSource   string   `json:"cost_source"`
}

// foldedUsage reads the general view's usage block. Its own struct rather than
// generalView's, which does not carry model or cost_source.
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

// advancedTraceRaw is advancedTrace with missing_count, which the shared
// advancedView struct drops.
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
