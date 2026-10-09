package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/product/operations"
)

const findingFacts = `{"jobs":{"purge":{"overdue":2.5},"rotate":{"overdue":3}},"budget_days":40}`

type findingWorld struct {
	t        *testing.T
	svc      *operations.Service
	def      operations.Definition
	operator *client
}

func newFindingWorld(t *testing.T, name string) *findingWorld {
	t.Helper()
	svc, def, operator := loopAgentWithOperator(t, name, 1_000_000, "maintenance_report")
	def.CheckResult, def.Sightings = operations.CitesOnlyReturnedFacts, operations.DailyReportSightings
	t.Cleanup(func() { removeFindings(t, name) })
	return &findingWorld{t: t, svc: svc, def: def, operator: operator}
}

func removeFindings(t *testing.T, agent string) {
	ctx := context.Background()
	if err := pgx.BeginFunc(ctx, testPool, func(tx pgx.Tx) error {
		for _, stmt := range []string{
			"DELETE FROM platform_agent_finding_events WHERE finding_id IN (SELECT f.id FROM platform_agent_findings f JOIN platform_agents a ON a.id = f.agent_id WHERE a.name = $1)",
			"DELETE FROM platform_agent_findings WHERE agent_id IN (SELECT id FROM platform_agents WHERE name = $1)",
			"SET LOCAL skillhub.purge = 'on'",
			"DELETE FROM audit_events WHERE actor_agent_id IN (SELECT id FROM platform_agents WHERE name = $1)",
		} {
			var args []any
			if stmt[0] == 'D' {
				args = []any{agent}
			}
			if _, err := tx.Exec(ctx, stmt, args...); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Errorf("cleanup findings: %v", err)
	}
}

func (w *findingWorld) report(items ...string) {
	w.t.Helper()
	result := `{"items":[` + joinComma(items) + `]}`
	facts := operations.Tool{
		Name: "maintenance_report", Description: "facts", Parameters: map[string]any{"type": "object"},
		Run: func(context.Context, json.RawMessage) (any, error) { return json.RawMessage(findingFacts), nil },
	}
	s := &loopScript{t: w.t, answers: answers(intent("maintenance_report"), final(result))}
	report, err := s.runner(w.svc).Run(context.Background(), w.def, []operations.Tool{facts}, loopLimits)
	if err != nil || report.Status != operations.RunCompleted {
		w.t.Fatalf("report run %+v, err %v", report, err)
	}
}

func joinComma(items []string) string {
	out := ""
	for i, item := range items {
		if i > 0 {
			out += ","
		}
		out += item
	}
	return out
}

func attention(text string, cites ...string) string {
	encoded, _ := json.Marshal(map[string]any{"status": "attention", "text": text, "cites": cites})
	return string(encoded)
}

func fine(text string, cites ...string) string {
	encoded, _ := json.Marshal(map[string]any{"status": "fine", "text": text, "cites": cites})
	return string(encoded)
}

type storedFinding struct {
	id        pgtype.UUID
	status    string
	title     string
	seenCount int
}

func (w *findingWorld) findings() []storedFinding {
	w.t.Helper()
	rows, err := testPool.Query(context.Background(), `SELECT f.id, f.status, f.title, f.seen_count
		FROM platform_agent_findings f JOIN platform_agents a ON a.id = f.agent_id
		WHERE a.name = $1 ORDER BY f.first_seen_at, f.title`, w.def.Name)
	if err != nil {
		w.t.Fatal(err)
	}
	defer rows.Close()
	var out []storedFinding
	for rows.Next() {
		var f storedFinding
		if err := rows.Scan(&f.id, &f.status, &f.title, &f.seenCount); err != nil {
			w.t.Fatal(err)
		}
		out = append(out, f)
	}
	return out
}

func (w *findingWorld) eventKinds(finding pgtype.UUID) []string {
	w.t.Helper()
	rows, err := testPool.Query(context.Background(),
		"SELECT kind FROM platform_agent_finding_events WHERE finding_id = $1 ORDER BY seq", finding)
	if err != nil {
		w.t.Fatal(err)
	}
	defer rows.Close()
	var kinds []string
	for rows.Next() {
		var kind string
		if err := rows.Scan(&kind); err != nil {
			w.t.Fatal(err)
		}
		kinds = append(kinds, kind)
	}
	return kinds
}

func (w *findingWorld) move(finding pgtype.UUID, status, note string) (int, map[string]any) {
	w.t.Helper()
	body, _ := json.Marshal(map[string]string{"status": status, "note": note})
	return operatorCall(w.t, w.operator, http.MethodPut, "/admin/agents/findings/"+uuidString(finding)+"/status", string(body))
}

func TestAReportedProblemIsOneFindingAcrossDaysUntilItRecoversAndReopens(t *testing.T) {
	w := newFindingWorld(t, "agent-findings-lifecycle")

	w.report(attention("purge is late", "/jobs/purge/overdue"), fine("budget fine", "/budget_days"))
	got := w.findings()
	if len(got) != 1 || got[0].status != "open" || got[0].seenCount != 1 {
		t.Fatalf("after day 1: %+v, want one open finding for the attention item only", got)
	}
	purge := got[0].id

	w.report(attention("purge is still late", "/jobs/purge/overdue"), attention("rotate is late", "/jobs/rotate/overdue"))
	got = w.findings()
	if len(got) != 2 || got[0].id != purge || got[0].seenCount != 2 || got[0].title != "purge is still late" {
		t.Fatalf("after day 2: %+v, want purge folded with the latest text, and rotate new", got)
	}

	w.report(attention("rotate is late", "/jobs/rotate/overdue"))
	if got = w.findings(); got[0].status != "recovered" || got[1].status != "open" {
		t.Fatalf("after day 3: %+v, want purge recovered and rotate still open", got)
	}

	w.report(attention("purge is late again", "/jobs/purge/overdue", "/budget_days"), attention("rotate is late", "/jobs/rotate/overdue"))
	got = w.findings()
	if len(got) != 2 || got[0].status != "open" || got[0].seenCount != 3 {
		t.Fatalf("after day 4: %+v, want purge reopened on the same row", got)
	}
	assertPurgeHistory(t, w, purge)
}

func TestARunThatFailsLeavesEveryFindingAsItWas(t *testing.T) {
	w := newFindingWorld(t, "agent-findings-failed-run")
	w.report(attention("purge is late", "/jobs/purge/overdue"))

	s := &loopScript{t: t, answers: answers(final(`{`))}
	report, err := s.runner(w.svc).Run(context.Background(), w.def, nil, loopLimits)
	if err != nil || report.Status != operations.RunFailed {
		t.Fatalf("broken run %+v, err %v, want it failed", report, err)
	}
	if got := w.findings(); len(got) != 1 || got[0].status != "open" || got[0].seenCount != 1 {
		t.Errorf("after a failed run: %+v, want the finding still open and seen once", got)
	}
}

func assertPurgeHistory(t *testing.T, w *findingWorld, purge pgtype.UUID) {
	t.Helper()
	if kinds := w.eventKinds(purge); !slices.Equal(kinds, []string{"opened", "seen", "recovered", "reopened"}) {
		t.Errorf("purge history %v, want opened, seen, recovered, reopened", kinds)
	}
	var evidence map[string]any
	if err := testPool.QueryRow(context.Background(),
		"SELECT evidence FROM platform_agent_finding_events WHERE finding_id = $1 AND seq = 0", purge).Scan(&evidence); err != nil ||
		evidence["/jobs/purge/overdue"] != 2.5 {
		t.Errorf("opening evidence %v (%v), want the cited value 2.5", evidence, err)
	}
	if n := countRow(t, testPool, "SELECT count(*) FROM audit_events WHERE resource_id = $1 AND actor_agent_id IS NOT NULL", purge); n != 3 {
		t.Errorf("agent audit events for purge: %d, want opened, recovered and reopened", n)
	}
}

func TestAnOperatorMovesAFindingOnlyAlongTheAllowedPaths(t *testing.T) {
	w := newFindingWorld(t, "agent-findings-moves")
	w.report(attention("purge is late", "/jobs/purge/overdue"))
	purge := w.findings()[0].id

	steps := []struct {
		status string
		note   string
		want   int
	}{
		{"acknowledged", "  ", http.StatusBadRequest},
		{"recovered", "only a report recovers it", http.StatusBadRequest},
		{"acknowledged", "looking into it", http.StatusNoContent},
		{"acknowledged", "again", http.StatusConflict},
		{"open", "never mind", http.StatusConflict},
		{"resolved", "rotated the job", http.StatusNoContent},
		{"dismissed", "too late", http.StatusConflict},
		{"acknowledged", "picking it up", http.StatusConflict},
		{"open", "it came back", http.StatusNoContent},
		{"acknowledged", "on it again", http.StatusNoContent},
		{"dismissed", "known and accepted", http.StatusNoContent},
	}
	for _, step := range steps {
		if code, body := w.move(purge, step.status, step.note); code != step.want {
			t.Fatalf("move to %s with %q: %d %v, want %d", step.status, step.note, code, body, step.want)
		}
	}
	if kinds := w.eventKinds(purge); !slices.Equal(kinds, []string{"opened", "acknowledged", "resolved", "reopened", "acknowledged", "dismissed"}) {
		t.Errorf("history %v, want each accepted move once", kinds)
	}
	var assignee pgtype.UUID
	if err := testPool.QueryRow(context.Background(),
		"SELECT assignee_id FROM platform_agent_findings WHERE id = $1", purge).Scan(&assignee); err != nil || !assignee.Valid {
		t.Errorf("assignee %v (%v), want the operator who took it on, kept after", assignee, err)
	}

	w.report(attention("purge is late", "/jobs/purge/overdue"))
	if got := w.findings(); len(got) != 1 || got[0].status != "dismissed" || got[0].seenCount != 2 {
		t.Errorf("a repeated dismissed finding: %+v, want it counted but still dismissed", got)
	}
	if code, _ := w.move(pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, "resolved", "nothing"); code != http.StatusNotFound {
		t.Errorf("unknown finding: %d, want 404", code)
	}
}

func TestAWrongFindingStatusNamesEveryStatusTheRequestMayUse(t *testing.T) {
	w := newFindingWorld(t, "agent-findings-status-words")
	w.report(attention("purge is late", "/jobs/purge/overdue"))
	purge := w.findings()[0].id

	if code, body := operatorCall(t, w.operator, http.MethodGet, "/admin/agents/findings?status=late", ""); code != http.StatusBadRequest ||
		body["error"] != "status must be one of open, acknowledged, resolved, dismissed, recovered" {
		t.Errorf("listing by an unknown status: %d %v, want 400 naming all five statuses", code, body)
	}
	if code, body := w.move(purge, "recovered", "only a report recovers it"); code != http.StatusBadRequest ||
		body["error"] != "status must be one of open, acknowledged, resolved, dismissed" {
		t.Errorf("moving to recovered: %d %v, want 400 naming the four an operator may set", code, body)
	}
}

func TestAReportLeavesClosedFindingsAloneUntilAResolvedOneComesBack(t *testing.T) {
	w := newFindingWorld(t, "agent-findings-closed")
	w.report(attention("purge is late", "/jobs/purge/overdue"), attention("rotate is late", "/jobs/rotate/overdue"))
	got := w.findings()
	purge, rotate := got[0].id, got[1].id
	if code, _ := w.move(purge, "resolved", "rotated the job"); code != http.StatusNoContent {
		t.Fatalf("resolve purge: %d", code)
	}
	if code, _ := w.move(rotate, "dismissed", "accepted"); code != http.StatusNoContent {
		t.Fatalf("dismiss rotate: %d", code)
	}

	w.report(fine("budget fine", "/budget_days"))
	if got = w.findings(); got[0].status != "resolved" || got[1].status != "dismissed" {
		t.Fatalf("after a quiet report: %+v, want resolved and dismissed left as they were", got)
	}

	w.report(attention("purge is late again", "/jobs/purge/overdue"))
	if got = w.findings(); got[0].status != "open" || got[0].seenCount != 2 {
		t.Fatalf("after purge came back: %+v, want the resolved finding reopened on the same row", got)
	}
	if kinds := w.eventKinds(purge); !slices.Equal(kinds, []string{"opened", "resolved", "reopened"}) {
		t.Errorf("purge history %v, want opened, resolved, reopened", kinds)
	}
}

func TestOperatorsListLiveFindingsWithCountsAndReadOneWithItsHistory(t *testing.T) {
	w := newFindingWorld(t, "agent-findings-read")
	w.report(attention("purge is late", "/jobs/purge/overdue"), attention("rotate is late", "/jobs/rotate/overdue"))
	got := w.findings()
	if code, body := w.move(got[1].id, "dismissed", "accepted"); code != http.StatusNoContent {
		t.Fatalf("dismiss: %d %v", code, body)
	}

	code, body := operatorCall(t, w.operator, http.MethodGet, "/admin/agents/findings", "")
	if code != http.StatusOK {
		t.Fatalf("GET findings: %d %v", code, body)
	}
	live := idsOf(body["findings"])
	if !slices.Contains(live, uuidString(got[0].id)) || slices.Contains(live, uuidString(got[1].id)) {
		t.Errorf("default list %v, want the open one and not the dismissed one", live)
	}
	if counts, _ := body["counts"].(map[string]any); counts["dismissed"].(float64) < 1 || counts["recovered"] == nil {
		t.Errorf("counts %v, want every status counted", counts)
	}
	_, body = operatorCall(t, w.operator, http.MethodGet, "/admin/agents/findings?status=dismissed", "")
	if dismissed := idsOf(body["findings"]); !slices.Contains(dismissed, uuidString(got[1].id)) {
		t.Errorf("dismissed list %v, want the dismissed one", dismissed)
	}
	if code, _ := operatorCall(t, w.operator, http.MethodGet, "/admin/agents/findings?status=closed", ""); code != http.StatusBadRequest {
		t.Errorf("unknown status filter: %d, want 400", code)
	}

	assertDismissedFindingDetail(t, w, got[1].id)
}

func TestFindingInboxContinuesPastOnePageWithoutLosingTiedRows(t *testing.T) {
	w := newFindingWorld(t, "agent-findings-pages")
	_, err := testPool.Exec(context.Background(), `
		INSERT INTO platform_agent_findings (agent_id, title, cites, last_seen_at)
		SELECT a.id, 'page item ' || n, ARRAY['/jobs/purge/overdue'], now() + interval '10 years'
		FROM platform_agents a CROSS JOIN generate_series(1, 26) AS n
		WHERE a.name = $1`, w.def.Name)
	if err != nil {
		t.Fatal(err)
	}

	code, first := operatorCall(t, w.operator, http.MethodGet, "/admin/agents/findings", "")
	if code != http.StatusOK || len(idsOf(first["findings"])) != 25 {
		t.Fatalf("first page = status %d, %d findings; want 200 and 25", code, len(idsOf(first["findings"])))
	}
	cursor, ok := first["next_cursor"].(string)
	if !ok || cursor == "" {
		t.Fatalf("first page continuation = %v, want cursor", first["next_cursor"])
	}
	_, err = testPool.Exec(context.Background(), `
		INSERT INTO platform_agent_findings (agent_id, title, cites, last_seen_at)
		SELECT id, 'newer page item', ARRAY['/jobs/purge/overdue'], now() + interval '11 years'
		FROM platform_agents WHERE name = $1`, w.def.Name)
	if err != nil {
		t.Fatal(err)
	}
	code, second := operatorCall(t, w.operator, http.MethodGet, "/admin/agents/findings?cursor="+cursor, "")
	if code != http.StatusOK || len(idsOf(second["findings"])) != 1 || second["next_cursor"] != nil {
		t.Fatalf("second page = status %d, %d findings, cursor %v; want 200, one finding, no cursor", code, len(idsOf(second["findings"])), second["next_cursor"])
	}
	for _, id := range idsOf(first["findings"]) {
		if id == idsOf(second["findings"])[0] {
			t.Fatalf("finding %s repeated across pages", id)
		}
	}
	if code, _ := operatorCall(t, w.operator, http.MethodGet, "/admin/agents/findings?cursor=bad", ""); code != http.StatusBadRequest {
		t.Fatalf("malformed cursor status = %d, want 400", code)
	}
	if code, _ := operatorCall(t, w.operator, http.MethodGet, "/admin/agents/findings?cursor="+cursor+"&cursor="+cursor, ""); code != http.StatusBadRequest {
		t.Fatalf("repeated cursor status = %d, want 400", code)
	}
}

func assertDismissedFindingDetail(t *testing.T, w *findingWorld, id pgtype.UUID) {
	t.Helper()
	code, body := operatorCall(t, w.operator, http.MethodGet, "/admin/agents/findings/"+uuidString(id), "")
	events, _ := body["events"].([]any)
	if code != http.StatusOK || len(events) != 2 {
		t.Fatalf("GET finding: %d %v, want it with two events", code, body)
	}
	opened, dismissed := events[0].(map[string]any), events[1].(map[string]any)
	if opened["kind"] != "opened" || opened["run_id"] == nil || opened["evidence"].(map[string]any)["/jobs/rotate/overdue"] != float64(3) {
		t.Errorf("first event %v, want the opening sighting with its run and evidence", opened)
	}
	if dismissed["kind"] != "dismissed" || dismissed["note"] != "accepted" || dismissed["operator_user_id"] == nil {
		t.Errorf("second event %v, want the operator's dismissal with the note", dismissed)
	}
}

func idsOf(list any) []string {
	items, _ := list.([]any)
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if row, ok := item.(map[string]any); ok {
			id, _ := row["id"].(string)
			ids = append(ids, id)
		}
	}
	return ids
}
