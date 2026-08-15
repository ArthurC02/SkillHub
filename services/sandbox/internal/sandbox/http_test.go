package sandbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/services/sandbox/internal/sandbox"
)

const testToken = "test-provider-token"

// fakeDriver stands in for Docker. Only the container work is faked: the
// idempotency key, the state machine, the wall clock and destroy semantics
// under test are the real ones from Manager.
type fakeDriver struct {
	mu        sync.Mutex
	starts    map[string]int
	stops     map[string]time.Duration
	removes   map[string]int
	release   map[string]chan sandbox.Outcome
	startErr  error
	removeErr error
}

func newFakeDriver() *fakeDriver {
	return &fakeDriver{
		starts:  map[string]int{},
		stops:   map[string]time.Duration{},
		removes: map[string]int{},
		release: map[string]chan sandbox.Outcome{},
	}
}

func (f *fakeDriver) Start(_ context.Context, id string, _ sandbox.RunRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts[id]++
	if f.startErr != nil {
		return f.startErr
	}
	f.release[id] = make(chan sandbox.Outcome, 1)
	return nil
}

func (f *fakeDriver) Wait(ctx context.Context, id string) (sandbox.Outcome, error) {
	f.mu.Lock()
	ch := f.release[id]
	f.mu.Unlock()
	select {
	case out := <-ch:
		return out, nil
	case <-ctx.Done():
		return sandbox.Outcome{}, ctx.Err()
	}
}

// Stop mimics the runtime: the workload goes down, so the pending Wait returns
// with the exit code a killed process leaves behind.
func (f *fakeDriver) Stop(_ context.Context, id string, grace time.Duration) error {
	f.mu.Lock()
	f.stops[id] = grace
	ch := f.release[id]
	f.mu.Unlock()
	select {
	case ch <- sandbox.Outcome{ExitCode: 137}:
	default:
	}
	return nil
}

func (f *fakeDriver) Remove(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removes[id]++
	return f.removeErr
}

func (f *fakeDriver) Adopt(context.Context) ([]sandbox.Adopted, error) { return nil, nil }
func (f *fakeDriver) Healthy(context.Context) bool                     { return true }

func (f *fakeDriver) exit(id string, out sandbox.Outcome) {
	f.mu.Lock()
	ch := f.release[id]
	f.mu.Unlock()
	ch <- out
}

func (f *fakeDriver) count(m map[string]int, id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return m[id]
}

func newServer(t *testing.T) (*fakeDriver, http.Handler) {
	t.Helper()
	drv := newFakeDriver()
	m := sandbox.NewManager(drv, sandbox.Config{
		Provider:       "docker_dev",
		Runtimes:       []sandbox.RuntimeCapability{{Runtime: "claude_agent_sdk", Versions: []string{"0.3.233"}, AgentIntegration: []string{"in_sandbox_sdk"}}},
		MaxResources:   sandbox.DefaultLimits,
		IsolationLevel: "container",
		EgressModes:    []string{"none"},
		Slots:          2,
	}, slog.New(slog.DiscardHandler))
	return drv, (&sandbox.Server{M: m, Token: testToken}).Routes()
}

func runRequest() sandbox.RunRequest {
	return sandbox.RunRequest{
		RunID:        "11111111-1111-1111-1111-111111111111",
		RunAttemptID: "22222222-2222-2222-2222-222222222222",
		Attempt:      1,
		WorkspaceID:  "33333333-3333-3333-3333-333333333333",
		SkillVersion: sandbox.PackageRef{SkillVersionID: "44444444-4444-4444-4444-444444444444", ContentHash: "sha256:abc"},
		TestCase: sandbox.TestCaseSnapshotRef{
			SnapshotID: "55555555-5555-5555-5555-555555555555", ContentHash: "sha256:def",
			UserPrompt: "use the skill to summarise the dataset",
		},
		Runtime:        sandbox.RuntimeProfile{Runtime: "claude_agent_sdk", RuntimeVersion: "0.3.233", AgentIntegration: "in_sandbox_sdk"},
		ResourceLimits: sandbox.DefaultLimits,
		Egress:         sandbox.EgressPolicy{Mode: "default_deny"},
		Trace:          sandbox.TracePolicy{Level: "standard"},
	}
}

func do(t *testing.T, h http.Handler, method, path string, body any, token string) (*httptest.ResponseRecorder, sandbox.ProviderRun) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var run sandbox.ProviderRun
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &run)
	}
	return rec, run
}

// Idempotency is what makes a dispatch safe to retry when the worker cannot
// tell whether the first one arrived (ADR-004). The second call must not open a
// second sandbox — the assertion on starts is the whole point of the test.
func TestCreateIsIdempotentOnRunIDAndAttempt(t *testing.T) {
	drv, h := newServer(t)
	req := runRequest()

	rec, first := do(t, h, "POST", "/runs", req, testToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first dispatch: got %d, want 201 (body %s)", rec.Code, rec.Body)
	}
	rec, second := do(t, h, "POST", "/runs", req, testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("re-dispatch: got %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	if first.ProviderRunID != second.ProviderRunID {
		t.Errorf("re-dispatch returned a different handle: %s vs %s", first.ProviderRunID, second.ProviderRunID)
	}
	if n := drv.count(drv.starts, first.ProviderRunID); n != 1 {
		t.Errorf("driver started %d sandboxes for one (run_id, attempt), want 1", n)
	}
}

// A repeat carrying different content is a caller bug; serving the first body's
// run would make a superseded attempt look dispatched.
func TestCreateRejectsSameKeyWithDifferentContent(t *testing.T) {
	_, h := newServer(t)
	req := runRequest()
	if rec, _ := do(t, h, "POST", "/runs", req, testToken); rec.Code != http.StatusCreated {
		t.Fatalf("first dispatch: got %d, want 201", rec.Code)
	}

	changed := runRequest()
	changed.TestCase.UserPrompt = "a different prompt entirely"
	rec, _ := do(t, h, "POST", "/runs", changed, testToken)
	if rec.Code != http.StatusConflict {
		t.Fatalf("changed re-dispatch: got %d, want 409 (body %s)", rec.Code, rec.Body)
	}
}

// A different attempt of the same run is a different key and gets its own
// sandbox: retries must not collapse into the first attempt.
func TestCreateTreatsEachAttemptSeparately(t *testing.T) {
	_, h := newServer(t)
	first := runRequest()
	second := runRequest()
	second.Attempt = 2
	second.RunAttemptID = "66666666-6666-6666-6666-666666666666"

	_, a := do(t, h, "POST", "/runs", first, testToken)
	rec, b := do(t, h, "POST", "/runs", second, testToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("second attempt: got %d, want 201", rec.Code)
	}
	if a.ProviderRunID == b.ProviderRunID {
		t.Error("attempt 2 reused attempt 1's sandbox")
	}
}

// The caller polls on an interval and a run can finish between the read and the
// cancel; making that race an error would turn a timing window into a failure
// the worker has to special-case.
func TestCancelOnTerminalRunStillAnswers202(t *testing.T) {
	drv, h := newServer(t)
	_, run := do(t, h, "POST", "/runs", runRequest(), testToken)
	drv.exit(run.ProviderRunID, sandbox.Outcome{ExitCode: 0})
	waitForTerminal(t, h, run.ProviderRunID)

	rec, cancelled := do(t, h, "POST", "/runs/"+run.ProviderRunID+"/cancel", nil, testToken)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("cancel of a finished run: got %d, want 202", rec.Code)
	}
	if cancelled.State != sandbox.StateCompleted {
		t.Errorf("cancel rewrote a terminal state to %s", cancelled.State)
	}
}

func TestCancelRunningRunReachesCancelled(t *testing.T) {
	_, h := newServer(t)
	_, run := do(t, h, "POST", "/runs", runRequest(), testToken)

	rec, accepted := do(t, h, "POST", "/runs/"+run.ProviderRunID+"/cancel", nil, testToken)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("cancel: got %d, want 202", rec.Code)
	}
	if accepted.CancelRequestedAt.IsZero() {
		t.Error("cancel_requested_at not recorded")
	}
	final := waitForTerminal(t, h, run.ProviderRunID)
	if final.State != sandbox.StateCancelled {
		t.Errorf("state after cancel = %s, want cancelled", final.State)
	}
	if final.Result == nil || final.Result.Status != sandbox.ResultCancelled {
		t.Errorf("result after cancel = %+v, want status cancelled", final.Result)
	}
}

// Destroy has no 404 and no second-call failure: a worker that crashed
// mid-cleanup has to be able to retry it (iron rule 9).
func TestDestroyIsIdempotentAndHasNo404(t *testing.T) {
	drv, h := newServer(t)
	_, run := do(t, h, "POST", "/runs", runRequest(), testToken)

	for i := range 2 {
		rec, _ := do(t, h, "DELETE", "/runs/"+run.ProviderRunID, nil, testToken)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("destroy #%d: got %d, want 204", i+1, rec.Code)
		}
	}
	if n := drv.count(drv.removes, run.ProviderRunID); n != 2 {
		t.Errorf("driver saw %d removes, want 2: the repeat must still reach the runtime", n)
	}
	rec, _ := do(t, h, "DELETE", "/runs/never-existed", nil, testToken)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("destroy of an unknown handle: got %d, want 204", rec.Code)
	}
	// The run is gone from the provider's records, so reading it is a 404.
	if rec, _ := do(t, h, "GET", "/runs/"+run.ProviderRunID, nil, testToken); rec.Code != http.StatusNotFound {
		t.Errorf("read after destroy: got %d, want 404", rec.Code)
	}
}

func TestDestroyReports500WhenResourcesAreStillHeld(t *testing.T) {
	drv, h := newServer(t)
	_, run := do(t, h, "POST", "/runs", runRequest(), testToken)
	drv.mu.Lock()
	drv.removeErr = errRemove{}
	drv.mu.Unlock()

	rec, _ := do(t, h, "DELETE", "/runs/"+run.ProviderRunID, nil, testToken)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("failed destroy: got %d, want 500 so the platform records the cleanup failure", rec.Code)
	}
}

type errRemove struct{}

func (errRemove) Error() string { return "still held" }

// Every route is behind the token, including the ones that only read.
func TestEveryRouteRefusesWithoutTheProviderToken(t *testing.T) {
	_, h := newServer(t)
	_, run := do(t, h, "POST", "/runs", runRequest(), testToken)

	for _, tc := range []struct{ method, path string }{
		{"GET", "/capability"},
		{"POST", "/runs"},
		{"GET", "/runs"},
		{"GET", "/runs/" + run.ProviderRunID},
		{"DELETE", "/runs/" + run.ProviderRunID},
		{"POST", "/runs/" + run.ProviderRunID + "/cancel"},
	} {
		for _, token := range []string{"", "wrong-token"} {
			rec, _ := do(t, h, tc.method, tc.path, nil, token)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s with token %q: got %d, want 401", tc.method, tc.path, token, rec.Code)
			}
		}
	}
}

// 422, not 400: a capability refusal is classified so the platform can tell it
// from a transient failure without parsing message strings.
func TestCreateRefusesLimitsItCannotEnforce(t *testing.T) {
	_, h := newServer(t)
	req := runRequest()
	req.ResourceLimits.MemoryBytes = sandbox.DefaultLimits.MemoryBytes * 4

	rec, _ := do(t, h, "POST", "/runs", req, testToken)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("oversized memory: got %d, want 422", rec.Code)
	}
	var re sandbox.RunError
	if err := json.Unmarshal(rec.Body.Bytes(), &re); err != nil || re.Class != sandbox.ClassCapabilityMismatch {
		t.Errorf("body = %s, want a RunError with class capability_mismatch", rec.Body)
	}
}

func TestCreateRefusesUnknownRuntime(t *testing.T) {
	_, h := newServer(t)
	req := runRequest()
	req.Runtime.Runtime = "some_other_harness"
	if rec, _ := do(t, h, "POST", "/runs", req, testToken); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown runtime: got %d, want 422", rec.Code)
	}
}

func TestCreateRefusesWhenSlotsAreFull(t *testing.T) {
	_, h := newServer(t)
	for i := 1; i <= 2; i++ {
		req := runRequest()
		req.Attempt = i
		if rec, _ := do(t, h, "POST", "/runs", req, testToken); rec.Code != http.StatusCreated {
			t.Fatalf("dispatch %d: got %d, want 201", i, rec.Code)
		}
	}
	req := runRequest()
	req.Attempt = 3
	if rec, _ := do(t, h, "POST", "/runs", req, testToken); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third dispatch into two slots: got %d, want 429", rec.Code)
	}
}

func TestListServesOnlyActiveTrue(t *testing.T) {
	_, h := newServer(t)
	_, run := do(t, h, "POST", "/runs", runRequest(), testToken)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/runs?active=true", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	h.ServeHTTP(rec, req)
	var list sandbox.ProviderRunList
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Runs) != 1 || list.Runs[0].ProviderRunID != run.ProviderRunID {
		t.Fatalf("list = %+v, want the one live run", list.Runs)
	}
	if list.ObservedAt.IsZero() {
		t.Error("observed_at missing: an orphan scan cannot order the snapshot without it")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/runs?active=false", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("active=false: got %d, want 400", rec.Code)
	}
}

// `result` is present exactly when the state is terminal, and never in a
// listing.
func TestResultAppearsOnlyOnTerminalSingleReads(t *testing.T) {
	drv, h := newServer(t)
	_, run := do(t, h, "POST", "/runs", runRequest(), testToken)

	if _, live := do(t, h, "GET", "/runs/"+run.ProviderRunID, nil, testToken); live.Result != nil {
		t.Error("a running attempt carried a result")
	}
	drv.exit(run.ProviderRunID, sandbox.Outcome{ExitCode: 0})
	final := waitForTerminal(t, h, run.ProviderRunID)
	if final.Result == nil || final.Result.Status != sandbox.ResultSucceeded {
		t.Fatalf("terminal read = %+v, want a succeeded result", final.Result)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/runs", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	h.ServeHTTP(rec, req)
	var list sandbox.ProviderRunList
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list.Runs) != 1 || list.Runs[0].Result != nil {
		t.Errorf("listing carried a result: %+v", list.Runs)
	}
}

// A workload that ran to its own end and reported failure is `completed` with
// result status `failed` — not `failed`, which means the provider could not
// carry the attempt through.
func TestNonZeroExitIsCompletedWithFailedResult(t *testing.T) {
	drv, h := newServer(t)
	_, run := do(t, h, "POST", "/runs", runRequest(), testToken)
	drv.exit(run.ProviderRunID, sandbox.Outcome{ExitCode: 3})

	final := waitForTerminal(t, h, run.ProviderRunID)
	if final.State != sandbox.StateCompleted {
		t.Errorf("state = %s, want completed", final.State)
	}
	if final.Result.Status != sandbox.ResultFailed {
		t.Errorf("result status = %s, want failed", final.Result.Status)
	}
}

// The wall clock is the provider enforcing a limit against the attempt: state
// failed, result timed_out (RUN-004).
func TestWallClockStopsTheRunAsFailedTimedOut(t *testing.T) {
	drv, h := newServer(t)
	req := runRequest()
	req.ResourceLimits.WallClockSoftSeconds = 1
	req.ResourceLimits.WallClockHardSeconds = 2

	_, run := do(t, h, "POST", "/runs", req, testToken)
	final := waitForTerminal(t, h, run.ProviderRunID)
	if final.State != sandbox.StateFailed {
		t.Errorf("state = %s, want failed", final.State)
	}
	if final.Result.Status != sandbox.ResultTimedOut {
		t.Errorf("result status = %s, want timed_out", final.Result.Status)
	}
	if final.Result.Error == nil || final.Result.Error.Class != sandbox.ClassTimeout {
		t.Errorf("error = %+v, want class timeout", final.Result.Error)
	}
	// The grace window handed to the runtime is the soft-to-hard gap: the time
	// PDM-005 5.2 reserves for a cooperative stop to collect artifacts.
	drv.mu.Lock()
	grace := drv.stops[run.ProviderRunID]
	drv.mu.Unlock()
	if grace != time.Second {
		t.Errorf("stop grace = %s, want 1s (hard minus soft)", grace)
	}
}

// A provision failure is the provider's own: state failed, class provision, and
// no sandbox left behind for the caller to guess about.
func TestStartFailureIsAProvisionError(t *testing.T) {
	drv, h := newServer(t)
	drv.startErr = errRemove{}

	rec, run := do(t, h, "POST", "/runs", runRequest(), testToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d, want 201: the attempt exists and its failure is readable", rec.Code)
	}
	if run.State != sandbox.StateFailed || run.Result == nil || run.Result.Error.Class != sandbox.ClassProvision {
		t.Fatalf("run = %+v, want failed with class provision", run)
	}
}

// Secrets this provider injected must not come back out through the workload's
// own output (iron rule 11).
func TestWorkloadOutputIsScrubbedOfInjectedSecrets(t *testing.T) {
	drv, h := newServer(t)
	req := runRequest()
	req.ModelGateway = &sandbox.ModelGatewayGrant{BaseURL: "http://gateway:4000", VirtualKey: "sk-run-supersecret"}
	req.ObjectGrants = []sandbox.ObjectGrant{{
		Purpose: "skill_package", ObjectKey: "k", Access: "read",
		URL: "https://store.example/pkg?sig=deadbeef",
	}}

	_, run := do(t, h, "POST", "/runs", req, testToken)
	drv.exit(run.ProviderRunID, sandbox.Outcome{
		ExitCode: 0,
		Output:   "key=sk-run-supersecret url=https://store.example/pkg?sig=deadbeef",
	})
	final := waitForTerminal(t, h, run.ProviderRunID)
	got := final.Result.AgentOutput
	if got != "key=*** url=***" {
		t.Errorf("agent_output = %q, want both injected secrets masked", got)
	}
}

func TestCreateRejectsMalformedBody(t *testing.T) {
	_, h := newServer(t)
	req := httptest.NewRequest("POST", "/runs", bytes.NewReader([]byte("{not json")))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed body: got %d, want 400", rec.Code)
	}

	incomplete := runRequest()
	incomplete.TestCase.UserPrompt = ""
	if rec, _ := do(t, h, "POST", "/runs", incomplete, testToken); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing prompt: got %d, want 400", rec.Code)
	}
}

func waitForTerminal(t *testing.T, h http.Handler, id string) sandbox.ProviderRun {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, run := do(t, h, "GET", "/runs/"+id, nil, testToken)
		if run.State.Terminal() {
			return run
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run %s never reached a terminal state", id)
	return sandbox.ProviderRun{}
}
