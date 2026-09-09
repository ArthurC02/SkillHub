package apiserver

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
)

// The SSE half of ADR-069 (05 R-71, signed 2026-09-09).
//
// # What this streams, and what it refuses to
//
// State Go has already committed, never text a model is still writing. A model
// reply is a proposal until proposal() accepts it: it can be rejected whole, or
// thrown away and asked for again (the nudge path, twice per session). Painting
// tokens on a screen would assert something the platform has not agreed to, and
// on the nudge path the person would watch a draft appear and silently vanish.
// So each event carries one CreationSession — the same document GET returns.
//
// # Why there is no relay store
//
// `revision` is already the per-session monotonic sequence, written in the same
// transaction as the snapshot, so Last-Event-ID is a comparison rather than a
// second copy of the stream. The usual Redis relay exists because streamed
// tokens belong to no table; these rows do.
//
// # Why it polls the row instead of LISTEN/NOTIFY
//
// ADR-069 決策 4 allows exactly this and says why: the degraded path is still
// strictly better than what it replaces, because the browser was fetching the
// whole document every second. The cadence is the one this codebase already
// runs for cross-process cancellation (job.go's 250ms watcher on the same row),
// so this adds a second reader of a row that is already being read that way,
// not a new mechanism. The ceiling is honest and small: one indexed primary-key
// read per connection per tick.
const (
	streamTick      = 250 * time.Millisecond
	streamKeepAlive = 20 * time.Second
)

// Stream answers GET /creation-sessions/{session_id}/events.
func (h *creationHandler) Stream(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.scope(w, r)
	if !ok {
		return
	}
	id, err := creation.ParseID(r.PathValue("session_id"))
	if err != nil {
		h.creationError(w, creation.ErrNotFound)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		// No streaming below us. Say so as a service condition rather than
		// hanging: a client that cannot be streamed to must fall back to
		// polling, and it can only do that if it gets an answer.
		h.creationError(w, creation.ErrUnavailable)
		return
	}

	// The cursor is only ever a hint about what the client already has, so a
	// malformed one means 「send me everything」 rather than an error: a browser
	// reconnect must not be able to fail this route.
	cursor := creation.StreamCursor(0)
	if n, convErr := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64); convErr == nil && n > 0 {
		cursor = creation.StreamCursor(n)
	}

	// Before any header goes out: one read, so an unreadable or missing session
	// answers with its real status code instead of a 200 that carries an error
	// inside a stream nobody parses for errors.
	first, changed, err := h.Svc.Changed(r.Context(), ws, id, cursor)
	if err != nil {
		h.creationError(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	// Proxies that buffer turn a stream into one late document. nginx honours
	// this header (infra/images/web/nginx.conf sets proxy_buffering off for
	// this route as well; either alone is enough, and both is the point —
	// buffering is invisible when it happens).
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	send := func(v creation.View) bool {
		b, marshalErr := json.Marshal(v)
		if marshalErr != nil {
			return false
		}
		// `id:` is the revision, which is what a reconnect hands back.
		if _, writeErr := w.Write([]byte("id: " + strconv.FormatInt(v.Revision, 10) + "\ndata: " + string(b) + "\n\n")); writeErr != nil {
			return false
		}
		flusher.Flush()
		cursor = creation.StreamCursor(v.Revision)
		return true
	}

	if changed && !send(first) {
		return
	}
	if changed && creation.StreamDone(first) {
		return
	}

	tick := time.NewTicker(streamTick)
	defer tick.Stop()
	keepAlive := time.NewTicker(streamKeepAlive)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepAlive.C:
			// A comment line: legal SSE, ignored by every client, and enough to
			// stop an idle proxy from closing a session that is simply thinking.
			if _, err := w.Write([]byte(": keep-alive\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case <-tick.C:
			v, moved, err := h.Svc.Changed(r.Context(), ws, id, cursor)
			if err != nil {
				// The session went away or expired under us. Ending the stream
				// is the whole message: the client falls back to GET, which
				// answers with the real status.
				return
			}
			if !moved {
				continue
			}
			if !send(v) {
				return
			}
			if creation.StreamDone(v) {
				return
			}
		}
	}
}
