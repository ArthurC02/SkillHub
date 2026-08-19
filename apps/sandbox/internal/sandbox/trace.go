package sandbox

// TRACE-002: getting the workload's trace events out of a network-less sandbox.
//
// The container runs on `--network none`, so it cannot post anywhere. It writes
// one JSON object per line to /out/trace/events.jsonl instead, and this file
// reads that out and pushes it to the control plane's ingestion endpoint. Only
// sandboxd has a route to the platform, and it still never touches the database
// (iron rule 2): it POSTs JSON and is told nothing back but a count.
//
// Delivery is at-least-once by construction. A batch whose response was not a
// 2xx is not marked sent, so the next tick sends it again; the platform dedupes
// on the producer's event_id, which is exactly what that key is for (TRACE-008).
// The failure this protects against is the common one - a restart or a blip
// between the workload writing an event and the platform storing it.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

const (
	// traceInterval is how often a live sandbox's trace file is drained. NFR-004
	// targets 3 seconds from an event being produced to it showing on screen, and
	// this is the largest part of that budget.
	traceInterval = 2 * time.Second
	// traceBatch bounds one POST. A workload that logs in a loop drains over
	// several passes instead of building one enormous request.
	traceBatch = 200
)

// TraceSink pushes one batch of already-encoded events. Split out so the manager
// tests exercise the collection and idempotency rules without an HTTP server.
type TraceSink interface {
	Push(ctx context.Context, url string, events []json.RawMessage) error
}

// HTTPTraceSink is the real sink.
type HTTPTraceSink struct{ Client *http.Client }

func (s *HTTPTraceSink) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: 15 * time.Second}
}

// Push POSTs the batch to the run's ingestion URL. The URL carries its own
// credential (the platform's per-attempt signed token), so no header is added
// here and nothing about the token is logged - it is secret material that the
// masker on the far side also knows to redact.
func (s *HTTPTraceSink) Push(ctx context.Context, url string, events []json.RawMessage) error {
	body, err := json.Marshal(events)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &RunError{Class: ClassExecution, Message: "trace ingestion returned " + resp.Status}
	}
	return nil
}

// splitEvents turns the raw JSONL into whole events, dropping a trailing
// fragment. The workload appends while this is being read, so the last line can
// be half written; sending it would produce a parse error at the far end and,
// worse, would make the next pass skip it as already sent.
func splitEvents(raw []byte) []json.RawMessage {
	var out []json.RawMessage
	for {
		i := bytes.IndexByte(raw, '\n')
		if i < 0 {
			return out
		}
		line := bytes.TrimSpace(raw[:i])
		raw = raw[i+1:]
		if len(line) == 0 {
			continue
		}
		if !json.Valid(line) {
			// A line that is complete but not JSON is the workload writing
			// something that is not a trace event. Dropping it here keeps one
			// bad line from stalling the whole stream behind it.
			continue
		}
		out = append(out, json.RawMessage(line))
	}
}

// startTraceCollector begins draining one sandbox's trace file and returns the
// stop function. Stopping performs one final drain: the workload has exited by
// then but the container - and with it the /out tmpfs - is still there, and this
// is the only window in which the last events can still be read.
func (m *Manager) startTraceCollector(id string) func() {
	m.mu.Lock()
	var url string
	if e := m.runs[id]; e != nil {
		url = e.traceURL
	}
	m.mu.Unlock()
	if url == "" || m.sink == nil {
		return func() {}
	}

	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(traceInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				m.flushTrace(id, url)
				// The workload announces it has finished and waits. Everything
				// that has to come out of the tmpfs comes out now, in this
				// window, because after the workload's process exits there is
				// nothing left to read (see DonePath in the driver).
				if m.collect(id, url) {
					return
				}
			}
		}
	}()
	return func() {
		close(stop)
		<-done
		// The final drain gets its own retries. Everything before it could rely
		// on the next tick; this one cannot, and the events it carries are the
		// tail of the run - the error, the final answer, the token usage - which
		// is the part a user most needs when a run went wrong (RUN-004).
		for i := 0; i < finalFlushAttempts; i++ {
			if m.flushTrace(id, url) {
				return
			}
			time.Sleep(finalFlushBackoff)
		}
		m.log.Error("gave up pushing the tail of a run's trace", "provider_run_id", id)
	}
}

const (
	// finalFlushAttempts and finalFlushBackoff bound the wait before DELETE
	// takes the sandbox - and its trace file - away. Short: the control plane
	// schedules cleanup as soon as the run goes terminal.
	finalFlushAttempts = 4
	finalFlushBackoff  = 250 * time.Millisecond
)

// flushTrace reads the file and pushes whatever has not been pushed yet,
// recording how far it got only after each batch is accepted. It reports
// whether there is nothing left to send.
func (m *Manager) flushTrace(id, url string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	raw, err := m.drv.ReadTrace(ctx, id)
	if err != nil || len(raw) == 0 {
		// No trace file yet, or the sandbox is already gone. Neither is
		// retryable from here, so this counts as done rather than as a failure
		// that would hold the caller in a loop it cannot win.
		return true
	}
	events := splitEvents(raw)

	m.mu.Lock()
	e := m.runs[id]
	if e == nil {
		m.mu.Unlock()
		return true // destroyed under us; there is nothing left to read from
	}
	sent := e.traceSent
	m.mu.Unlock()

	for sent < len(events) {
		end := min(sent+traceBatch, len(events))
		if err := m.sink.Push(ctx, url, events[sent:end]); err != nil {
			// Not advanced: the same events go again next tick, and the platform
			// drops the ones it already has by event_id.
			m.log.Warn("trace push failed", "provider_run_id", id, "err", err)
			m.metrics.tracePush("error", 0)
			return false
		}
		m.metrics.tracePush("ok", end-sent)
		sent = end

		m.mu.Lock()
		if e := m.runs[id]; e != nil {
			e.traceSent = sent
		}
		m.mu.Unlock()
	}
	return true
}
