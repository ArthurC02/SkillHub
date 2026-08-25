// Run Trace ingestion and reading, end to end through the real route table
// (TRACE-002~008). The producer is simulated - a sandbox posting a batch to the
// ingestion URL it was handed - because the pipeline being tested here is the
// platform's half: token, scope, masking, idempotency, ordering and the two
// read modes. The real container half is exercised by
// apps/sandbox and by the docker end-to-end run in docs/plans/mvp/m2.
package apiserver_test

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

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
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
	Complete  bool          `json:"complete"`
	Streams   []traceStream `json:"streams"`
	NextAfter int64         `json:"next_after"`
	HasMore   bool          `json:"has_more"`
	Events    []struct {
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
	SkillsTotal  int `json:"skills_total"`
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
	ErrorsTotal int    `json:"errors_total"`
	Truncated   bool   `json:"summary_truncated"`
	FinalOutput string `json:"final_output"`
	Usage       *struct {
		InputTokens int64    `json:"input_tokens"`
		CostUSD     *float64 `json:"cost_usd"`
	} `json:"usage"`
	Steps []string `json:"steps"`
}

func (c *client) advancedTrace(t *testing.T, runID string) (int, advancedView) {
	return c.advancedTraceAfter(t, runID, 0)
}

func (c *client) advancedTraceAfter(t *testing.T, runID string, after int64) (int, advancedView) {
	t.Helper()
	resp, err := c.Get(fmt.Sprintf("%s/runs/%s/trace?mode=advanced&after=%d", c.base, runID, after))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out advancedView
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// One endpoint, one answer to "this violates the schema".
//
// `after` used to be parsed inside the `advanced` arm of the mode switch, so
// `?mode=general&after=-5` was answered with the general view and a 200 while
// `?mode=advanced&after=-5` was a 400. Same parameter, same illegal value, two
// different answers depending on a *different* parameter -- and the 200 is the
// dangerous half, because a caller who paged wrongly got a page that looked
// right. The table drives both modes on purpose: an implementation that fixed
// only the reported case would pass a test that only asked about that case.
func TestATraceCursorIsRefusedWhicheverViewWasAskedFor(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "trace-cursor")
	runID := seedRun(t, pool, c.workspaceID, seedSkill(t, pool, c.workspaceID, "trace-cursor-skill"))

	for _, mode := range []string{"", "general", "advanced"} {
		for _, after := range []string{"-5", "not-a-number"} {
			url := fmt.Sprintf("%s/runs/%s/trace?after=%s", c.base, runID, after)
			if mode != "" {
				url += "&mode=" + mode
			}
			resp, err := c.Get(url)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("after=%q mode=%q: got %d, want 400 -- an unusable cursor was answered with a page",
					after, mode, resp.StatusCode)
			}
		}
	}
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

	// event_id is the key by itself. A producer retry with a changed clock must
	// not bypass dedupe by moving the partition key.
	var shifted map[string]any
	if err := json.Unmarshal([]byte(toolCall), &shifted); err != nil {
		t.Fatal(err)
	}
	shifted["occurred_at"] = time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)
	shiftedJSON, err := json.Marshal(shifted)
	if err != nil {
		t.Fatal(err)
	}
	code, report = a.ingest(t, runID, 1, string(shiftedJSON))
	if code != http.StatusAccepted || report.Stored != 0 || report.Duplicate != 1 {
		t.Fatalf("same event id with shifted time: got %d %+v, want one duplicate", code, report)
	}

	status, view := owner.advancedTrace(t, runID)
	if status != http.StatusOK {
		t.Fatalf("GET trace: got %d", status)
	}
	if len(view.Events) != 1 {
		t.Fatalf("stored %d events after a re-send, want 1", len(view.Events))
	}
	if view.NextAfter <= 0 || view.HasMore {
		t.Fatalf("cursor = %d, has_more = %v; want a final non-zero cursor", view.NextAfter, view.HasMore)
	}
	if code, delta := owner.advancedTraceAfter(t, runID, view.NextAfter); code != http.StatusOK || len(delta.Events) != 0 || delta.NextAfter != view.NextAfter {
		t.Fatalf("empty delta: code=%d view=%+v", code, delta)
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

func TestConcurrentTraceDeliveryClaimsAnEventIDOnce(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "trace-concurrent-owner")
	skillID := seedSkill(t, pool, owner.workspaceID, "trace-concurrent-skill")
	runID := seedRun(t, pool, owner.workspaceID, skillID)
	eventID := deterministicEventID(runID, 1, 2)

	const insert = `INSERT INTO trace_events (
		event_id, workspace_id, run_id, attempt, seq, occurred_at,
		event_type, source, status, schema_version, masked, masked_fields, payload, late
	) VALUES ($1,$2,$3,1,2,$4,'agent_output','sandbox','ok','1.0',true,'[]'::jsonb,'{}'::jsonb,false)`
	type outcome struct {
		rows int64
		err  error
	}
	eventUUID := mustUUID(t, eventID)
	workspaceUUID := mustUUID(t, owner.workspaceID)
	runUUID := mustUUID(t, runID)
	ctx := context.Background()
	conn1, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn1.Release()
	conn2, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.Release()
	tx1, err := conn1.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx1.Rollback(ctx) //nolint:errcheck // no-op after commit
	tx2, err := conn2.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx2.Rollback(ctx) //nolint:errcheck // no-op after commit

	firstTag, err := tx1.Exec(ctx, insert, eventUUID, workspaceUUID, runUUID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	out := make(chan outcome, 1)
	go func() {
		tag, err := tx2.Exec(ctx, insert, eventUUID, workspaceUUID, runUUID, time.Now().UTC().Add(time.Minute))
		out <- outcome{tag.RowsAffected(), err}
	}()

	deadline := time.Now().Add(2 * time.Second)
	waiting := false
	for !waiting && time.Now().Before(deadline) {
		if err := pool.QueryRow(ctx, `SELECT COALESCE((SELECT wait_event_type = 'Lock'
			FROM pg_stat_activity WHERE pid=$1), false)`, conn2.Conn().PgConn().PID()).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if !waiting {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if !waiting {
		t.Fatal("second insert did not wait on the first transaction's advisory lock")
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	second := <-out
	if second.err != nil {
		t.Fatal(second.err)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if firstTag.RowsAffected() != 1 || second.rows != 0 {
		t.Fatalf("rows affected = %d and %d, want first=1 second=0", firstTag.RowsAffected(), second.rows)
	}
	var stored int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM trace_events WHERE event_id=$1", eventUUID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 1 {
		t.Fatalf("stored %d rows for one event_id, want 1", stored)
	}
}

func TestTraceCursorAssignmentSerializesWithCommitPerRun(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "trace-cursor-owner")
	skillID := seedSkill(t, pool, owner.workspaceID, "trace-cursor-skill")
	runID := seedRun(t, pool, owner.workspaceID, skillID)

	ctx := context.Background()
	tx1, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx1.Rollback(ctx) //nolint:errcheck // no-op after commit
	insert := `INSERT INTO trace_events
		(event_id, workspace_id, run_id, attempt, seq, occurred_at, event_type, source,
		 schema_version, masked, masked_fields, payload, late)
		VALUES (gen_random_uuid(), $1, $2, 1, $3, now(), 'script_log', 'sandbox',
		        '1.0', true, '[]', '{}', false)`
	if _, err := tx1.Exec(ctx, insert, owner.workspaceID, runID, 1); err != nil {
		t.Fatal(err)
	}

	tx2, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx2.Rollback(ctx) //nolint:errcheck // no-op after commit
	second := make(chan error, 1)
	go func() {
		_, execErr := tx2.Exec(ctx, insert, owner.workspaceID, runID, 2)
		second <- execErr
	}()
	select {
	case err := <-second:
		t.Fatalf("second insert did not wait for the first commit: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	var first, secondCursor int64
	if err := pool.QueryRow(ctx, `SELECT ingest_seq FROM trace_events WHERE run_id=$1 AND seq=1`, runID).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT ingest_seq FROM trace_events WHERE run_id=$1 AND seq=2`, runID).Scan(&secondCursor); err != nil {
		t.Fatal(err)
	}
	if first >= secondCursor {
		t.Fatalf("commit-ordered cursors = %d then %d", first, secondCursor)
	}
}

func TestEvaluationTraceSelectionIsBoundedAndCanonicallyOrdered(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "trace-page-order-owner")
	skillID := seedSkill(t, pool, owner.workspaceID, "trace-page-order-skill")
	runID := seedRun(t, pool, owner.workspaceID, skillID)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO trace_events
		(event_id, workspace_id, run_id, attempt, seq, occurred_at, event_type, source,
		 schema_version, masked, masked_fields, payload, late)
		SELECT gen_random_uuid(), $1, $2, 1, n,
		       now() - (n * interval '1 second'), 'script_log', 'sandbox',
		       '1.0', true, '[]', '{}', false
		FROM generate_series(1, 1001) AS n`, owner.workspaceID, runID)
	if err != nil {
		t.Fatal(err)
	}
	runs := &run.Service{Pool: pool}
	traceSvc := &trace.Service{
		Pool: pool,
		ReadRunState: func(ctx context.Context, workspaceID, runID pgtype.UUID) (trace.RunState, bool, error) {
			state, found, err := runs.TraceRun(ctx, workspaceID, runID)
			return trace.RunState{Status: state.Status, StatusReason: state.StatusReason}, found, err
		},
	}
	view, err := traceSvc.AdvancedAll(ctx, mustUUID(t, owner.workspaceID), mustUUID(t, runID))
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Events) != 500 || !view.EvaluationTruncated || view.Complete {
		t.Fatalf("bounded evaluation trace = %d events, truncated=%v complete=%v", len(view.Events), view.EvaluationTruncated, view.Complete)
	}
	if view.Events[0].Seq != 500 || view.Events[len(view.Events)-1].Seq != 1 {
		t.Fatalf("bounded canonical order starts/ends at seq %d/%d", view.Events[0].Seq, view.Events[len(view.Events)-1].Seq)
	}
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

	forged := strings.Replace(event(runID, 1, 2, "error", `{"category":"runtime","code":"forged","message":"fake"}`),
		`"emitted_by": "sandbox"`, `"emitted_by": "orchestrator"`, 1)
	code, report = a.ingest(t, runID, 1, forged)
	if code != http.StatusAccepted || report.Rejected != 1 || report.Stored != 0 {
		t.Fatalf("producer impersonation: got %d %+v, want it rejected", code, report)
	}

	_, view := owner.advancedTrace(t, otherRunID)
	if len(view.Events) != 0 {
		t.Errorf("run B has %d events it was never sent", len(view.Events))
	}
}

// TRACE-001: "Trace 事件順序可被重建，並能識別缺失或延遲事件". A hole in a
// producer's gapless sequence is a lost event and the view has to say so.
// TRACE-008: a sandbox is an at-least-once producer, so one event the platform
// cannot accept must not sink the batch it arrived in. If it did, the producer
// would resend, collide on the same event, get another 500, and every event
// queued behind it would never land - while the events ahead of it are already
// committed, so the retry could never converge either.
//
//nolint:unused
func offTestOneRefusedEventStillDeliversTheRestAndLetsTheResendConverge(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "trace-seq-collision-owner")
	skillID := seedSkill(t, pool, owner.workspaceID, "trace-seq-collision-skill")
	runID := seedRun(t, pool, owner.workspaceID, skillID)

	output := `{"kind":"final","text":"%s","truncated":false}`

	// seq 2 is claimed first, by the event the producer originally emitted there.
	if code, report := a.ingest(t, runID, 1,
		event(runID, 1, 2, "agent_output", fmt.Sprintf(output, "the original seq 2"))); code != http.StatusAccepted || report.Stored != 1 {
		t.Fatalf("seeding seq 2: got %d %+v, want 202 with 1 stored", code, report)
	}

	// The same stream position now arrives under a different event_id. That is
	// not an at-least-once redelivery (0031 would have swallowed it), it is the
	// producer contradicting itself - and it sits in the middle of the batch.
	collidingID := deterministicEventID(runID, 99, 99)
	collision := strings.Replace(
		event(runID, 1, 2, "agent_output", fmt.Sprintf(output, "a second claim on seq 2")),
		deterministicEventID(runID, 1, 2), collidingID, 1)
	batch := []string{
		event(runID, 1, 1, "agent_output", fmt.Sprintf(output, "ahead of the collision")),
		collision,
		event(runID, 1, 3, "agent_output", fmt.Sprintf(output, "behind the collision")),
	}

	code, report := a.ingest(t, runID, 1, batch...)
	if code != http.StatusAccepted {
		t.Fatalf("colliding batch: got %d %+v, want 202 - one bad event is not a failed batch", code, report)
	}
	if report.Stored != 2 || report.Rejected != 1 {
		t.Fatalf("colliding batch: got %+v, want 2 stored and 1 rejected", report)
	}
	// The producer has to be told *which* event was refused, or it cannot stop
	// sending it.
	if !strings.Contains(strings.Join(report.Reasons, " | "), collidingID) {
		t.Errorf("report does not name the refused event %s: %+v", collidingID, report.Reasons)
	}

	// The event behind the collision landed, and the evidence already at seq 2
	// was not overwritten by the second claim (trace_events is append-only).
	assertCollisionTrace := func(stage string) {
		t.Helper()
		status, view := owner.advancedTrace(t, runID)
		if status != http.StatusOK || len(view.Events) != 3 {
			t.Fatalf("%s: got %d with %d events, want 200 with seq 1, 2 and 3", stage, status, len(view.Events))
		}
		// Ordering is by occurred_at across streams, so index is not seq: look
		// each position up by the number the producer declared.
		bySeq := map[int64]json.RawMessage{}
		for _, ev := range view.Events {
			bySeq[ev.Seq] = ev.Payload
		}
		for _, seq := range []int64{1, 2, 3} {
			if _, ok := bySeq[seq]; !ok {
				t.Fatalf("%s: seq %d never landed; stored %d events", stage, seq, len(view.Events))
			}
		}
		if !strings.Contains(string(bySeq[2]), "the original seq 2") {
			t.Errorf("%s: seq 2 was overwritten by the colliding event: %s", stage, bySeq[2])
		}
		if !view.Complete {
			t.Errorf("%s: stream reports a gap although seq 1..3 are all stored", stage)
		}
	}
	assertCollisionTrace("after the colliding batch")

	// The push that matters: the producer never got an ack it could trust, so it
	// sends the identical batch again. It must converge - duplicates for the two
	// that landed, the same refusal for the one that cannot.
	code, report = a.ingest(t, runID, 1, batch...)
	if code != http.StatusAccepted || report.Stored != 0 || report.Duplicate != 2 || report.Rejected != 1 {
		t.Fatalf("resend: got %d %+v, want 202 with 0 stored, 2 duplicate and 1 rejected", code, report)
	}
	assertCollisionTrace("after the resend")
}

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

	events := []string{
		event(runID, 1, 1, "skill_activation", `{"skill_version_id":"4d5e6f7a-8b9c-4d1e-9f2a-3b4c5d6e7f81","skill_name":"excel-deduplicate","decision":"activated","reason":"the task mentions duplicate rows"}`),
		event(runID, 1, 2, "resource_read", `{"resource_path":"references/dedupe-rules.md","outcome":"read","bytes_read":4812,"truncated":false}`),
		event(runID, 1, 3, "tool_call", `{"tool_name":"bash","outcome":"succeeded","duration_ms":3412,"truncated":false}`),
		event(runID, 1, 4, "tool_call", `{"tool_name":"bash","outcome":"failed","duration_ms":120,"truncated":false}`),
		event(runID, 1, 5, "agent_output", `{"kind":"final","text":"Removed 17 duplicate rows.","truncated":false}`),
		event(runID, 1, 6, "usage", `{"scope":"run_total","model":"gpt-5-mini","input_tokens":27042,"output_tokens":1180,"cost_usd":null}`),
		// Payload shape is deliberately not trusted at ingress. An out-of-range
		// JSON number must be ignored by the aggregate, not turn this read into 500.
		event(runID, 1, 7, "tool_call", `{"tool_name":"hostile","outcome":"succeeded","duration_ms":1e100}`),
	}
	// Individually valid values may still overflow bigint when summed. The API
	// saturates its int64 transport field instead of making the whole trace 500.
	for seq := 8; seq <= 18; seq++ {
		events = append(events, event(runID, 1, seq, "tool_call", `{"tool_name":"large","outcome":"succeeded","duration_ms":900000000000000000}`))
	}
	if code, report := a.ingest(t, runID, 1, events...); code != http.StatusAccepted || report.Stored != len(events) {
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
	if view.SkillsTotal != 1 || view.ErrorsTotal != 0 || view.Truncated {
		t.Errorf("summary bounds = skills %d, errors %d, truncated %v", view.SkillsTotal, view.ErrorsTotal, view.Truncated)
	}
	if view.ResourceRead != 1 {
		t.Errorf("resources_read = %d, want 1", view.ResourceRead)
	}
	if view.ToolCalls.Total != 14 || view.ToolCalls.Succeeded != 13 || view.ToolCalls.Failed != 1 {
		t.Errorf("tool call summary = %+v", view.ToolCalls)
	}
	if view.ToolCalls.TotalMS != int64(1<<63-1) {
		t.Errorf("total tool duration = %d, want saturated int64", view.ToolCalls.TotalMS)
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
