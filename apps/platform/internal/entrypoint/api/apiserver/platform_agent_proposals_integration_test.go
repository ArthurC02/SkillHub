package apiserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	analytics "github.com/ArthurC02/skillhub/apps/platform/internal/product/learning"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/operations"
)

type proposalWorld struct {
	*findingWorld
	actions []operations.Action
}

func newProposalWorld(t *testing.T, name string, actions ...operations.Action) *proposalWorld {
	t.Helper()
	w := newFindingWorld(t, name)
	w.def.Proposals = operations.DailyReportProposals
	for _, a := range actions {
		w.def.Actions = append(w.def.Actions, a.Name)
	}
	t.Cleanup(func() {
		if _, err := testPool.Exec(context.Background(),
			"DELETE FROM platform_agent_proposals WHERE agent_id IN (SELECT id FROM platform_agents WHERE name = $1)", name); err != nil {
			t.Errorf("cleanup proposals: %v", err)
		}
	})
	return &proposalWorld{findingWorld: w, actions: actions}
}

func fixedPreview(counts ...operations.PreviewCount) func(context.Context) (operations.Preview, error) {
	return func(context.Context) (operations.Preview, error) {
		return operations.Preview{Counts: counts, BatchLimit: 100}, nil
	}
}

func proposal(action, reason string, cites ...string) string {
	encoded, _ := json.Marshal(map[string]any{"action": action, "reason": reason, "cites": cites})
	return string(encoded)
}

func (w *proposalWorld) propose(proposals ...string) operations.RunReport {
	w.t.Helper()
	result := `{"items":[` + attention("purge is late", "/jobs/purge/overdue") + `],"proposals":[` + joinComma(proposals) + `]}`
	facts := operations.Tool{
		Name: "maintenance_report", Description: "facts", Parameters: map[string]any{"type": "object"},
		Run: func(context.Context, json.RawMessage) (any, error) { return json.RawMessage(findingFacts), nil },
	}
	s := &loopScript{t: w.t, answers: answers(intent("maintenance_report"), final(result))}
	runner := s.runner(w.svc)
	runner.Actions = w.actions
	report, err := runner.Run(context.Background(), w.def, []operations.Tool{facts}, loopLimits)
	if err != nil {
		w.t.Fatalf("run: %v", err)
	}
	return report
}

type storedProposal struct {
	id      pgtype.UUID
	action  string
	tier    string
	status  string
	preview operations.Preview
	life    time.Duration
}

func (w *proposalWorld) proposals() []storedProposal {
	w.t.Helper()
	rows, err := testPool.Query(context.Background(), `SELECT p.id, p.action, p.tier, p.status, p.preview, extract(epoch FROM p.expires_at - p.proposed_at)::float8
		FROM platform_agent_proposals p JOIN platform_agents a ON a.id = p.agent_id
		WHERE a.name = $1 ORDER BY p.proposed_at, p.action`, w.def.Name)
	if err != nil {
		w.t.Fatal(err)
	}
	defer rows.Close()
	var out []storedProposal
	for rows.Next() {
		var p storedProposal
		var preview []byte
		var life float64
		if err := rows.Scan(&p.id, &p.action, &p.tier, &p.status, &preview, &life); err != nil {
			w.t.Fatal(err)
		}
		if err := json.Unmarshal(preview, &p.preview); err != nil {
			w.t.Fatal(err)
		}
		p.life = time.Duration(life * float64(time.Second))
		out = append(out, p)
	}
	return out
}

func (w *proposalWorld) decide(id pgtype.UUID, decision, note string) (int, map[string]any) {
	w.t.Helper()
	body, _ := json.Marshal(map[string]string{"decision": decision, "note": note})
	return operatorCall(w.t, w.operator, http.MethodPut, "/admin/agents/proposals/"+uuidString(id)+"/decision", string(body))
}

func TestAnAgentProposesOnlyARegisteredActionWithItsPreviewAndOnceWhileItIsLive(t *testing.T) {
	audit := operations.Action{Name: "run-test-proposal-audit", Tier: operations.TierDestructive,
		Preview: fixedPreview(operations.PreviewCount{Key: "audit_events_past_retention", Count: 42})}
	w := newProposalWorld(t, "agent-proposals-propose", audit)

	if report := w.propose(proposal(audit.Name, "purge is two periods late", "/jobs/purge/overdue")); report.Status != operations.RunCompleted {
		t.Fatalf("run %+v, want completed", report)
	}
	got := w.proposals()
	if len(got) != 1 || got[0].status != "proposed" || got[0].tier != "destructive" || got[0].life != 24*time.Hour {
		t.Fatalf("proposals %+v, want one destructive proposal waiting 24 hours", got)
	}
	if c := got[0].preview.Counts; len(c) != 1 || c[0].Key != "audit_events_past_retention" || c[0].Count != 42 || got[0].preview.BatchLimit != 100 {
		t.Errorf("preview %+v, want the action's own counts and batch limit", got[0].preview)
	}
	if n := countRow(t, testPool, "SELECT count(*) FROM audit_events WHERE resource_id = $1 AND action = 'platform_agent_proposal.proposed' AND actor_agent_id IS NOT NULL", got[0].id); n != 1 {
		t.Errorf("proposal audit events by the agent: %d, want 1", n)
	}

	w.propose(proposal(audit.Name, "still late", "/jobs/purge/overdue"))
	if got := w.proposals(); len(got) != 1 {
		t.Errorf("after a second report: %d proposals, want the live one kept and no second", len(got))
	}
}

func TestARunThatProposesBeyondWhatItMayEndsFailedAndProposesNothing(t *testing.T) {
	allowed := operations.Action{Name: "run-test-proposal-allowed", Tier: operations.TierDestructive, Preview: fixedPreview()}
	broken := operations.Action{Name: "run-test-proposal-broken", Tier: operations.TierDestructive,
		Preview: func(context.Context) (operations.Preview, error) {
			return operations.Preview{}, errors.New("count failed")
		}}
	w := newProposalWorld(t, "agent-proposals-refused", allowed, broken)
	unregistered := "run-test-proposal-unregistered"
	w.def.Actions = append(w.def.Actions, unregistered)

	cases := []struct {
		name, action, reason string
	}{
		{"an action the agent is not allowed", "run-test-proposal-forbidden", "it is not allowed to propose"},
		{"an allowed action nobody registered", unregistered, "no action named"},
		{"an action whose preview fails", broken.Name, "count failed"},
	}
	for _, c := range cases {
		report := w.propose(proposal(allowed.Name, "fine", "/jobs/purge/overdue"), proposal(c.action, "why", "/jobs/purge/overdue"))
		if report.Status != operations.RunFailed || !strings.Contains(report.Reason, c.reason) {
			t.Errorf("%s: run %s %q, want failed naming %q", c.name, report.Status, report.Reason, c.reason)
		}
	}
	if got := w.proposals(); len(got) != 0 {
		t.Errorf("proposals %+v, want none from a failed run", got)
	}
}

func TestAnOperatorDecidesAWaitingProposalOnceAndNotAfterItExpires(t *testing.T) {
	first := operations.Action{Name: "run-test-proposal-first", Tier: operations.TierDestructive, Preview: fixedPreview()}
	second := operations.Action{Name: "run-test-proposal-second", Tier: operations.TierReversible, Preview: fixedPreview()}
	third := operations.Action{Name: "run-test-proposal-third", Tier: operations.TierDestructive, Preview: fixedPreview()}
	w := newProposalWorld(t, "agent-proposals-decide", first, second, third)
	w.propose(proposal(first.Name, "a", "/jobs/purge/overdue"), proposal(second.Name, "b", "/jobs/rotate/overdue"),
		proposal(third.Name, "c", "/budget_days"))
	got := w.proposals()
	ids := map[string]pgtype.UUID{}
	for _, p := range got {
		ids[p.action] = p.id
	}

	steps := []struct {
		action, decision, note string
		want                   int
	}{
		{first.Name, "approve", "  ", http.StatusBadRequest},
		{first.Name, "run", "now", http.StatusBadRequest},
		{first.Name, "approve", "the job is two periods late", http.StatusNoContent},
		{first.Name, "approve", "again", http.StatusConflict},
		{first.Name, "reject", "changed my mind", http.StatusConflict},
		{second.Name, "reject", "rotation runs tonight anyway", http.StatusNoContent},
	}
	for _, s := range steps {
		if code, body := w.decide(ids[s.action], s.decision, s.note); code != s.want {
			t.Fatalf("%s %s %q: %d %v, want %d", s.decision, s.action, s.note, code, body, s.want)
		}
	}
	if code, _ := w.decide(pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, "approve", "x"); code != http.StatusNotFound {
		t.Errorf("unknown proposal: %d, want 404", code)
	}
	assertDecided(t, w, ids[first.Name], "approved", false)
	assertDecided(t, w, ids[second.Name], "rejected", true)

	if _, err := testPool.Exec(context.Background(),
		"UPDATE platform_agent_proposals SET proposed_at = now() - interval '2 days', expires_at = now() - interval '1 second' WHERE id = $1",
		ids[third.Name]); err != nil {
		t.Fatal(err)
	}
	if code, _ := w.decide(ids[third.Name], "approve", "too late"); code != http.StatusConflict {
		t.Errorf("deciding a lapsed proposal: %d, want 409", code)
	}
	if n, err := w.svc.ExpireProposals(context.Background()); err != nil || n < 1 {
		t.Fatalf("expire: %d, %v", n, err)
	}
	assertDecided(t, w, ids[third.Name], "expired", true)
	if n := countRow(t, testPool, "SELECT count(*) FROM audit_events WHERE resource_id = $1 AND action = 'platform_agent_proposal.expired' AND actor_user_id IS NULL AND actor_agent_id IS NULL", ids[third.Name]); n != 1 {
		t.Errorf("system expiry audit events: %d, want 1", n)
	}
}

func assertDecided(t *testing.T, w *proposalWorld, id pgtype.UUID, status string, finished bool) {
	t.Helper()
	code, body := operatorCall(t, w.operator, http.MethodGet, "/admin/agents/proposals/"+uuidString(id), "")
	if code != http.StatusOK || body["status"] != status {
		t.Fatalf("proposal: %d %v, want status %s", code, body, status)
	}
	if _, has := body["finished_at"]; has != finished {
		t.Errorf("%s proposal finished_at present = %v, want %v", status, has, finished)
	}
	if status == "expired" {
		return
	}
	if body["decision_note"] == "" || body["decided_by_user_id"] == nil {
		t.Errorf("%s proposal %v, want the operator and their note", status, body)
	}
}

func TestTheMaintenanceRunnerTakesEachApprovedProposalOnceAndRecordsItsOutcome(t *testing.T) {
	ok := operations.Action{Name: "run-test-proposal-ok", Tier: operations.TierDestructive, Preview: fixedPreview()}
	bad := operations.Action{Name: "run-test-proposal-bad", Tier: operations.TierDestructive, Preview: fixedPreview()}
	w := newProposalWorld(t, "agent-proposals-run", ok, bad)
	w.propose(proposal(ok.Name, "a", "/jobs/purge/overdue"), proposal(bad.Name, "b", "/jobs/rotate/overdue"))
	for _, p := range w.proposals() {
		if code, _ := w.decide(p.id, "approve", "go ahead"); code != http.StatusNoContent {
			t.Fatalf("approve %s: %d", p.action, code)
		}
	}

	ctx := context.Background()
	outcomes := map[string]error{ok.Name: nil, bad.Name: errors.New("the job stopped part way")}
	claimed := map[string]pgtype.UUID{}
	for range 2 {
		p, found, err := w.svc.ClaimApprovedProposal(ctx)
		if err != nil || !found {
			t.Fatalf("claim: %v %v, want an approved proposal", found, err)
		}
		claimed[p.Action] = p.ID
		if err := w.svc.FinishProposal(ctx, p.ID, outcomes[p.Action]); err != nil {
			t.Fatal(err)
		}
	}
	if _, found, err := w.svc.ClaimApprovedProposal(ctx); err != nil || found {
		t.Errorf("third claim: %v %v, want nothing left", found, err)
	}
	if err := w.svc.FinishProposal(ctx, claimed[ok.Name], nil); !errors.Is(err, operations.ErrProposalClosed) {
		t.Errorf("finishing twice: %v, want ErrProposalClosed", err)
	}
	_, body := operatorCall(t, w.operator, http.MethodGet, "/admin/agents/proposals/"+uuidString(claimed[ok.Name]), "")
	if body["status"] != "succeeded" || body["started_at"] == nil {
		t.Errorf("successful proposal %v, want succeeded with its start", body)
	}
	_, body = operatorCall(t, w.operator, http.MethodGet, "/admin/agents/proposals/"+uuidString(claimed[bad.Name]), "")
	if body["status"] != "failed" || body["outcome"] != "the job stopped part way" {
		t.Errorf("failed proposal %v, want failed with the error", body)
	}
	if n := countRow(t, testPool, "SELECT count(*) FROM audit_events WHERE resource_id = ANY($1) AND action IN ('platform_agent_proposal.succeeded', 'platform_agent_proposal.failed')",
		[]pgtype.UUID{claimed[ok.Name], claimed[bad.Name]}); n != 2 {
		t.Errorf("outcome audit events: %d, want one each", n)
	}
}

func TestOperatorsListLiveAndRecentlyClosedProposals(t *testing.T) {
	live := operations.Action{Name: "run-test-proposal-live", Tier: operations.TierDestructive, Preview: fixedPreview()}
	old := operations.Action{Name: "run-test-proposal-old", Tier: operations.TierDestructive, Preview: fixedPreview()}
	w := newProposalWorld(t, "agent-proposals-list", live, old)
	w.propose(proposal(live.Name, "a", "/jobs/purge/overdue"), proposal(old.Name, "b", "/jobs/rotate/overdue"))
	ids := map[string]pgtype.UUID{}
	for _, p := range w.proposals() {
		ids[p.action] = p.id
	}
	if code, _ := w.decide(ids[old.Name], "reject", "no"); code != http.StatusNoContent {
		t.Fatal("reject")
	}
	code, body := operatorCall(t, w.operator, http.MethodGet, "/admin/agents/proposals", "")
	listed := idsOf(body["proposals"])
	if code != http.StatusOK || !containsAll(listed, uuidString(ids[live.Name]), uuidString(ids[old.Name])) {
		t.Fatalf("list %d %v, want the waiting one and the one rejected today", code, listed)
	}
	if _, err := testPool.Exec(context.Background(),
		"UPDATE platform_agent_proposals SET proposed_at = now() - interval '9 days', expires_at = now() - interval '8 days', decided_at = now() - interval '8 days', finished_at = now() - interval '8 days' WHERE id = $1",
		ids[old.Name]); err != nil {
		t.Fatal(err)
	}
	_, body = operatorCall(t, w.operator, http.MethodGet, "/admin/agents/proposals", "")
	if listed := idsOf(body["proposals"]); containsAll(listed, uuidString(ids[old.Name])) || !containsAll(listed, uuidString(ids[live.Name])) {
		t.Errorf("list %v, want the live one and not one closed eight days ago", listed)
	}
}

func containsAll(list []string, want ...string) bool {
	for _, w := range want {
		found := false
		for _, item := range list {
			found = found || item == w
		}
		if !found {
			return false
		}
	}
	return true
}

func TestEveryMaintenanceActionPreviewsAgainstTheDatabase(t *testing.T) {
	for key, value := range map[string]string{
		"AUDIT_RETENTION": "2160h", "FEEDBACK_RETENTION": "2160h", "TRACE_RETENTION": "2160h",
		"ANALYTICS_RETENTION": "2160h", "SKILL_DELETION_GRACE": "720h", "PURGE_GRACE": "",
		"MAINTENANCE_BATCH": "37",
	} {
		t.Setenv(key, value)
	}
	actions := wiring.MaintenanceActions(testPool)
	if len(actions) != len(operations.ProposableMaintenanceJobs) {
		t.Fatalf("%d actions for %d proposable jobs", len(actions), len(operations.ProposableMaintenanceJobs))
	}
	for _, def := range operations.Definitions() {
		for _, name := range def.Actions {
			if !containsAll(actionNames(actions), name) {
				t.Errorf("%s may propose %s, which is not registered", def.Name, name)
			}
		}
	}
	for _, a := range actions {
		preview, err := a.Preview(context.Background())
		if err != nil || len(preview.Counts) == 0 {
			t.Errorf("%s preview: %+v, %v", a.Name, preview, err)
		}
		job, _ := operations.MaintenanceJobOf(a.Name)
		batched := job != operations.JobPurgeAudit && job != operations.JobPurgeFeedback && job != operations.JobRotatePartitions
		if batched != (preview.BatchLimit == 37) {
			t.Errorf("%s batch limit %d, want 37 only for a job that handles a batch per run", a.Name, preview.BatchLimit)
		}
	}
}

func TestARetentionPreviewCountsWhatThePurgeThenRemoves(t *testing.T) {
	ctx := context.Background()
	const retention = 90 * 24 * time.Hour
	for _, stmt := range []string{
		"INSERT INTO feedback_reports (kind, message, created_at) VALUES ('need_signal', 'old', now() - interval '100 days'), ('need_signal', 'old', now() - interval '91 days'), ('need_signal', 'new', now())",
		"INSERT INTO audit_events (action, resource_type, created_at) VALUES ('test.preview', 'test', now() - interval '100 days'), ('test.preview', 'test', now())",
	} {
		if _, err := testPool.Exec(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}

	feedback := &analytics.Service{Pool: testPool}
	counted, err := feedback.CountExpiredFeedback(ctx, retention)
	if err != nil || counted < 2 {
		t.Fatalf("feedback preview %d, %v, want at least the two old reports", counted, err)
	}
	if purged, err := feedback.PurgeExpiredFeedback(ctx, retention); err != nil || purged != counted {
		t.Errorf("feedback purge removed %d (%v), the preview said %d", purged, err, counted)
	}

	counted, err = audit.CountExpired(ctx, testPool, retention)
	if err != nil || counted < 1 {
		t.Fatalf("audit preview %d, %v, want at least the old event", counted, err)
	}
	if purged, err := audit.PurgeExpired(ctx, testPool, retention); err != nil || purged != counted {
		t.Errorf("audit purge removed %d (%v), the preview said %d", purged, err, counted)
	}
}

func actionNames(actions []operations.Action) []string {
	names := make([]string, len(actions))
	for i, a := range actions {
		names[i] = a.Name
	}
	return names
}
