package run

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type spendLogStub struct {
	calls  [][2]int // [prompt_tokens, completion_tokens] per call
	spends []float64

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
		rows := []map[string]any{}
		for i := (page - 1) * size; i < len(s.calls) && i < page*size; i++ {
			row := map[string]any{"prompt_tokens": s.calls[i][0], "completion_tokens": s.calls[i][1]}
			if s.spends != nil {
				row["spend"] = s.spends[i]
			}
			rows = append(rows, row)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": rows, "total_pages": totalPages})
	}))
	t.Cleanup(srv.Close)
	return &Gateway{AdminBaseURL: srv.URL, adminKey: "sk-master-test", HTTP: srv.Client()}
}

func TestAttemptTokensSumsWhatTheGatewayBilledThisAttempt(t *testing.T) {

	stub := &spendLogStub{calls: [][2]int{{420, 12}, {19_215, 300}, {19_415, 250}}}
	g := stub.start(t)

	used, err := g.AttemptUsage(context.Background(), "attempt-1", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if used.InputTokens != 39_050 {
		t.Errorf("input tokens = %d, want 39050 summed across the attempt's calls", used.InputTokens)
	}
	if used.OutputTokens != 562 {
		t.Errorf("output tokens = %d, want 562 summed across the attempt's calls", used.OutputTokens)
	}

	if stub.gotAlias != keyAlias("attempt-1") {
		t.Errorf("key_alias = %q, want the attempt-derived alias", stub.gotAlias)
	}
}

func TestAttemptUsageSumsSpendAndSaysWhetherAnyWasReported(t *testing.T) {
	priced := (&spendLogStub{calls: [][2]int{{420, 12}, {19_215, 300}}, spends: []float64{0.0012, 0.037}}).start(t)
	used, err := priced.AttemptUsage(context.Background(), "attempt-1", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !used.SpendReported || math.Abs(used.SpendUSD-0.0382) > 1e-9 {
		t.Errorf("spend = %v reported = %v, want 0.0382 / true", used.SpendUSD, used.SpendReported)
	}

	unpriced := (&spendLogStub{calls: [][2]int{{420, 12}}}).start(t)
	used, err = unpriced.AttemptUsage(context.Background(), "attempt-1", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if used.SpendReported || used.SpendUSD != 0 {
		t.Errorf("spend = %v reported = %v, want 0 / false for rows without a spend field", used.SpendUSD, used.SpendReported)
	}
}

func TestAttemptTokensFollowsThePagesTheGatewayReports(t *testing.T) {
	calls := make([][2]int, usagePageSize+40)
	for i := range calls {
		calls[i] = [2]int{1_000, 100}
	}
	stub := &spendLogStub{calls: calls}
	g := stub.start(t)

	used, err := g.AttemptUsage(context.Background(), "attempt-1", time.Now().Add(-time.Hour))
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

	calls := make([][2]int, 16)
	for i := range calls {
		calls[i] = [2]int{19_400, 500}
	}
	d := driverWithCeiling(t, (&spendLogStub{calls: calls}).start(t), 300_000, 60_000)

	reason := d.tokenCeilingBreach(context.Background(), anAttempt(t))
	if reason == "" {
		t.Fatal("a run 310400 input tokens into a 300000 ceiling was allowed to continue")
	}

	if !containsAll(reason, "token ceiling", "310400", "300000") {
		t.Errorf("reason = %q, want it to name the token ceiling and both numbers", reason)
	}
}

func TestARunPastItsOutputCeilingIsStoppedToo(t *testing.T) {
	d := driverWithCeiling(t, (&spendLogStub{calls: [][2]int{{1_000, 60_001}}}).start(t), 300_000, 60_000)
	if reason := d.tokenCeilingBreach(context.Background(), anAttempt(t)); reason == "" {
		t.Fatal("a run past its output ceiling was allowed to continue")
	}
}

func TestARunInsideItsTokenCeilingIsLeftAlone(t *testing.T) {

	d := driverWithCeiling(t, (&spendLogStub{calls: [][2]int{{300_000, 60_000}}}).start(t), 300_000, 60_000)
	if reason := d.tokenCeilingBreach(context.Background(), anAttempt(t)); reason != "" {
		t.Fatalf("a run exactly at its ceiling was stopped: %q", reason)
	}
}

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
