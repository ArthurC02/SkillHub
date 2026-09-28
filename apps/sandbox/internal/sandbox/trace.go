package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

const (
	traceInterval = 2 * time.Second

	traceBatch = 200

	traceBatchBytes = 3 << 20

	traceSinkTimeout = 15 * time.Second

	traceResponseDrainLimit = 1 << 20
)

type TraceSink interface {
	Push(ctx context.Context, url string, events []json.RawMessage) error
}

type HTTPTraceSink struct{ Client *http.Client }

func (s *HTTPTraceSink) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: traceSinkTimeout}
}

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
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, traceResponseDrainLimit))
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
	return encoded.Len() - 1, nil // exclude the encoder's trailing newline
}

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

			continue
		}
		out = append(out, traceLine{event: json.RawMessage(line), end: consumed})
	}
}

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

				if m.collect(ctx, id, url) {
					return
				}
			}
		}
	}()
	return func() {
		cancel()
		<-done

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
	finalFlushAttempts = 4
	finalFlushBackoff  = 250 * time.Millisecond
)

func (m *Manager) flushTrace(parent context.Context, id, url string) bool {
	if url == "" || m.sink == nil {
		return true
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	for {
		offset, secrets, tracked := m.traceCursor(id)
		if !tracked {
			return true
		}

		raw, more, err := m.drv.ReadTrace(ctx, id, offset)
		if err != nil {
			return false
		}
		if len(raw) == 0 {
			return true
		}
		lines, consumed := splitEvents(raw)
		if more && consumed == 0 {
			// A single line fills the whole read window with no terminator yet;
			// skip past it so the collector doesn't stall waiting for it to end.
			m.recordTraceOffset(id, offset+int64(len(raw)))
			continue
		}

		if !m.pushTraceLines(ctx, id, url, secrets, traceRead{offset: offset, lines: lines}) {
			return false
		}

		if consumed > 0 {
			m.recordTraceOffset(id, offset+consumed)
		}
		if !more {
			return true
		}
	}
}

func (m *Manager) traceCursor(id string) (int64, []string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.runs[id]
	if e == nil {
		return 0, nil, false
	}
	return e.traceOffset, e.secrets, true
}

func (m *Manager) recordTraceOffset(id string, offset int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.runs[id]; e != nil {
		e.traceOffset = offset
	}
}

type traceRead struct {
	offset int64
	lines  []traceLine
}

func (m *Manager) pushTraceLines(ctx context.Context, id, url string, secrets []string, read traceRead) bool {
	lines := read.lines
	for sent := 0; sent < len(lines); {
		if m.dropOversizedTraceEvent(id, read.offset, lines[sent]) {
			sent++
			continue
		}
		end := traceBatchEnd(lines, sent)
		if !m.pushTraceBatch(ctx, id, url, secrets, lines[sent:end]) {
			return false
		}
		sent = end
		m.recordTraceOffset(id, read.offset+lines[sent-1].end)
	}
	return true
}

func (m *Manager) dropOversizedTraceEvent(id string, offset int64, line traceLine) bool {
	wireBytes, err := traceEventWireBytes(line.event)
	if err != nil || wireBytes+3 > traceBatchBytes {
		m.log.Warn("dropping oversized trace event", "provider_run_id", id, "bytes", wireBytes)
		m.metrics.tracePush("dropped_oversized", 0)
		m.recordTraceOffset(id, offset+line.end)
		return true
	}
	return false
}

func traceBatchEnd(lines []traceLine, start int) int {
	end, size := start, 2
	for end < len(lines) && end-start < traceBatch {
		wireBytes, err := traceEventWireBytes(lines[end].event)
		if err != nil {
			break
		}
		next := wireBytes + 1
		if end > start && size+next > traceBatchBytes {
			break
		}
		size += next
		end++
	}
	return end
}

func (m *Manager) pushTraceBatch(ctx context.Context, id, url string, secrets []string, batch []traceLine) bool {
	events := make([]json.RawMessage, 0, len(batch))
	for _, line := range batch {
		events = append(events, line.event)
	}
	if err := m.sink.Push(ctx, url, events); err != nil {
		// mask() scrubs the run's secrets from the error text: a *url.Error
		// embeds the request URL, which carries the ingestion token.
		m.log.Warn("trace push failed", "provider_run_id", id, "err", mask(err.Error(), secrets))
		m.metrics.tracePush("error", 0)
		return false
	}
	m.metrics.tracePush("ok", len(batch))
	return true
}
