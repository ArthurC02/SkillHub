package dockerdrv_test

// The one test with no fake anywhere on the path (TRACE-002~008): a real
// container writes real JSONL into its own /out tmpfs, the real driver copies it
// out through the Docker API, the real Manager collector batches it, and a real
// HTTP server on the other end records what arrived.
//
// The workload is a shell script rather than the runtime image's Agent SDK
// harness for one honest reason: no model gateway grant is minted yet
// (SBX-008), and a sandbox on `--network none` with no gateway credential cannot
// reach a model, so it cannot produce SDK-derived events. What that leaves
// untested end to end is the mapping from SDK messages to events; what it does
// test is every step between a written line and a delivered batch.

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/services/sandbox/internal/sandbox"
)

// collector is the platform's ingestion endpoint, reduced to what the provider
// can observe of it: it accepts a batch and answers 202.
type collector struct {
	*httptest.Server
	mu     sync.Mutex
	events []json.RawMessage
	paths  []string
	pushes int
}

func newCollector() *collector {
	c := &collector{}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		var batch []json.RawMessage
		_ = json.Unmarshal(body, &batch)
		c.mu.Lock()
		c.events = append(c.events, batch...)
		c.paths = append(c.paths, r.URL.Path)
		c.pushes++
		c.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	return c
}

func (c *collector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events)
}

func TestTraceEventsReachTheCollectorFromARealContainer(t *testing.T) {
	drv, _ := newDriver(t)
	sink := newCollector()
	t.Cleanup(sink.Close)

	m := sandbox.NewManager(drv, sandbox.Config{
		Provider:       "docker_dev",
		Runtimes:       []sandbox.RuntimeCapability{{Runtime: "claude_agent_sdk", Versions: []string{"0.3.233"}, AgentIntegration: []string{"in_sandbox_sdk"}}},
		MaxResources:   sandbox.DefaultLimits,
		IsolationLevel: "container",
		EgressModes:    []string{"none"},
		Slots:          1,
	}, slog.New(slog.DiscardHandler)).WithTrace(&sandbox.HTTPTraceSink{}, nil)

	req := testRequest(traceScript)
	// The URL a RunRequest actually carries: the credential is the last path
	// segment, minted by the platform for this one (run, attempt).
	req.Trace = sandbox.TracePolicy{
		Level:        "standard",
		IngestionURL: sink.URL + "/internal/trace/11111111-1111-1111-1111-111111111111.1.99999999999.deadbeefcafe",
	}

	run, created, err := m.Create(context.Background(), req)
	if err != nil || !created {
		t.Fatalf("create: %v (created=%v)", err, created)
	}
	t.Cleanup(func() { _ = m.Destroy(context.Background(), run.ProviderRunID) })

	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) && sink.count() < 3 {
		time.Sleep(200 * time.Millisecond)
	}
	if n := sink.count(); n != 3 {
		t.Fatalf("collector received %d events, want 3", n)
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()

	// The sequence must arrive gapless and in order: that is what makes a hole
	// mean "an event was lost" on the reading side (TRACE-008).
	for i, raw := range sink.events {
		var e struct {
			SchemaVersion string `json:"schema_version"`
			EventID       string `json:"event_id"`
			RunID         string `json:"run_id"`
			Attempt       int    `json:"attempt"`
			Seq           int    `json:"seq"`
			EmittedBy     string `json:"emitted_by"`
			Type          string `json:"type"`
			Masked        bool   `json:"masked"`
		}
		if err := json.Unmarshal(raw, &e); err != nil {
			t.Fatalf("event %d did not survive the round trip as JSON: %v", i, err)
		}
		switch {
		case e.SchemaVersion != "1.0":
			t.Errorf("event %d: schema_version %q", i, e.SchemaVersion)
		case e.Seq != i+1:
			t.Errorf("event %d: seq %d, want %d", i, e.Seq, i+1)
		case e.EmittedBy != "sandbox" || e.Attempt != 1:
			t.Errorf("event %d: emitted_by %q attempt %d", i, e.EmittedBy, e.Attempt)
		case e.Masked:
			// The sandbox must not claim its own events are masked: masking is
			// the control plane's, and a producer vouching for itself is exactly
			// what the trust boundary forbids (iron rule 11).
			t.Errorf("event %d claims to be masked by the producer", i)
		}
	}

	// More than one push means the collector really is draining a live sandbox
	// rather than reading everything once at the end (NFR-004).
	if sink.pushes < 2 {
		t.Errorf("events arrived in %d push(es); the live drain never ran", sink.pushes)
	}
	if !strings.HasPrefix(sink.paths[0], "/internal/trace/") {
		t.Errorf("push went to %q, not the signed ingestion path", sink.paths[0])
	}

	// Written out so tools/contracts/validate_trace_events.py can check the real
	// output of the pipeline against the schema, rather than only its examples.
	if out := os.Getenv("SKILLHUB_TRACE_SAMPLE_OUT"); out != "" {
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

// traceScript writes the envelope shape the runtime image's harness writes. The
// pause in the middle is what forces a second collection pass, and the secret in
// the tool_call arguments is there so the sample this produces exercises the
// platform's masker (TRACE-005) rather than only its parser.
const traceScript = `
mkdir -p /out/trace
cat >> /out/trace/events.jsonl <<'EOF'
{"schema_version":"1.0","event_id":"0f0a1e6c-1c9a-4f8e-9a2b-1d5a2c7b3e01","run_id":"11111111-1111-1111-1111-111111111111","attempt":1,"seq":1,"occurred_at":"2026-08-16T09:12:04.002Z","emitted_by":"sandbox","type":"skill_activation","status":"ok","masked":false,"masked_fields":[],"payload":{"skill_version_id":"44444444-4444-4444-4444-444444444444","skill_name":"excel-deduplicate","decision":"activated","reason":"the task mentions removing duplicate rows"}}
EOF
sleep 4
cat >> /out/trace/events.jsonl <<'EOF'
{"schema_version":"1.0","event_id":"1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c02","run_id":"11111111-1111-1111-1111-111111111111","attempt":1,"seq":2,"occurred_at":"2026-08-16T09:12:07.880Z","emitted_by":"sandbox","type":"tool_call","status":"ok","masked":false,"masked_fields":[],"payload":{"tool_name":"bash","invocation_id":"call_7fd21","arguments":{"command":"curl -H 'Authorization: Bearer sk-TESTKEYAAAAAAAAAAAAAAAAAAAA' https://api.example"},"result_summary":"wrote output.xlsx, 1204 rows in, 1187 rows out","outcome":"succeeded","duration_ms":3412,"truncated":false}}
{"schema_version":"1.0","event_id":"2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d03","run_id":"11111111-1111-1111-1111-111111111111","attempt":1,"seq":3,"occurred_at":"2026-08-16T09:12:09.500Z","emitted_by":"sandbox","type":"agent_output","status":"ok","masked":false,"masked_fields":[],"payload":{"kind":"final","text":"Removed 17 duplicate rows and saved the result to output.xlsx.","truncated":false}}
EOF
sleep 1
`
