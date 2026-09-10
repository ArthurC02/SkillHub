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
)

type TraceSink interface {
	Push(ctx context.Context, url string, events []json.RawMessage) error
}

type HTTPTraceSink struct{ Client *http.Client }

func (s *HTTPTraceSink) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: 15 * time.Second}
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

		m.mu.Lock()
		e := m.runs[id]
		if e == nil {
			m.mu.Unlock()
			return true
		}
		offset := e.traceOffset

		secrets := e.secrets
		m.mu.Unlock()

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
			end, size := sent, 2
			for end < len(lines) && end-sent < traceBatch {
				wireBytes, err := traceEventWireBytes(lines[end].event)
				if err != nil {
					break
				}
				next := wireBytes + 1
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
				// mask() scrubs the run's secrets from the error text: a *url.Error
				// embeds the request URL, which carries the ingestion token.
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
