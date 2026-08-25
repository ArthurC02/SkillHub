package run

// PDM-005 5.2a-4: the Go worker is the token ceiling's only enforcement point.
//
// The counter that used to be the only one lives inside the workload it bounds
// (infra/images/runtime-agent-sdk/run.mjs), holding that workload's own gateway
// credential, so a skill that goes around the harness was bounded by nothing but
// the Virtual Key's max_budget - about 2.4M input tokens at cached prices, eight
// times what the user confirmed. These tests are about the ceiling that sits on
// the other side of the credential.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// spendLogStub is a stand-in for the gateway's /spend/logs/v2, paginating a list
// of per-call token counts the way LiteLLM does.
type spendLogStub struct {
	calls [][2]int // {prompt_tokens, completion_tokens} per model call
	// gotAlias is what the platform asked about, which is the whole reason the
	// answer belongs to one attempt and not to the fleet.
	gotAlias string
	requests int
}

func (s *spendLogStub) start(t *testing.T) *Gateway {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests++
		s.gotAlias = r.URL.Query().Get("key_alias")
		size, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		if size <= 0 {
			size = len(s.calls)
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page <= 0 {
			page = 1
		}
		totalPages := (len(s.calls) + size - 1) / size
		rows := []map[string]int{}
		for i := (page - 1) * size; i < len(s.calls) && i < page*size; i++ {
			rows = append(rows, map[string]int{
				"prompt_tokens": s.calls[i][0], "completion_tokens": s.calls[i][1],
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": rows, "total_pages": totalPages})
	}))
	t.Cleanup(srv.Close)
	return &Gateway{AdminBaseURL: srv.URL, adminKey: "sk-master-test", HTTP: srv.Client()}
}

func TestAttemptTokensSumsWhatTheGatewayBilledThisAttempt(t *testing.T) {
	// Three model calls, the shape §5.2a-2 measured: a small auxiliary call and
	// two full-prefix ones.
	stub := &spendLogStub{calls: [][2]int{{420, 12}, {19_215, 300}, {19_415, 250}}}
	g := stub.start(t)

	used, err := g.AttemptTokens(context.Background(), "attempt-1", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if used.InputTokens != 39_050 {
		t.Errorf("input tokens = %d, want 39050 summed across the attempt's calls", used.InputTokens)
	}
	if used.OutputTokens != 562 {
		t.Errorf("output tokens = %d, want 562 summed across the attempt's calls", used.OutputTokens)
	}
	// Scoped to one attempt by the same alias Revoke uses, so the answer needs
	// nothing that was held in memory before a restart.
	if stub.gotAlias != keyAlias("attempt-1") {
		t.Errorf("key_alias = %q, want the attempt-derived alias", stub.gotAlias)
	}
}

// A workload that made more calls than one page holds is exactly the one the
// ceiling is for, so the sum must not stop at the first page.
func TestAttemptTokensFollowsThePagesTheGatewayReports(t *testing.T) {
	calls := make([][2]int, usagePageSize+40)
	for i := range calls {
		calls[i] = [2]int{1_000, 100}
	}
	stub := &spendLogStub{calls: calls}
	g := stub.start(t)

	used, err := g.AttemptTokens(context.Background(), "attempt-1", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if used.InputTokens != len(calls)*1_000 {
		t.Errorf("input tokens = %d, want %d - a second page of calls was dropped",
			used.InputTokens, len(calls)*1_000)
	}
	if stub.requests != 2 {
		t.Errorf("spend log reads = %d, want 2 pages", stub.requests)
	}
}

// driverWithCeiling is a driver with nothing behind it but the gateway: the
// breach decision reads the run's frozen snapshot and the gateway, and no
// database at all.
func driverWithCeiling(t *testing.T, g *Gateway, maxInput, maxOutput int) *driver {
	t.Helper()
	limits := DefaultResourceLimits()
	limits.TokenBudget.MaxInputTokens = maxInput
	limits.TokenBudget.MaxOutputTokens = maxOutput
	snapshot, err := json.Marshal(policySnapshot{ResourceLimits: limits})
	if err != nil {
		t.Fatal(err)
	}
	return &driver{svc: &Service{Gateway: g}, cur: gen.Run{PolicySnapshot: snapshot}}
}

func anAttempt(t *testing.T) gen.RunAttempt {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan("11111111-1111-4111-8111-111111111111"); err != nil {
		t.Fatal(err)
	}
	return gen.RunAttempt{ID: id, CreatedAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true}}
}

func TestARunPastItsTokenCeilingIsStoppedAndToldWhy(t *testing.T) {
	// 300K of input across sixteen calls: inside the budget the key would allow,
	// past the number the user confirmed. That gap is the defect.
	calls := make([][2]int, 16)
	for i := range calls {
		calls[i] = [2]int{19_400, 500}
	}
	d := driverWithCeiling(t, (&spendLogStub{calls: calls}).start(t), 300_000, 60_000)

	reason := d.tokenCeilingBreach(context.Background(), anAttempt(t))
	if reason == "" {
		t.Fatal("a run 310400 input tokens into a 300000 ceiling was allowed to continue")
	}
	// The user is told which limit stopped their run, not that something failed.
	if !containsAll(reason, "token ceiling", "310400", "300000") {
		t.Errorf("reason = %q, want it to name the token ceiling and both numbers", reason)
	}
}

// The output half of the same ceiling (60K, PDM-005 5.2), which is where the
// cost of a run actually sits once caching has made input nearly free (5.2a-6).
func TestARunPastItsOutputCeilingIsStoppedToo(t *testing.T) {
	d := driverWithCeiling(t, (&spendLogStub{calls: [][2]int{{1_000, 60_001}}}).start(t), 300_000, 60_000)
	if reason := d.tokenCeilingBreach(context.Background(), anAttempt(t)); reason == "" {
		t.Fatal("a run past its output ceiling was allowed to continue")
	}
}

func TestARunInsideItsTokenCeilingIsLeftAlone(t *testing.T) {
	// Right up against it and not over: the boundary is the interesting case,
	// because a ceiling that stops the last allowed run is a ceiling that lies to
	// the permission summary in the other direction.
	d := driverWithCeiling(t, (&spendLogStub{calls: [][2]int{{300_000, 60_000}}}).start(t), 300_000, 60_000)
	if reason := d.tokenCeilingBreach(context.Background(), anAttempt(t)); reason != "" {
		t.Fatalf("a run exactly at its ceiling was stopped: %q", reason)
	}
}

// A gateway that cannot be read is not evidence that a run misbehaved, and the
// wall clock still bounds it. The warning that goes with this is the part that
// keeps the gap from being silent.
func TestAnUnreadableGatewayDoesNotKillAHealthyRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	d := driverWithCeiling(t, &Gateway{AdminBaseURL: srv.URL, adminKey: "k", HTTP: srv.Client()}, 300_000, 60_000)
	if reason := d.tokenCeilingBreach(context.Background(), anAttempt(t)); reason != "" {
		t.Fatalf("a broken gateway management API terminated a run: %q", reason)
	}
}

// A deployment with no gateway hands the sandbox no credential and no route out,
// so there is nothing to count and nothing to ask.
func TestNoGatewayMeansNoCeilingToEnforce(t *testing.T) {
	d := driverWithCeiling(t, nil, 300_000, 60_000)
	if reason := d.tokenCeilingBreach(context.Background(), anAttempt(t)); reason != "" {
		t.Fatalf("a deployment with no model gateway stopped a run over tokens: %q", reason)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}
