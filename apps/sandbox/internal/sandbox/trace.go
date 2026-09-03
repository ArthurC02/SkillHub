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
	"io"
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
	// The platform caps an ingestion body at 4 MiB. Stay below it after JSON
	// array punctuation so a batch of individually valid large events is not
	// rejected as one oversized request.
	traceBatchBytes = 3 << 20
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
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(events)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &encoded)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &RunError{Class: ClassExecution, Message: "trace ingestion returned " + resp.Status}
	}
	return nil
}

func traceEventWireBytes(event json.RawMessage) (int, error) {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(event); err != nil {
		return 0, err
	}
	return encoded.Len() - 1, nil // exclude Encoder's trailing newline
}

// splitEvents turns the raw JSONL into whole events, dropping a trailing
// fragment. The workload appends while this is being read, so the last line can
// be half written; sending it would produce a parse error at the far end and,
// worse, would make the next pass skip it as already sent.
type traceLine struct {
	event json.RawMessage
	end   int64
}

func splitEvents(raw []byte) ([]traceLine, int64) {
	var out []traceLine
	var consumed int64
	for {
		i := bytes.IndexByte(raw, '\n')
		if i < 0 {
			return out, consumed
		}
		line := bytes.TrimSpace(raw[:i])
		raw = raw[i+1:]
		consumed += int64(i + 1)
		if len(line) == 0 {
			continue
		}
		if !json.Valid(line) {
			// A line that is complete but not JSON is the workload writing
			// something that is not a trace event. Dropping it here keeps one
			// bad line from stalling the whole stream behind it.
			continue
		}
		out = append(out, traceLine{event: json.RawMessage(line), end: consumed})
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
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		if m.collect(ctx, id, url) {
			return
		}
		ticker := time.NewTicker(traceInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.flushTrace(ctx, id, url)
				// The workload announces it has finished and waits. Everything
				// that has to come out of the tmpfs comes out now, in this
				// window, because after the workload's process exits there is
				// nothing left to read (see DonePath in the driver).
				if m.collect(ctx, id, url) {
					return
				}
			}
		}
	}()
	return func() {
		cancel()
		<-done
		// The final drain gets its own retries. Everything before it could rely
		// on the next tick; this one cannot, and the events it carries are the
		// tail of the run - the error, the final answer, the token usage - which
		// is the part a user most needs when a run went wrong (RUN-004).
		for i := 0; i < finalFlushAttempts; i++ {
			if m.flushTrace(context.Background(), id, url) {
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
func (m *Manager) flushTrace(parent context.Context, id, url string) bool {
	if url == "" || m.sink == nil {
		return true
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	for {

		m.mu.Lock()
		e := m.runs[id]
		if e == nil {
			m.mu.Unlock()
			return true
		}
		offset := e.traceOffset
		// Read under the same lock as the offset: the push error below is logged
		// through this list, and it is the only place the run's own credentials
		// can reach the host's log.
		secrets := e.secrets
		m.mu.Unlock()

		raw, more, err := m.drv.ReadTrace(ctx, id, offset)
		if err != nil {
			return false
		}
		if len(raw) == 0 {
			// No trace file yet, or the sandbox is already gone. Neither is
			// retryable from here, so this counts as done rather than as a failure
			// that would hold the caller in a loop it cannot win.
			return true
		}
		lines, consumed := splitEvents(raw)
		if more && consumed == 0 {
			// One line exceeds the entire read window. It cannot satisfy the event
			// payload limit, so discard this chunk to guarantee forward progress.
			m.mu.Lock()
			if e := m.runs[id]; e != nil {
				e.traceOffset = offset + int64(len(raw))
			}
			m.mu.Unlock()
			continue
		}

		for sent := 0; sent < len(lines); {
			firstWireBytes, err := traceEventWireBytes(lines[sent].event)
			if err != nil || firstWireBytes+3 > traceBatchBytes {
				// This complete line can never fit in the platform's request body
				// (and is far beyond the event payload limit). Drop only that line
				// so untrusted output cannot pin every valid event behind it.
				m.log.Warn("dropping oversized trace event", "provider_run_id", id, "bytes", firstWireBytes)
				m.metrics.tracePush("dropped_oversized", 0)
				m.mu.Lock()
				if e := m.runs[id]; e != nil {
					e.traceOffset = offset + lines[sent].end
				}
				m.mu.Unlock()
				sent++
				continue
			}
			end, size := sent, 2 // JSON array brackets
			for end < len(lines) && end-sent < traceBatch {
				wireBytes, err := traceEventWireBytes(lines[end].event)
				if err != nil {
					break
				}
				next := wireBytes + 1 // comma or Encoder's trailing newline
				if end > sent && size+next > traceBatchBytes {
					break
				}
				size += next
				end++
			}
			events := make([]json.RawMessage, 0, end-sent)
			for _, line := range lines[sent:end] {
				events = append(events, line.event)
			}
			if err := m.sink.Push(ctx, url, events); err != nil {
				// Not advanced: the same events go again next tick, and the platform
				// drops the ones it already has by event_id.
				// Masked rather than unwrapped: a transport failure arrives as
				// *url.Error, whose Error() reads `Post "<full URL>": <cause>`, and
				// that URL's last path segment is this run's live ingestion token
				// (2h TTL) - writing it to the host log hands anyone with log access
				// the ability to append events to this run's timeline. Unwrapping
				// would drop the URL but would also flatten the non-transport cases
				// (a *RunError does not unwrap at all); masking is one call and holds
				// for whatever shape the error arrives in.
				m.log.Warn("trace push failed", "provider_run_id", id, "err", mask(err.Error(), secrets))
				m.metrics.tracePush("error", 0)
				return false
			}
			m.metrics.tracePush("ok", end-sent)
			sent = end

			m.mu.Lock()
			if e := m.runs[id]; e != nil {
				e.traceOffset = offset + lines[sent-1].end
			}
			m.mu.Unlock()
		}
		// Empty/invalid complete lines are intentionally discarded and must not
		// pin the cursor forever. A partial final line is excluded from consumed.
		if consumed > 0 {
			m.mu.Lock()
			if e := m.runs[id]; e != nil {
				e.traceOffset = offset + consumed
			}
			m.mu.Unlock()
		}
		if !more {
			return true
		}
	}
}
