package apiserver

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
)

const (
	streamTick      = 250 * time.Millisecond
	streamKeepAlive = 20 * time.Second
)

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

		h.creationError(w, creation.ErrUnavailable)
		return
	}

	cursor := creation.StreamCursor(0)
	if n, convErr := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64); convErr == nil && n > 0 {
		cursor = creation.StreamCursor(n)
	}

	first, changed, err := h.Svc.Changed(r.Context(), ws, id, cursor)
	if err != nil {
		h.creationError(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")

	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	send := func(v creation.View) bool {
		b, marshalErr := json.Marshal(v)
		if marshalErr != nil {
			return false
		}

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

			if _, err := w.Write([]byte(": keep-alive\n\n")); err != nil {
				return
			}
			flusher.Flush()
		case <-tick.C:
			v, moved, err := h.Svc.Changed(r.Context(), ws, id, cursor)
			if err != nil {

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
