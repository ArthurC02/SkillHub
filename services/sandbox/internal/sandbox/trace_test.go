package sandbox_test

// TRACE-002 on the provider side: the collector reads what the workload wrote
// and pushes it, and the two things that must hold under a flaky network are
// that nothing is lost and nothing is skipped.

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/services/sandbox/internal/sandbox"
)

// recordingSink stands in for the platform's ingestion endpoint. failNext makes
// one push fail, which is the case the at-least-once contract exists for.
type recordingSink struct {
	mu       sync.Mutex
	batches  [][]json.RawMessage
	failNext int
}

func (s *recordingSink) Push(_ context.Context, _ string, events []json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failNext > 0 {
		s.failNext--
		return errors.New("ingestion unavailable")
	}
	batch := make([]json.RawMessage, len(events))
	copy(batch, events)
	s.batches = append(s.batches, batch)
	return nil
}

func (s *recordingSink) received() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, batch := range s.batches {
		for _, e := range batch {
			out = append(out, string(e))
		}
	}
	return out
}

func event(seq int) string {
	return `{"schema_version":"1.0","event_id":"0f0a1e6c-1c9a-4f8e-9a2b-1d5a2c7b3e0` +
		string(rune('0'+seq)) + `","seq":` + string(rune('0'+seq)) + `,"type":"agent_output"}`
}

// newTracingServer is newServer with collection turned on.
func newTracingServer(t *testing.T, sink sandbox.TraceSink) (*fakeDriver, http.Handler) {
	t.Helper()
	drv := newFakeDriver()
	m := sandbox.NewManager(drv, sandbox.Config{
		Provider:       "docker_dev",
		Runtimes:       []sandbox.RuntimeCapability{{Runtime: "claude_agent_sdk", Versions: []string{"0.3.233"}, AgentIntegration: []string{"in_sandbox_sdk"}}},
		MaxResources:   sandbox.DefaultLimits,
		IsolationLevel: "container",
		EgressModes:    []string{"none"},
		Slots:          2,
	}, slog.New(slog.DiscardHandler)).WithTrace(sink, nil)
	return drv, (&sandbox.Server{M: m, Token: testToken}).Routes()
}

func tracedRequest() sandbox.RunRequest {
	req := runRequest()
	req.Trace = sandbox.TracePolicy{Level: "standard", IngestionURL: "http://platform/internal/trace/tok.1.99.sig"}
	return req
}

// The final drain is the one that matters most: it runs after the workload has
// exited but before DELETE removes the container, which is the only moment the
// tail of the trace can still be read out of the /out tmpfs.
func TestCollectorPushesTheTailAfterTheWorkloadExits(t *testing.T) {
	sink := &recordingSink{}
	drv, h := newTracingServer(t, sink)

	_, run := do(t, h, "POST", "/runs", tracedRequest(), testToken)
	drv.writeTrace(run.ProviderRunID, event(1), event(2))
	drv.exit(run.ProviderRunID, sandbox.Outcome{ExitCode: 0})

	waitFor(t, func() bool { return len(sink.received()) == 2 })
}

// A push that fails must not advance the high-water mark, or the events in that
// batch would be silently dropped. Re-sending them is safe because the platform
// dedupes on event_id (TRACE-008).
func TestFailedPushIsRetriedRatherThanSkipped(t *testing.T) {
	sink := &recordingSink{failNext: 1}
	drv, h := newTracingServer(t, sink)

	_, run := do(t, h, "POST", "/runs", tracedRequest(), testToken)
	drv.writeTrace(run.ProviderRunID, event(1))
	drv.exit(run.ProviderRunID, sandbox.Outcome{ExitCode: 0})

	waitFor(t, func() bool {
		got := sink.received()
		return len(got) == 1 && got[0] == event(1)
	})
}

// A half-written last line is normal: the workload appends while the collector
// reads. Sending it would fail validation at the far end and, worse, would mark
// it as sent so the complete version never went.
func TestPartialTrailingLineIsHeldBackUntilComplete(t *testing.T) {
	sink := &recordingSink{}
	drv, h := newTracingServer(t, sink)

	_, run := do(t, h, "POST", "/runs", tracedRequest(), testToken)
	// One whole line plus a fragment with no newline terminator.
	drv.writeTrace(run.ProviderRunID, event(1))
	drv.appendRawTrace(run.ProviderRunID, `{"schema_version":"1.0","ev`)
	drv.exit(run.ProviderRunID, sandbox.Outcome{ExitCode: 0})

	waitFor(t, func() bool { return len(sink.received()) == 1 })
	if got := sink.received(); got[0] != event(1) {
		t.Errorf("pushed %q, want only the complete line", got[0])
	}
}

// A run with no ingestion URL is a run nothing is collecting. It must not fail,
// and it must not push anywhere.
func TestNoIngestionURLCollectsNothing(t *testing.T) {
	sink := &recordingSink{}
	drv, h := newTracingServer(t, sink)

	_, run := do(t, h, "POST", "/runs", runRequest(), testToken) // no ingestion_url
	drv.writeTrace(run.ProviderRunID, event(1))
	drv.exit(run.ProviderRunID, sandbox.Outcome{ExitCode: 0})

	time.Sleep(100 * time.Millisecond)
	if got := sink.received(); len(got) != 0 {
		t.Errorf("pushed %d events with no destination configured", len(got))
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was never met")
}
