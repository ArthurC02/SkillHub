package sandbox_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	neturl "net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

type recordingSink struct {
	mu       sync.Mutex
	batches  [][]json.RawMessage
	failNext int
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type trackingBody struct {
	data []byte
	off  int
	eof  bool
}

func (b *trackingBody) Read(p []byte) (int, error) {
	if b.off == len(b.data) {
		b.eof = true
		return 0, io.EOF
	}
	n := min(2, len(b.data)-b.off)
	copy(p, b.data[b.off:b.off+n])
	b.off += n
	return n, nil
}

func (*trackingBody) Close() error { return nil }

func TestHTTPTraceSinkDrainsTheResponseBody(t *testing.T) {
	body := &trackingBody{data: []byte("response body")}
	sink := &sandbox.HTTPTraceSink{Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNoContent, Status: "204 No Content", Body: body}, nil
	})}}

	if err := sink.Push(context.Background(), "http://platform/internal/trace/token", nil); err != nil {
		t.Fatal(err)
	}
	if !body.eof {
		t.Fatal("response body was not drained to EOF, so the transport cannot reuse the connection")
	}
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

func TestCollectorPushesTheTailAfterTheWorkloadExits(t *testing.T) {
	sink := &recordingSink{}
	drv, h := newTracingServer(t, sink)

	_, run := do(t, h, "POST", "/runs", tracedRequest(), testToken)
	drv.writeTrace(run.ProviderRunID, event(1), event(2))
	drv.exit(run.ProviderRunID, sandbox.Outcome{ExitCode: 0})

	waitFor(t, func() bool { return len(sink.received()) == 2 })
}

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

func TestFailedTraceReadIsRetriedBeforeTheWorkloadIsReleased(t *testing.T) {
	sink := &recordingSink{}
	drv, h := newTracingServer(t, sink)

	_, run := do(t, h, "POST", "/runs", tracedRequest(), testToken)
	drv.writeTrace(run.ProviderRunID, event(1))
	drv.mu.Lock()
	drv.readTraceFailures = 1
	drv.done[run.ProviderRunID] = true
	drv.mu.Unlock()

	waitFor(t, func() bool {
		drv.mu.Lock()
		defer drv.mu.Unlock()
		return len(sink.received()) == 1 && drv.released[run.ProviderRunID] == 1
	})
	drv.exit(run.ProviderRunID, sandbox.Outcome{ExitCode: 0})
}

func TestPartialTrailingLineIsHeldBackUntilComplete(t *testing.T) {
	sink := &recordingSink{}
	drv, h := newTracingServer(t, sink)

	_, run := do(t, h, "POST", "/runs", tracedRequest(), testToken)

	drv.writeTrace(run.ProviderRunID, event(1))
	drv.appendRawTrace(run.ProviderRunID, `{"schema_version":"1.0","ev`)
	drv.exit(run.ProviderRunID, sandbox.Outcome{ExitCode: 0})

	waitFor(t, func() bool { return len(sink.received()) == 1 })
	if got := sink.received(); got[0] != event(1) {
		t.Errorf("pushed %q, want only the complete line", got[0])
	}
}

func TestCollectorDrainsPastTheEightMiBReadWindow(t *testing.T) {
	sink := &recordingSink{}
	drv, h := newTracingServer(t, sink)

	_, run := do(t, h, "POST", "/runs", tracedRequest(), testToken)
	padding := strings.Repeat("x", 90<<10)
	for seq := 1; seq <= 100; seq++ {
		drv.appendRawTrace(run.ProviderRunID, fmt.Sprintf(
			`{"event_id":"%08d-0000-4000-8000-000000000000","seq":%d,"payload":"%s"}`+"\n",
			seq, seq, padding,
		))
	}
	tail := `{"event_id":"tail","seq":101,"payload":"last"}`
	drv.appendRawTrace(run.ProviderRunID, tail+"\n")
	drv.exit(run.ProviderRunID, sandbox.Outcome{ExitCode: 0})

	waitFor(t, func() bool {
		got := sink.received()
		return len(got) == 101 && got[len(got)-1] == tail
	})
}

func TestOversizedEventDoesNotPinValidTail(t *testing.T) {
	sink := &recordingSink{}
	drv, h := newTracingServer(t, sink)

	_, run := do(t, h, "POST", "/runs", tracedRequest(), testToken)
	drv.appendRawTrace(run.ProviderRunID, `{"event_id":"oversized","padding":"`+strings.Repeat("x", 4<<20)+`"}`+"\n")
	tail := `{"event_id":"tail","seq":2,"payload":"last"}`
	drv.appendRawTrace(run.ProviderRunID, tail+"\n")
	drv.exit(run.ProviderRunID, sandbox.Outcome{ExitCode: 0})

	waitFor(t, func() bool {
		got := sink.received()
		return len(got) == 1 && got[0] == tail
	})
}

func TestNoIngestionURLCollectsNothing(t *testing.T) {
	sink := &recordingSink{}
	drv, h := newTracingServer(t, sink)

	_, run := do(t, h, "POST", "/runs", runRequest(), testToken)
	drv.writeTrace(run.ProviderRunID, event(1))
	drv.exit(run.ProviderRunID, sandbox.Outcome{ExitCode: 0})

	time.Sleep(100 * time.Millisecond)
	if got := sink.received(); len(got) != 0 {
		t.Errorf("pushed %d events with no destination configured", len(got))
	}
}

const liveToken = "11111111-1111-1111-1111-111111111111.1.99999999999.f2c1a0deadbeefcafe"

const liveTraceURL = "http://platform:8080/internal/trace/" + liveToken

func TestWorkloadOutputIsScrubbedOfTheTraceIngestionToken(t *testing.T) {
	sink := &recordingSink{}
	drv, h := newTracingServer(t, sink)
	req := runRequest()
	req.Trace = sandbox.TracePolicy{Level: "standard", IngestionURL: liveTraceURL}

	_, run := do(t, h, "POST", "/runs", req, testToken)
	drv.exit(run.ProviderRunID, sandbox.Outcome{
		ExitCode: 0,
		Output:   "config: SKILLHUB_TRACE_URL=" + liveTraceURL,
	})
	final := waitForTerminal(t, h, run.ProviderRunID)

	if strings.Contains(final.Result.AgentOutput, liveToken) {
		t.Fatalf("agent_output = %q, still carries the live ingestion token", final.Result.AgentOutput)
	}
	if want := "config: SKILLHUB_TRACE_URL=http://platform:8080/internal/trace/***"; final.Result.AgentOutput != want {
		t.Errorf("agent_output = %q, want %q", final.Result.AgentOutput, want)
	}
}

type urlErrorSink struct{}

func (urlErrorSink) Push(_ context.Context, url string, _ []json.RawMessage) error {
	return &neturl.Error{Op: "Post", URL: url, Err: errors.New("dial tcp 10.0.0.2:8080: connect: connection refused")}
}

func TestPushFailureDoesNotLogTheIngestionToken(t *testing.T) {
	var logged safeBuffer
	drv := newFakeDriver()
	m := sandbox.NewManager(drv, sandbox.Config{
		Provider:       "docker_dev",
		Runtimes:       []sandbox.RuntimeCapability{{Runtime: "claude_agent_sdk", Versions: []string{"0.3.233"}, AgentIntegration: []string{"in_sandbox_sdk"}}},
		MaxResources:   sandbox.DefaultLimits,
		IsolationLevel: "container",
		EgressModes:    []string{"none"},
		Slots:          2,
	}, slog.New(slog.NewTextHandler(&logged, nil))).WithTrace(urlErrorSink{}, nil)
	h := (&sandbox.Server{M: m, Token: testToken}).Routes()

	req := runRequest()
	req.Trace = sandbox.TracePolicy{Level: "standard", IngestionURL: liveTraceURL}
	_, run := do(t, h, "POST", "/runs", req, testToken)
	drv.writeTrace(run.ProviderRunID, event(1))
	drv.exit(run.ProviderRunID, sandbox.Outcome{ExitCode: 0})
	waitForTerminal(t, h, run.ProviderRunID)

	out := logged.String()
	if !strings.Contains(out, "trace push failed") {
		t.Fatalf("the failing push was never logged at all, so this proves nothing: %q", out)
	}
	if strings.Contains(out, liveToken) {
		t.Errorf("the sandbox host's log carries a live ingestion token: %s", out)
	}
}

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
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
