// Run Trace ingestion and reading, end to end through the real route table
// (TRACE-002~008). The producer is simulated - a sandbox posting a batch to the
// ingestion URL it was handed - because the pipeline being tested here is the
// platform's half: token, scope, masking, idempotency, ordering and the two
// read modes. The real container half is exercised by
// services/sandbox and by the docker end-to-end run in plans/mvp/m2.
package identity_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/services/platform/internal/run"
	"github.com/ArthurC02/skillhub/services/platform/internal/trace"
)

// --- seeding -----------------------------------------------------------------

// seedRun writes a queued run straight into the database. These tests are about
// the trace, so the run is a fixture: going through POST /skills/{id}/runs would
// couple them to scheduling, permission confirmation and the queue, none of
// which decides anything here.
func seedRun(t *testing.T, pool *pgxpool.Pool, workspaceID, skillID string) string {
	t.Helper()
	ctx := context.Background()
	versionID := seedSkillVersion(t, pool, workspaceID, skillID)
	testCaseID := seedTestCase(t, pool, workspaceID, skillID)

	var snapshotID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO test_case_snapshots (workspace_id, test_case_id, user_prompt, acceptance_criteria, content_hash)
		VALUES ($1, $2, 'summarise the attached csv', '[]'::jsonb, 'sha256:snapshot')
		RETURNING id::text`,
		mustUUID(t, workspaceID), mustUUID(t, testCaseID),
	).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}

	var runID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO runs (workspace_id, skill_version_id, test_case_snapshot_id, provider,
		                  runtime_snapshot, policy_snapshot)
		VALUES ($1, $2, $3, 'fake_sandbox', '{}'::jsonb, '{}'::jsonb)
		RETURNING id::text`,
		mustUUID(t, workspaceID), mustUUID(t, versionID), mustUUID(t, snapshotID),
	).Scan(&runID); err != nil {
		t.Fatal(err)
	}

	// The first transition, as Create writes it. Without it the general view has
	// no progress steps, and the fixture would be testing a run shape that the
	// API cannot produce.
	if _, err := pool.Exec(ctx, `
		INSERT INTO run_status_transitions (run_id, workspace_id, to_status, reason)
		VALUES ($1, $2, 'queued', 'run requested')`,
		mustUUID(t, runID), mustUUID(t, workspaceID),
	); err != nil {
		t.Fatal(err)
	}
	return runID
}

// --- producer side -----------------------------------------------------------

// event builds one wire event the way the sandbox harness does.
func event(runID string, attempt, seq int, eventType string, payload string) string {
	return fmt.Sprintf(`{
		"schema_version": "1.0",
		"event_id": "%s",
		"run_id": "%s",
		"attempt": %d,
		"seq": %d,
		"occurred_at": "%s",
		"emitted_by": "sandbox",
		"type": "%s",
		"status": "ok",
		"masked": false,
		"masked_fields": [],
		"payload": %s
	}`, deterministicEventID(runID, attempt, seq), runID, attempt, seq,
		time.Now().UTC().Format(time.RFC3339Nano), eventType, payload)
}

// deterministicEventID makes a re-post of "the same event" actually the same
// event: the idempotency key is the producer's event_id, so a test for
// at-least-once delivery has to keep it stable across the two posts.
func deterministicEventID(runID string, attempt, seq int) string {
	return fmt.Sprintf("%s%02d%02d", runID[:len(runID)-4], attempt, seq)
}

type ingestReport struct {
	Received  int      `json:"received"`
	Stored    int      `json:"stored"`
	Duplicate int      `json:"duplicate"`
	Rejected  int      `json:"rejected"`
	Reasons   []string `json:"reasons"`
}

// ingest posts a batch the way sandboxd does: no session, no provider token,
// just the signed URL it was handed in its RunRequest.
func (a *api) ingest(t *testing.T, runID string, attempt int, events ...string) (int, ingestReport) {
	t.Helper()
	var runUUID pgtype.UUID
	if err := runUUID.Scan(runID); err != nil {
		t.Fatal(err)
	}
	url := a.traceSigner.IngestionURL(a.URL, runUUID, attempt, time.Now())
	return a.ingestTo(t, url, events...)
}

func (a *api) ingestTo(t *testing.T, url string, events ...string) (int, ingestReport) {
	t.Helper()
	body := "[" + strings.Join(events, ",") + "]"
	resp, err := http.Post(url, "application/json", strings.NewReader(body)) //nolint:noctx // test client
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var report ingestReport
	_ = json.NewDecoder(resp.Body).Decode(&report)
	return resp.StatusCode, report
}

// --- reader side -------------------------------------------------------------

type traceStream struct {
	Attempt    int     `json:"attempt"`
	EmittedBy  string  `json:"emitted_by"`
	Received   int     `json:"received"`
	MissingSeq []int64 `json:"missing_seq"`
	LateEvents int     `json:"late_events"`
}

type advancedView struct {
	Complete bool          `json:"complete"`
	Streams  []traceStream `json:"streams"`
	Events   []struct {
		Seq          int64           `json:"seq"`
		Type         string          `json:"type"`
		EmittedBy    string          `json:"emitted_by"`
		Late         bool            `json:"late"`
		MaskedFields []string        `json:"masked_fields"`
		Payload      json.RawMessage `json:"payload"`
	} `json:"events"`
}

type generalView struct {
	Status   string `json:"status"`
	Complete bool   `json:"complete"`
	Skills   []struct {
		Name     string `json:"name"`
		Decision string `json:"decision"`
	} `json:"skills"`
	ResourceRead int `json:"resources_read"`
	ToolCalls    struct {
		Total     int   `json:"total"`
		Succeeded int   `json:"succeeded"`
		Failed    int   `json:"failed"`
		TotalMS   int64 `json:"total_duration_ms"`
	} `json:"tool_calls"`
	Errors []struct {
		Category string `json:"category"`
		Code     string `json:"code"`
		Message  string `json:"message"`
	} `json:"errors"`
	FinalOutput string `json:"final_output"`
	Usage       *struct {
		InputTokens int64    `json:"input_tokens"`
		CostUSD     *float64 `json:"cost_usd"`
	} `json:"usage"`
	Steps []string `json:"steps"`
}

func (c *client) advancedTrace(t *testing.T, runID string) (int, advancedView) {
	t.Helper()
	resp, err := c.Get(c.base + "/runs/" + runID + "/trace?mode=advanced")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out advancedView
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (c *client) generalTrace(t *testing.T, runID string) (int, generalView) {
	t.Helper()
	resp, err := c.Get(c.base + "/runs/" + runID + "/trace")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out generalView
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// --- tests -------------------------------------------------------------------

// TRACE-005 plus TRACE-008's idempotency, in one pass because they are the two
// things that must hold on every single write.
func TestTraceIngestionMasksBeforeStorageAndDedupesOnResend(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "trace-owner")
	skillID := seedSkill(t, pool, owner.workspaceID, "trace-skill")
	runID := seedRun(t, pool, owner.workspaceID, skillID)

	toolCall := event(runID, 1, 1, "tool_call", `{
		"tool_name": "bash",
		"invocation_id": "call_7fd21",
		"arguments": {"command": "curl -H 'Authorization: Bearer sk-TESTKEYAAAAAAAAAAAAAAAAAAAA' https://api"},
		"result_summary": "wrote output.xlsx",
		"outcome": "succeeded",
		"duration_ms": 3412,
		"truncated": false
	}`)

	code, report := a.ingest(t, runID, 1, toolCall)
	if code != http.StatusAccepted || report.Stored != 1 {
		t.Fatalf("first push: got %d %+v, want 202 with 1 stored", code, report)
	}

	// At-least-once delivery: the same batch again must change nothing.
	code, report = a.ingest(t, runID, 1, toolCall)
	if code != http.StatusAccepted || report.Stored != 0 || report.Duplicate != 1 {
		t.Fatalf("re-push: got %d %+v, want 202 with 0 stored and 1 duplicate", code, report)
	}

	status, view := owner.advancedTrace(t, runID)
	if status != http.StatusOK {
		t.Fatalf("GET trace: got %d", status)
	}
	if len(view.Events) != 1 {
		t.Fatalf("stored %d events after a re-send, want 1", len(view.Events))
	}
	stored := string(view.Events[0].Payload)
	if strings.Contains(stored, "sk-TESTKEY") {
		t.Errorf("the key reached storage in plaintext: %s", stored)
	}
	if !strings.Contains(stored, trace.Placeholder) {
		t.Errorf("nothing was redacted: %s", stored)
	}
	if len(view.Events[0].MaskedFields) == 0 {
		t.Error("masked_fields is empty although a value was redacted")
	}

	// Iron rule 11, checked at the source rather than through the API: nothing
	// unmasked is in the column at all.
	var raw string
	if err := pool.QueryRow(context.Background(),
		`SELECT payload::text FROM trace_events WHERE run_id = $1`, mustUUID(t, runID)).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "sk-TESTKEY") {
		t.Errorf("plaintext secret in trace_events.payload: %s", raw)
	}

	dumpStoredEvents(t, pool, runID)
}

// dumpStoredEvents writes the stored rows back out as wire envelopes, so
// tools/contracts/validate_trace_events.py can check what the pipeline actually
// persists - masked and all - against the schema, rather than only the schema's
// own examples.
func dumpStoredEvents(t *testing.T, pool *pgxpool.Pool, runID string) {
	t.Helper()
	path := os.Getenv("SKILLHUB_TRACE_SAMPLE_OUT")
	if path == "" {
		return
	}
	rows, err := pool.Query(context.Background(), `
		SELECT jsonb_build_object(
			'schema_version', schema_version, 'event_id', event_id, 'run_id', run_id,
			'attempt', attempt, 'seq', seq,
			'occurred_at', to_char(occurred_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
			'emitted_by', source, 'type', event_type, 'status', status,
			'masked', masked, 'masked_fields', masked_fields, 'payload', payload
		)::text
		FROM trace_events WHERE run_id = $1 ORDER BY occurred_at, source, seq`, mustUUID(t, runID))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		out.WriteString(line)
		out.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(out.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The token is the only thing standing between an untrusted execution plane and
// somebody else's timeline.
func TestTraceIngestionRefusesWhatTheTokenDoesNotCover(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "trace-token-owner")
	// Two skills, because seedSkillVersion derives the version's content hash
	// from the skill id and versions are unique on content (INGEST-005).
	runID := seedRun(t, pool, owner.workspaceID, seedSkill(t, pool, owner.workspaceID, "trace-token-skill-a"))
	otherRunID := seedRun(t, pool, owner.workspaceID, seedSkill(t, pool, owner.workspaceID, "trace-token-skill-b"))

	output := `{"kind":"final","text":"done","truncated":false}`

	// Unsigned token: no session, no provider credential, nothing.
	code, _ := a.ingestTo(t, a.URL+trace.IngestPath+"not-a-token", event(runID, 1, 1, "agent_output", output))
	if code != http.StatusUnauthorized {
		t.Errorf("unsigned token: got %d, want 401", code)
	}

	// A valid token for run A carrying an event that claims run B. Accepted as a
	// request, rejected as an event: it must not land on B's timeline.
	code, report := a.ingest(t, runID, 1, event(otherRunID, 1, 1, "agent_output", output))
	if code != http.StatusAccepted || report.Rejected != 1 || report.Stored != 0 {
		t.Fatalf("cross-run event: got %d %+v, want it rejected", code, report)
	}

	// Same for an event claiming a different attempt of its own run: that would
	// collide with the other attempt's sequence numbers.
	code, report = a.ingest(t, runID, 1, event(runID, 2, 1, "agent_output", output))
	if code != http.StatusAccepted || report.Rejected != 1 {
		t.Fatalf("cross-attempt event: got %d %+v, want it rejected", code, report)
	}

	_, view := owner.advancedTrace(t, otherRunID)
	if len(view.Events) != 0 {
		t.Errorf("run B has %d events it was never sent", len(view.Events))
	}
}

// TRACE-001: "Trace 事件順序可被重建，並能識別缺失或延遲事件". A hole in a
// producer's gapless sequence is a lost event and the view has to say so.
func TestAdvancedViewNamesMissingEventsAndRefusesToLookComplete(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "trace-gap-owner")
	skillID := seedSkill(t, pool, owner.workspaceID, "trace-gap-skill")
	runID := seedRun(t, pool, owner.workspaceID, skillID)

	output := `{"kind":"intermediate","text":"working","truncated":false}`
	if code, report := a.ingest(t, runID, 1,
		event(runID, 1, 1, "agent_output", output),
		// seq 2 was lost in transit.
		event(runID, 1, 3, "agent_output", output),
	); code != http.StatusAccepted || report.Stored != 2 {
		t.Fatalf("push: got %d %+v", code, report)
	}

	_, view := owner.advancedTrace(t, runID)
	if view.Complete {
		t.Error("a trace with a hole in it reported itself complete")
	}
	if len(view.Streams) != 1 || len(view.Streams[0].MissingSeq) != 1 || view.Streams[0].MissingSeq[0] != 2 {
		t.Fatalf("streams = %+v, want one stream missing seq 2", view.Streams)
	}
	// Order is reconstructed, not accidental.
	if view.Events[0].Seq != 1 || view.Events[1].Seq != 3 {
		t.Errorf("events came back out of order: %d then %d", view.Events[0].Seq, view.Events[1].Seq)
	}

	// The gap arriving late fills it, and the view stops claiming loss.
	if code, report := a.ingest(t, runID, 1, event(runID, 1, 2, "agent_output", output)); code != http.StatusAccepted || report.Stored != 1 {
		t.Fatalf("late push: got %d %+v", code, report)
	}
	if _, view = owner.advancedTrace(t, runID); !view.Complete {
		t.Errorf("the filled gap was still reported missing: %+v", view.Streams)
	}
}

// TRACE-008: a terminal run is not a closed inbox. The sandbox pushes its last
// batch as it shuts down, which is routinely after the platform has settled the
// run - and that tail is the part a failed run most needs (RUN-004).
func TestEventsArrivingAfterTheRunFinishedAreKeptAndFlagged(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "trace-late-owner")
	skillID := seedSkill(t, pool, owner.workspaceID, "trace-late-skill")
	runID := seedRun(t, pool, owner.workspaceID, skillID)

	svc := &run.Service{Pool: pool}
	if _, err := svc.Transition(context.Background(), run.TransitionParams{
		WorkspaceID: mustUUID(t, owner.workspaceID), RunID: mustUUID(t, runID),
		From: gen.RunStatusQueued, To: gen.RunStatusFailed,
		Reason: "the provider could not carry the attempt", FailureClass: "provider_error",
	}); err != nil {
		t.Fatal(err)
	}

	if code, report := a.ingest(t, runID, 1,
		event(runID, 1, 1, "agent_output", `{"kind":"final","text":"partial answer","truncated":false}`),
	); code != http.StatusAccepted || report.Stored != 1 {
		t.Fatalf("late push: got %d %+v, want it accepted", code, report)
	}

	_, view := owner.advancedTrace(t, runID)
	var sandboxEvent, orchestratorEvent bool
	for _, e := range view.Events {
		switch e.EmittedBy {
		case "sandbox":
			sandboxEvent = true
			if !e.Late {
				t.Error("an event that arrived after the run finished was not flagged late")
			}
		case "orchestrator":
			orchestratorEvent = true
			if e.Type != "error" {
				t.Errorf("orchestrator wrote a %q event for a failed run, want error", e.Type)
			}
		}
	}
	if !sandboxEvent {
		t.Error("the late sandbox event was dropped")
	}
	// TRACE-004: a run that never reached a sandbox would otherwise have an
	// empty timeline, and RUN-004 says a failed run must still show diagnostics.
	if !orchestratorEvent {
		t.Error("the control plane's own failure was not recorded in the trace")
	}
}

// TRACE-006: the general mode has to be readable without opening a raw event,
// and its run status comes from the runs table, never from replayed events
// (iron rule 5).
func TestGeneralModeSummarisesTheRunWithoutRawEvents(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "trace-summary-owner")
	skillID := seedSkill(t, pool, owner.workspaceID, "trace-summary-skill")
	runID := seedRun(t, pool, owner.workspaceID, skillID)

	if code, report := a.ingest(t, runID, 1,
		event(runID, 1, 1, "skill_activation", `{"skill_version_id":"4d5e6f7a-8b9c-4d1e-9f2a-3b4c5d6e7f81","skill_name":"excel-deduplicate","decision":"activated","reason":"the task mentions duplicate rows"}`),
		event(runID, 1, 2, "resource_read", `{"resource_path":"references/dedupe-rules.md","outcome":"read","bytes_read":4812,"truncated":false}`),
		event(runID, 1, 3, "tool_call", `{"tool_name":"bash","outcome":"succeeded","duration_ms":3412,"truncated":false}`),
		event(runID, 1, 4, "tool_call", `{"tool_name":"bash","outcome":"failed","duration_ms":120,"truncated":false}`),
		event(runID, 1, 5, "agent_output", `{"kind":"final","text":"Removed 17 duplicate rows.","truncated":false}`),
		event(runID, 1, 6, "usage", `{"scope":"run_total","model":"gpt-5-mini","input_tokens":27042,"output_tokens":1180,"cost_usd":null}`),
	); code != http.StatusAccepted || report.Stored != 6 {
		t.Fatalf("push: got %d %+v", code, report)
	}

	status, view := owner.generalTrace(t, runID)
	if status != http.StatusOK {
		t.Fatalf("GET trace: got %d", status)
	}
	if view.Status != string(gen.RunStatusQueued) {
		t.Errorf("status = %q, want the runs table's own %q", view.Status, gen.RunStatusQueued)
	}
	if !view.Complete {
		t.Error("a gapless trace was reported incomplete")
	}
	if len(view.Skills) != 1 || view.Skills[0].Name != "excel-deduplicate" {
		t.Errorf("skills = %+v", view.Skills)
	}
	if view.ResourceRead != 1 {
		t.Errorf("resources_read = %d, want 1", view.ResourceRead)
	}
	if view.ToolCalls.Total != 2 || view.ToolCalls.Succeeded != 1 || view.ToolCalls.Failed != 1 {
		t.Errorf("tool call summary = %+v", view.ToolCalls)
	}
	if view.ToolCalls.TotalMS != 3532 {
		t.Errorf("total tool duration = %d, want 3532", view.ToolCalls.TotalMS)
	}
	if view.FinalOutput != "Removed 17 duplicate rows." {
		t.Errorf("final output = %q", view.FinalOutput)
	}
	if view.Usage == nil || view.Usage.InputTokens != 27042 {
		t.Fatalf("usage = %+v", view.Usage)
	}
	// A cost the gateway never reported must stay unreported, not become 0
	// (contract README §5): showing 0 would tell the user the run was free.
	if view.Usage.CostUSD != nil {
		t.Errorf("cost_usd = %v, want null (unreported)", *view.Usage.CostUSD)
	}
	if len(view.Steps) == 0 {
		t.Error("progress steps are empty although the run has a transition history")
	}
}

// WS-006: a trace is user data. Another workspace gets 404, not 403 - existence
// is itself private.
func TestTraceIsWorkspaceScoped(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "trace-scope-owner")
	stranger := a.login(t, "trace-scope-stranger")
	skillID := seedSkill(t, pool, owner.workspaceID, "trace-scope-skill")
	runID := seedRun(t, pool, owner.workspaceID, skillID)

	if code, _ := a.ingest(t, runID, 1,
		event(runID, 1, 1, "agent_output", `{"kind":"final","text":"secret business","truncated":false}`),
	); code != http.StatusAccepted {
		t.Fatalf("push: got %d", code)
	}

	if code, view := stranger.advancedTrace(t, runID); code != http.StatusNotFound || len(view.Events) != 0 {
		t.Errorf("stranger read another workspace's trace: %d, %d events", code, len(view.Events))
	}
	if code, _ := stranger.generalTrace(t, runID); code != http.StatusNotFound {
		t.Errorf("stranger read another workspace's summary: %d", code)
	}
}
