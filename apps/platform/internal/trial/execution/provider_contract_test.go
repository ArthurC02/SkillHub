// RUN-009: the provider contract test suite.
//
// One suite, two targets. By default it runs against the in-repo fake
// (internal/run/providertest), which is what makes it part of `go test ./...`.
// Point SKILLHUB_PROVIDER_CONTRACT_URL at a real provider and the same assertions
// run against that instead:
//
//	SKILLHUB_PROVIDER_CONTRACT_URL=http://localhost:9000 \
//	SKILLHUB_PROVIDER_CONTRACT_TOKEN=... \
//	go test ./internal/run -run TestProviderContract -v
//
// ADR-004 asks for exactly this: "使用 Fake Provider 通過完整生命週期契約測試" and
// "SelfHostedProvider 與 LocalRunnerProvider 使用同一組核心測試". A provider that
// passes here is one the orchestrator can drive; it says nothing about whether that
// provider isolates anything (ADR-015, SEC-009).
//
// External test package on purpose: it may only use what a provider implementor
// can use, which is the exported client and nothing else.
package run_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/providertest"
)

const (
	contractURLEnv   = "SKILLHUB_PROVIDER_CONTRACT_URL"
	contractTokenEnv = "SKILLHUB_PROVIDER_CONTRACT_TOKEN"
)

// target is the provider under test plus its base URL, which a couple of the
// assertions need directly because the client deliberately cannot express them
// (there is no way to ask for active=false through it, which is itself the point).
type target struct {
	provider *run.Provider
	baseURL  string
	token    string
	// profile is the runtime this target actually declares. Negotiated rather than
	// hardcoded: a provider refuses a version it does not have (422), so a suite
	// that assumed one would only ever pass against the fake it was written next
	// to. This is the same resolution the scheduler does (run.Match).
	profile run.RuntimeProfile
}

func newTarget(t *testing.T) target {
	t.Helper()
	tg := target{provider: providertest.New("contract_fake", "test-token").Provider()}
	if url := os.Getenv(contractURLEnv); url != "" {
		token := os.Getenv(contractTokenEnv)
		t.Logf("running the contract suite against %s", url)
		tg = target{provider: run.NewProvider("contract_target", url, token), baseURL: url, token: token}
	} else {
		fake := providertest.New("contract_fake", "test-token")
		t.Cleanup(fake.Close)
		tg = target{provider: fake.Provider(), baseURL: fake.URL, token: fake.Token}
	}
	tg.profile = negotiate(t, tg.provider)
	return tg
}

// negotiate reads the target's capability and picks a runtime it will accept.
func negotiate(t *testing.T, p *run.Provider) run.RuntimeProfile {
	t.Helper()
	capability, err := p.Capability(context.Background())
	if err != nil {
		t.Fatalf("GET /capability: %v", err)
	}
	if len(capability.Runtimes) == 0 || len(capability.Runtimes[0].Versions) == 0 {
		t.Fatal("the target declares no runtime, so nothing can be dispatched to it")
	}
	rt := capability.Runtimes[0]
	profile := run.RuntimeProfile{
		Runtime:        rt.Runtime,
		RuntimeVersion: rt.Versions[len(rt.Versions)-1],
	}
	if len(rt.AgentIntegration) > 0 {
		profile.AgentIntegration = rt.AgentIntegration[0]
	}
	return profile
}

// request builds a minimal valid RunRequest for a fresh (run_id, attempt) pair.
// The ids are random so the suite can be run repeatedly against a live provider
// that remembers what it was told last time.
func (tg target) request(prompt string) run.RunRequest {
	runID, attemptID := randomUUID(), randomUUID()
	return run.RunRequest{
		RunID: runID, RunAttemptID: attemptID, Attempt: 1,
		WorkspaceID:    randomUUID(),
		IdempotencyKey: attemptID,
		SkillVersion:   run.PackageRef{SkillVersionID: randomUUID(), ContentHash: "sha256:" + strings.Repeat("0", 64)},
		TestCaseSnapshot: run.TestCaseSnapshotRef{
			TestCaseSnapshotID: randomUUID(),
			ContentHash:        "sha256:" + strings.Repeat("1", 64),
			UserPrompt:         prompt,
		},
		Runtime:        tg.profile,
		ResourceLimits: run.DefaultResourceLimits(),
		Egress:         run.EgressPolicy{Mode: "default_deny"},
		Trace:          run.TracePolicy{Level: "standard"},
	}
}

// dispatch creates a run and registers its teardown, so a live provider is not
// left holding sandboxes after the suite.
func (tg target) dispatch(t *testing.T, req run.RunRequest) run.ProviderRun {
	t.Helper()
	pr, err := tg.provider.CreateRun(context.Background(), req)
	if err != nil {
		t.Fatalf("POST /runs: %v", err)
	}
	t.Cleanup(func() { _ = tg.provider.Destroy(context.Background(), pr.ProviderRunID) })
	return pr
}

func TestProviderContract(t *testing.T) {
	tg := newTarget(t)
	ctx := context.Background()

	// GET /capability — read before dispatch, and answered in a shape the
	// scheduler can match against.
	t.Run("capability is readable and names its runtimes", func(t *testing.T) {
		capability, err := tg.provider.Capability(ctx)
		if err != nil {
			t.Fatalf("GET /capability: %v", err)
		}
		if capability.Provider == "" {
			t.Error("capability does not name the provider")
		}
		if len(capability.Runtimes) == 0 {
			t.Error("capability declares no runtime, so nothing could ever be dispatched to it")
		}
		if capability.Isolation.Level == "" {
			t.Error("capability declares no isolation level")
		}
	})

	// The idempotency guarantee dispatch retries are built on.
	t.Run("re-sending one run_id and attempt returns the same run", func(t *testing.T) {
		req := tg.request("idempotent dispatch")
		first := tg.dispatch(t, req)
		if first.ProviderRunID == "" {
			t.Fatal("the provider created a run without naming a handle for it")
		}
		if first.RunID != req.RunID || first.RunAttemptID != req.RunAttemptID {
			t.Error("the provider did not echo the platform identifiers back")
		}

		second, err := tg.provider.CreateRun(ctx, req)
		if err != nil {
			t.Fatalf("re-sending the same request: %v", err)
		}
		if second.ProviderRunID != first.ProviderRunID {
			t.Errorf("a re-send started a second sandbox: %q then %q",
				first.ProviderRunID, second.ProviderRunID)
		}
	})

	t.Run("re-sending with different content is a conflict", func(t *testing.T) {
		req := tg.request("original prompt")
		tg.dispatch(t, req)

		changed := req
		changed.TestCaseSnapshot.UserPrompt = "a different prompt entirely"
		_, err := tg.provider.CreateRun(ctx, changed)
		if err == nil {
			t.Fatal("a superseded attempt was served the first body's run instead of a conflict")
		}
		if !strings.Contains(err.Error(), "409") {
			t.Errorf("error = %v, want a 409", err)
		}
	})

	// A cancel that races the run finishing must not be an error the worker has to
	// special-case: it polls on an interval, so that race is ordinary.
	t.Run("cancel is accepted and stays accepted when the run is terminal", func(t *testing.T) {
		pr := tg.dispatch(t, tg.request("cancel me"))
		if _, err := tg.provider.Cancel(ctx, pr.ProviderRunID); err != nil {
			t.Fatalf("first cancel: %v", err)
		}
		// Poll until it is actually down, then cancel again.
		waitForTerminal(t, tg.provider, pr.ProviderRunID)
		if _, err := tg.provider.Cancel(ctx, pr.ProviderRunID); err != nil {
			t.Errorf("cancelling an already-terminal run: %v, want it accepted", err)
		}
	})

	// The property the whole cleanup path rests on (iron rule 9).
	t.Run("destroy is repeatable and never a 404", func(t *testing.T) {
		pr := tg.dispatch(t, tg.request("destroy me"))
		for i := range 3 {
			if err := tg.provider.Destroy(ctx, pr.ProviderRunID); err != nil {
				t.Fatalf("destroy #%d: %v", i+1, err)
			}
		}
		// A handle that never existed is already in the state the caller wants.
		if err := tg.provider.Destroy(ctx, "handle-that-never-existed"); err != nil {
			t.Errorf("destroying an unknown handle: %v, want it accepted", err)
		}
	})

	t.Run("terminal runs carry a result and running ones do not", func(t *testing.T) {
		pr := tg.dispatch(t, tg.request("run to completion"))
		final := waitForTerminal(t, tg.provider, pr.ProviderRunID)
		if final.Result == nil {
			t.Fatal("a terminal run carries no result, so the platform cannot classify it")
		}
		switch final.Result.Status {
		case "succeeded", "failed", "cancelled", "timed_out":
		default:
			t.Errorf("result status = %q, which is not one of the four terminal outcomes", final.Result.Status)
		}
		if final.ObservedAt.IsZero() {
			t.Error("no observed_at, so two answers cannot be ordered")
		}
	})

	// The orphan scan reads this, and an unbounded or wrongly-shaped answer would
	// make it destroy the wrong things.
	t.Run("active listing is served and dated", func(t *testing.T) {
		pr := tg.dispatch(t, tg.request("stay listed"))
		list, err := tg.provider.ListActive(ctx)
		if err != nil {
			t.Fatalf("GET /runs?active=true: %v", err)
		}
		if list.ObservedAt.IsZero() {
			t.Fatal("the listing has no observed_at, so a fresh sandbox could be judged leaked")
		}
		var found bool
		for _, entry := range list.Runs {
			if entry.ProviderRunID == pr.ProviderRunID {
				found = true
				if entry.Result != nil {
					t.Error("listings must omit the result; read the single run for it")
				}
			}
		}
		if !found {
			t.Error("a live run is missing from the active listing")
		}
	})

	t.Run("active=false is refused rather than answered empty", func(t *testing.T) {
		resp := tg.raw(t, http.MethodGet, "/runs?active=false")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("GET /runs?active=false: got %d, want 400", resp.StatusCode)
		}
	})

	t.Run("requests without the provider token are refused", func(t *testing.T) {
		if tg.token == "" {
			t.Skip("target requires no token")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(tg.baseURL, "/")+"/capability", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("unauthenticated GET /capability: got %d, want 401", resp.StatusCode)
		}
	})
}

// The other half of "does this provider work with this platform": the endpoints
// can be perfect and the scheduler still refuse to dispatch to it. This runs the
// real capability answer through the real matcher with the platform's own default
// policy, which is precisely the check that caught apps/sandbox declaring
// `egress_modes: ["none"]` — every endpoint green, and not one run dispatchable.
func TestSchedulerAcceptsTheTargetsRealCapability(t *testing.T) {
	// The in-repo fake declares `container` — the honest name for what a developer
	// machine runs — and the scheduler now accepts that only from a deployment that
	// has declared itself an offline development one (ADR-020, ADR-015). This suite
	// is that deployment. A real target pointed at by SKILLHUB_PROVIDER_CONTRACT_URL
	// declares its own level and this changes nothing for it.
	t.Setenv("DEV_LOGIN", "1")
	tg := newTarget(t)
	capability, err := tg.provider.Capability(context.Background())
	if err != nil {
		t.Fatalf("GET /capability: %v", err)
	}
	profile, err := run.Match(capability, run.DefaultRequirements())
	if err != nil {
		t.Fatalf("the scheduler will not dispatch to this provider: %v", err)
	}
	if profile.RuntimeVersion == "" {
		t.Error("matching resolved no runtime version to pin onto the run")
	}
}

// A 422 is the provider refusing the request itself, and the platform must be able
// to tell it from a transient failure without reading message strings. Only the
// fake can be made to produce one on demand.
func TestProviderRefusalIsClassifiedAndNotRetried(t *testing.T) {
	if os.Getenv(contractURLEnv) != "" {
		t.Skip("a live provider cannot be told to refuse on demand")
	}
	fake := providertest.New("refusing_fake", "test-token")
	defer fake.Close()

	tg := target{provider: fake.Provider(), baseURL: fake.URL, token: fake.Token}
	tg.profile = negotiate(t, tg.provider)
	fake.DispatchStatuses = []int{http.StatusUnprocessableEntity}
	_, err := tg.provider.CreateRun(context.Background(), tg.request("refuse me"))
	if err == nil {
		t.Fatal("a 422 was reported as a successful dispatch")
	}
	if !strings.Contains(err.Error(), "capability_mismatch") {
		t.Errorf("error = %v, want it to carry the RunError class", err)
	}
	if fake.Dispatches() != 0 {
		t.Errorf("the provider created %d sandboxes while refusing the request", fake.Dispatches())
	}
}

// --- helpers ------------------------------------------------------------------

func (tg target) raw(t *testing.T, method, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, strings.TrimSuffix(tg.baseURL, "/")+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if tg.token != "" {
		req.Header.Set("Authorization", "Bearer "+tg.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func waitForTerminal(t *testing.T, p *run.Provider, handle string) run.ProviderRun {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(30 * time.Second)
	var last run.ProviderRun
	for time.Now().Before(deadline) {
		pr, err := p.GetRun(ctx, handle)
		if err != nil {
			t.Fatalf("GET /runs/{id}: %v", err)
		}
		last = pr
		if pr.State.Terminal() {
			return pr
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("run never reached a terminal state; last was %q", last.State)
	return last
}

func randomUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
