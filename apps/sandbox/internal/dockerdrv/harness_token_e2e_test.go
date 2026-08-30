package dockerdrv_test

// The runtime harness's two token duties, asked of a real container talking to a
// real gateway: report the run's usage however the turn ends (TRACE-004), and
// stop the run at the ceiling it was given (PDM-005 5.2a).
//
// Neither is testable without a model. A fake stream would only prove that the
// code we wrote does what we wrote, and the bug this replaces was precisely an
// assumption about what the SDK emits: usage was emitted only on the `result`
// message, and a real turn that produced none (measured, add-iso3166) reported
// no cost at all. So this test spends money, and is gated on the two variables
// that say a gateway is there to spend it at:
//
//	SKILLHUB_E2E_GATEWAY_URL   the gateway address *as the container sees it*
//	                           (on the dev egress network, http://litellm:4000)
//	SKILLHUB_E2E_GATEWAY_KEY   a key that may call it
//	SKILLHUB_E2E_EGRESS_NETWORK  the docker network both are on (skillhub_egress)
//	SKILLHUB_E2E_RUNTIME_IMAGE   defaults to skillhub/runtime-agent-sdk:2026.08-2
//
// The image must be built from the working tree, not pulled: what is under test
// is run.mjs, and a tag left over from an earlier build fails these assertions
// while looking like a product bug.
//
//	docker build -t skillhub/runtime-agent-sdk:2026.08-2 infra/images/runtime-agent-sdk
//
// Two turns of a trivial prompt: a few cents at the mini tier.

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	neturl "net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/dockerdrv"
	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

type traceEvent struct {
	SchemaVersion string `json:"schema_version"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	Payload       struct {
		// usage
		InputTokens  int64    `json:"input_tokens"`
		OutputTokens int64    `json:"output_tokens"`
		CostUSD      *float64 `json:"cost_usd"`
		CostSource   *string  `json:"cost_source"`
		TokenSource  *string  `json:"token_source"`
		// error
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"payload"`
}

func gatewayHarness(t *testing.T) (*sandbox.Manager, sandbox.RunRequest, *collector) {
	t.Helper()
	url, key := os.Getenv("SKILLHUB_E2E_GATEWAY_URL"), os.Getenv("SKILLHUB_E2E_GATEWAY_KEY")
	if url == "" || key == "" {
		t.Skip("SKILLHUB_E2E_GATEWAY_URL / _KEY not set: this test calls a real model")
	}
	network := os.Getenv("SKILLHUB_E2E_EGRESS_NETWORK")
	if network == "" {
		network = "skillhub_egress"
	}
	image := os.Getenv("SKILLHUB_E2E_RUNTIME_IMAGE")
	if image == "" {
		image = "skillhub/runtime-agent-sdk:2026.08-2"
	}

	d, err := dockerdrv.New(dockerdrv.Config{
		Image:       image,
		Network:     network,
		UID:         65532,
		GID:         65532,
		ExtraLabels: map[string]string{testLabel: "1"},
	})
	if err != nil {
		t.Skipf("no docker: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	// The node has to render the destination it is about to be asked for, or
	// accept() refuses the dispatch before any container starts (ADR-022 A1-e).
	// Left out until 2026-08-30, which made both tests in this file unrunnable
	// from the day A1-e landed — and invisibly so, because without the two
	// gateway variables they skip and a skip looks like a pass. Derived from the
	// URL under test rather than hard-coded, so a gateway addressed by IP is
	// described by the same rule as one addressed by name.
	gwURL, err := neturl.Parse(url)
	if err != nil {
		t.Fatalf("SKILLHUB_E2E_GATEWAY_URL %q does not parse: %v", url, err)
	}
	gwHost, gwPortStr, err := net.SplitHostPort(gwURL.Host)
	if err != nil {
		t.Fatalf("SKILLHUB_E2E_GATEWAY_URL %q must carry an explicit port: %v", url, err)
	}
	gwPort, err := strconv.Atoi(gwPortStr)
	if err != nil {
		t.Fatalf("SKILLHUB_E2E_GATEWAY_URL %q has a non-numeric port: %v", url, err)
	}
	dest := sandbox.EgressDestination{Purpose: "model_gateway", Port: gwPort, Protocol: "tcp"}
	if net.ParseIP(gwHost) != nil {
		dest.PinnedIP = gwHost
	} else {
		dest.FQDN = gwHost
	}

	m := sandbox.NewManager(d, sandbox.Config{
		Provider:       "docker_dev",
		Runtimes:       []sandbox.RuntimeCapability{{Runtime: "claude_agent_sdk", Versions: []string{"0.3.233"}, AgentIntegration: []string{"in_sandbox_sdk"}}},
		MaxResources:   sandbox.DefaultLimits,
		IsolationLevel: "container",
		EgressModes:    []string{"default_deny"},
		EgressAllow:    []sandbox.EgressDestination{dest},
		Slots:          1,
	}, slog.New(slog.DiscardHandler)).WithTrace(&sandbox.HTTPTraceSink{}, nil)

	// /out is a tmpfs and the container is gone soon after it exits, so the only
	// way to see what the harness wrote is the same one production uses: the
	// node drains the file while the sandbox lives and pushes it here.
	sink := newCollector()
	t.Cleanup(sink.Close)

	req := testRequest("")
	delete(req.Extensions, "dev_cmd") // run the image's own entrypoint, not a shell
	req.TestCase.UserPrompt = "Reply with the single word DONE and nothing else."
	req.ResourceLimits.MemoryBytes = 2 << 30
	req.ResourceLimits.DiskBytes = 2 << 30
	req.ResourceLimits.MaxPIDs = 256
	req.ResourceLimits.WallClockSoftSeconds = 180
	req.ResourceLimits.WallClockHardSeconds = 240
	req.Runtime.Model = "gpt-5.4-mini"
	req.Egress = sandbox.EgressPolicy{
		Mode:  "default_deny",
		Allow: []sandbox.EgressAllowEntry{{Purpose: "model_gateway", URL: url}},
	}
	req.ModelGateway = &sandbox.ModelGatewayGrant{BaseURL: url, VirtualKey: key}
	req.Trace = sandbox.TracePolicy{
		Level:        "standard",
		IngestionURL: sink.URL + "/internal/trace/11111111-1111-1111-1111-111111111111.1.99999999999.deadbeefcafe",
	}
	return m, req, sink
}

// runToEnd drives one attempt to its terminal state and returns the result plus
// every trace event that reached the collector, including the closing push the
// node makes after the container exits.
func runToEnd(t *testing.T, m *sandbox.Manager, req sandbox.RunRequest, sink *collector) (sandbox.ProviderRun, []traceEvent) {
	t.Helper()
	ctx := context.Background()
	run, created, err := m.Create(ctx, req)
	if err != nil || !created {
		t.Fatalf("create: %v (created=%v)", err, created)
	}
	t.Cleanup(func() { _ = m.Destroy(context.Background(), run.ProviderRunID) })

	var final sandbox.ProviderRun
	deadline := time.Now().Add(6 * time.Minute)
	for time.Now().Before(deadline) {
		final, err = m.Get(run.ProviderRunID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if final.State.Terminal() {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !final.State.Terminal() {
		t.Fatal("the run never reached a terminal state")
	}

	// The usage event is the last thing the harness writes, so waiting for it is
	// waiting for the closing push. Bounded: if it never arrives, that is the
	// failure this test exists to catch, and the assertions say so.
	settle := time.Now().Add(45 * time.Second)
	var events []traceEvent
	for {
		events = decodeEvents(t, sink)
		if findEvent(events, "usage") != nil || time.Now().After(settle) {
			return final, events
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func decodeEvents(t *testing.T, sink *collector) []traceEvent {
	t.Helper()
	sink.mu.Lock()
	defer sink.mu.Unlock()
	events := make([]traceEvent, 0, len(sink.events))
	for _, raw := range sink.events {
		var e traceEvent
		if err := json.Unmarshal(raw, &e); err != nil {
			t.Fatalf("trace line is not an event: %v (%s)", err, raw)
		}
		events = append(events, e)
	}
	return events
}

func findEvent(events []traceEvent, typ string) *traceEvent {
	for i := range events {
		if events[i].Type == typ {
			return &events[i]
		}
	}
	return nil
}

// The ordinary path: a turn that reaches its result reports usage from the SDK's
// own total, with cost from the gateway.
func TestHarnessReportsUsageForACompletedTurn(t *testing.T) {
	m, req, sink := gatewayHarness(t)

	final, events := runToEnd(t, m, req, sink)
	if final.Result == nil || final.Result.Status != sandbox.ResultSucceeded {
		t.Fatalf("result = %+v, want succeeded", final.Result)
	}
	usage := findEvent(events, "usage")
	if usage == nil {
		t.Fatal("no usage event: the run's cost is invisible to the platform")
	}
	if usage.Payload.InputTokens <= 0 {
		t.Errorf("input_tokens = %d, want the turn's real count", usage.Payload.InputTokens)
	}
	if usage.Payload.TokenSource == nil {
		t.Error("token_source is absent: a consumer cannot tell an SDK total from a running sum")
	}
	if usage.SchemaVersion != "1.1" {
		t.Errorf("schema_version = %q, want 1.1 (the version that defines token_source)", usage.SchemaVersion)
	}
	t.Logf("usage: in=%d out=%d token_source=%v cost=%v/%v",
		usage.Payload.InputTokens, usage.Payload.OutputTokens,
		deref(usage.Payload.TokenSource), derefF(usage.Payload.CostUSD), deref(usage.Payload.CostSource))
}

// The ceiling path, which is also the no-result path: the harness stops the turn
// itself, so there is no `result` message, and usage still has to be reported -
// from the running sum - or the run's cost vanishes exactly as it did for
// add-iso3166.
func TestHarnessStopsAtTheTokenCeilingAndStillReportsUsage(t *testing.T) {
	m, req, sink := gatewayHarness(t)
	// One token: the first response crosses it, so the turn stops after the
	// response that was already paid for and before any further call.
	req.ResourceLimits.TokenBudget = &sandbox.TokenBudget{MaxInputTokens: 1, MaxOutputTokens: 1}

	final, events := runToEnd(t, m, req, sink)

	// Completed, not failed: the workload stopped itself cleanly, so the attempt
	// is collectable and the platform classifies it as a run that ran.
	if final.State != sandbox.StateCompleted {
		t.Errorf("state = %s, want completed", final.State)
	}
	if final.Result == nil || final.Result.Status != sandbox.ResultFailed {
		t.Fatalf("result = %+v, want status failed", final.Result)
	}
	if final.Result.Error == nil || final.Result.Error.Message == "" {
		t.Fatal("no RunError: the run's own limit stopped it and said nothing")
	}
	t.Logf("provider error: %s", final.Result.Error.Message)

	errEvent := findEvent(events, "error")
	if errEvent == nil || errEvent.Payload.Code != "token_budget_exceeded" {
		t.Fatalf("error event = %+v, want code token_budget_exceeded", errEvent)
	}
	usage := findEvent(events, "usage")
	if usage == nil {
		t.Fatal("no usage event on the ceiling path: this is the TRACE-004 hole reopening")
	}
	if usage.Payload.TokenSource == nil || *usage.Payload.TokenSource != "accumulated" {
		t.Errorf("token_source = %v, want accumulated", deref(usage.Payload.TokenSource))
	}
	if usage.Payload.InputTokens <= 0 {
		t.Errorf("input_tokens = %d: the tokens that were spent before the stop were not counted",
			usage.Payload.InputTokens)
	}
	t.Logf("stopped at in=%d out=%d cost=%v", usage.Payload.InputTokens, usage.Payload.OutputTokens,
		derefF(usage.Payload.CostUSD))

	// Recorded for tools/contracts/validate_trace_events.py: real 1.1 output from
	// the path that used to produce no usage event at all. See
	// contracts/events/samples/README.md.
	if out := os.Getenv("SKILLHUB_TRACE_SAMPLE_OUT"); out != "" {
		sink.mu.Lock()
		defer sink.mu.Unlock()
		var b strings.Builder
		for _, raw := range sink.events {
			b.Write(raw)
			b.WriteString("\n")
		}
		if err := os.WriteFile(out, []byte(b.String()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func derefF(f *float64) any {
	if f == nil {
		return "<nil>"
	}
	return *f
}
