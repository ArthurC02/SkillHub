package apiserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
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

	stream := &creationStream{h: h, w: w, flusher: flusher, cursor: lastEventCursor(r)}
	first, changed, err := h.Svc.Changed(r.Context(), ws, id, stream.cursor)
	if err != nil {
		h.creationError(w, err)
		return
	}

	stream.open()
	if changed && !stream.send(first) {
		return
	}
	if changed && creation.StreamDone(first) {
		return
	}
	stream.follow(r.Context(), ws, id)
}

func lastEventCursor(r *http.Request) creation.StreamCursor {
	if n, err := strconv.ParseInt(r.Header.Get("Last-Event-ID"), 10, 64); err == nil && n > 0 {
		return creation.StreamCursor(n)
	}
	return creation.StreamCursor(0)
}

type creationStream struct {
	h       *creationHandler
	w       http.ResponseWriter
	flusher http.Flusher
	cursor  creation.StreamCursor
}

func (s *creationStream) open() {
	s.w.Header().Set("Content-Type", "text/event-stream")
	s.w.Header().Set("Cache-Control", "no-store")
	s.w.Header().Set("X-Accel-Buffering", "no")
	s.w.WriteHeader(http.StatusOK)
	s.flusher.Flush()
}

func (s *creationStream) send(v creation.View) bool {
	presented, err := s.h.present(v)
	if err != nil {
		return false
	}
	b, err := json.Marshal(presented)
	if err != nil {
		return false
	}
	if _, err := s.w.Write([]byte("id: " + strconv.FormatInt(v.Revision, 10) + "\ndata: " + string(b) + "\n\n")); err != nil {
		return false
	}
	s.flusher.Flush()
	s.cursor = creation.StreamCursor(v.Revision)
	return true
}

func (s *creationStream) keepAlive() bool {
	if _, err := s.w.Write([]byte(": keep-alive\n\n")); err != nil {
		return false
	}
	s.flusher.Flush()
	return true
}

func (s *creationStream) follow(ctx context.Context, ws identity.Workspace, id pgtype.UUID) {
	tick := time.NewTicker(streamTick)
	defer tick.Stop()
	keepAlive := time.NewTicker(streamKeepAlive)
	defer keepAlive.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-keepAlive.C:
			if !s.keepAlive() {
				return
			}
		case <-tick.C:
			if !s.relayChange(ctx, ws, id) {
				return
			}
		}
	}
}

func (s *creationStream) relayChange(ctx context.Context, ws identity.Workspace, id pgtype.UUID) bool {
	v, moved, err := s.h.Svc.Changed(ctx, ws, id, s.cursor)
	if err != nil {
		return false
	}
	if !moved {
		return true
	}
	return s.send(v) && !creation.StreamDone(v)
}
