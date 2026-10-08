package operations

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

const findingListLimit = 100

var findingStatuses = []FindingStatus{FindingOpen, FindingAcknowledged, FindingResolved, FindingDismissed, FindingRecovered}

type findingView struct {
	ID              string   `json:"id"`
	Agent           string   `json:"agent"`
	Status          string   `json:"status"`
	Title           string   `json:"title"`
	Cites           []string `json:"cites"`
	AssigneeUserID  string   `json:"assignee_user_id,omitempty"`
	FirstSeenAt     string   `json:"first_seen_at"`
	LastSeenAt      string   `json:"last_seen_at"`
	SeenCount       int32    `json:"seen_count"`
	StatusChangedAt string   `json:"status_changed_at"`
}

type findingEventView struct {
	Seq            int32           `json:"seq"`
	Kind           string          `json:"kind"`
	RunID          string          `json:"run_id,omitempty"`
	OperatorUserID string          `json:"operator_user_id,omitempty"`
	Text           string          `json:"text,omitempty"`
	Evidence       json.RawMessage `json:"evidence,omitempty"`
	Note           string          `json:"note,omitempty"`
	OccurredAt     string          `json:"occurred_at"`
}

type findingsResponse struct {
	Findings []findingView    `json:"findings"`
	Counts   map[string]int32 `json:"counts"`
}

type findingDetailResponse struct {
	Finding findingView        `json:"finding"`
	Events  []findingEventView `json:"events"`
}

func (s *Service) Findings(ctx context.Context, statuses []string) ([]gen.ListFindingsRow, map[string]int32, error) {
	q := gen.New(s.Pool)
	rows, err := q.ListFindings(ctx, gen.ListFindingsParams{Statuses: statuses, RowLimit: findingListLimit})
	if err != nil {
		return nil, nil, err
	}
	countRows, err := q.CountFindingsByStatus(ctx)
	if err != nil {
		return nil, nil, err
	}
	counts := make(map[string]int32, len(findingStatuses))
	for _, status := range findingStatuses {
		counts[string(status)] = 0
	}
	for _, row := range countRows {
		counts[row.Status] = row.Findings
	}
	return rows, counts, nil
}

func (s *Service) Finding(ctx context.Context, id pgtype.UUID) (gen.GetFindingRow, []gen.ListFindingEventsRow, error) {
	q := gen.New(s.Pool)
	row, err := q.GetFinding(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, nil, ErrUnknownFinding
	}
	if err != nil {
		return row, nil, err
	}
	events, err := q.ListFindingEvents(ctx, id)
	return row, events, err
}

func (h *Handler) Findings(w http.ResponseWriter, r *http.Request) {
	statuses := liveStatuses
	if raw := r.URL.Query().Get("status"); raw != "" {
		if !slices.Contains(findingStatuses, FindingStatus(raw)) {
			httpx.WriteError(w, http.StatusBadRequest, "status must be one of open, acknowledged, resolved, dismissed, recovered")
			return
		}
		statuses = []string{raw}
	}
	rows, counts, err := h.Svc.Findings(r.Context(), statuses)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "finding lookup failed")
		return
	}
	response := findingsResponse{Findings: make([]findingView, len(rows)), Counts: counts}
	for i, row := range rows {
		response.Findings[i] = findingBody(gen.GetFindingRow(row))
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

func (h *Handler) Finding(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "finding")
	if !ok {
		return
	}
	row, events, err := h.Svc.Finding(r.Context(), id)
	if errors.Is(err, ErrUnknownFinding) {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "finding lookup failed")
		return
	}
	response := findingDetailResponse{Finding: findingBody(row), Events: make([]findingEventView, len(events))}
	for i, e := range events {
		response.Events[i] = findingEventView{
			Seq: e.Seq, Kind: e.Kind, RunID: pgconv.UUIDString(e.RunID), OperatorUserID: pgconv.UUIDString(e.OperatorID),
			Text: deref(e.Text), Evidence: e.Evidence, Note: deref(e.Note), OccurredAt: pgconv.RFC3339(e.OccurredAt),
		}
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

func (h *Handler) MoveFinding(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "finding")
	if !ok {
		return
	}
	var body struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if !decode(w, r, &body) {
		return
	}
	to := FindingStatus(body.Status)
	if to == FindingRecovered || !slices.Contains(findingStatuses, to) {
		httpx.WriteError(w, http.StatusBadRequest, "status must be one of open, acknowledged, resolved, dismissed")
		return
	}
	note, ok := requireNote(w, body.Note)
	if !ok {
		return
	}
	operator, ok := h.operator(w, r)
	if !ok {
		return
	}
	err := h.Svc.MoveFinding(r.Context(), id, to, operator, note)
	switch {
	case errors.Is(err, ErrUnknownFinding):
		httpx.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, ErrFindingTransition):
		httpx.WriteError(w, http.StatusConflict, "the finding cannot move to that status from where it is now")
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "finding update failed")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func findingBody(row gen.GetFindingRow) findingView {
	return findingView{
		ID: pgconv.UUIDString(row.ID), Agent: row.Agent, Status: row.Status, Title: row.Title, Cites: row.Cites,
		AssigneeUserID: pgconv.UUIDString(row.AssigneeID), FirstSeenAt: pgconv.RFC3339(row.FirstSeenAt),
		LastSeenAt: pgconv.RFC3339(row.LastSeenAt), SeenCount: row.SeenCount,
		StatusChangedAt: pgconv.RFC3339(row.StatusChangedAt),
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
