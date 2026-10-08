package operations

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

const (
	proposalListLimit   = 20
	closedProposalShown = 7 * 24 * time.Hour
)

type proposalView struct {
	ID         string `json:"id"`
	Agent      string `json:"agent"`
	Action     string `json:"action"`
	Tier       string `json:"tier"`
	Reason     string `json:"reason"`
	Status     string `json:"status"`
	ProposedAt string `json:"proposed_at"`
	ExpiresAt  string `json:"expires_at"`
	FinishedAt string `json:"finished_at,omitempty"`
}

type proposalDetailView struct {
	proposalView
	RunID         string          `json:"run_id"`
	Cites         []string        `json:"cites"`
	Preview       json.RawMessage `json:"preview"`
	DecidedUserID string          `json:"decided_by_user_id,omitempty"`
	DecidedAt     string          `json:"decided_at,omitempty"`
	DecisionNote  string          `json:"decision_note,omitempty"`
	StartedAt     string          `json:"started_at,omitempty"`
	Outcome       string          `json:"outcome,omitempty"`
}

type proposalsResponse struct {
	Proposals []proposalView `json:"proposals"`
	Total     int64          `json:"total"`
}

func (s *Service) Proposals(ctx context.Context, now time.Time, view string, offset int32) ([]gen.ListProposalsRow, int64, error) {
	queries := gen.New(s.Pool)
	closed := []string{string(ProposalRejected), string(ProposalExpired), string(ProposalSucceeded), string(ProposalFailed)}
	unbounded, recent := liveProposalStatuses, closed
	switch view {
	case "proposed":
		unbounded, recent = []string{string(ProposalProposed)}, nil
	case "processing":
		unbounded, recent = []string{string(ProposalApproved), string(ProposalRunning)}, nil
	case "closed":
		unbounded, recent = nil, closed
	}
	params := gen.ListProposalsParams{
		Unbounded: unbounded, Recent: recent, PendingStatus: string(ProposalProposed),
		ClosedSince: pgconv.Timestamptz(now.Add(-closedProposalShown)), RowOffset: offset, RowLimit: proposalListLimit,
	}
	rows, err := queries.ListProposals(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	if len(rows) > 0 {
		return rows, rows[0].Total, nil
	}
	if offset == 0 {
		return rows, 0, nil
	}
	params.RowOffset, params.RowLimit = 0, 1
	first, err := queries.ListProposals(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	if len(first) > 0 {
		return rows, first[0].Total, nil
	}
	return rows, 0, nil
}

func (s *Service) Proposal(ctx context.Context, id pgtype.UUID) (gen.GetProposalRow, error) {
	row, err := gen.New(s.Pool).GetProposal(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrUnknownProposal
	}
	return row, err
}

func (h *Handler) Proposals(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	view := "all"
	if query.Has("view") {
		view = query.Get("view")
	}
	switch view {
	case "all", "proposed", "processing", "closed":
	default:
		httpx.WriteError(w, http.StatusBadRequest, "view must be all, proposed, processing or closed")
		return
	}
	var offset int32
	if query.Has("offset") {
		parsed, err := strconv.ParseInt(query.Get("offset"), 10, 32)
		if err != nil || parsed < 0 {
			httpx.WriteError(w, http.StatusBadRequest, "offset must be a nonnegative integer")
			return
		}
		offset = int32(parsed)
	}
	rows, total, err := h.Svc.Proposals(r.Context(), time.Now(), view, offset)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "proposal lookup failed")
		return
	}
	response := proposalsResponse{Proposals: make([]proposalView, len(rows)), Total: total}
	for i, row := range rows {
		response.Proposals[i] = proposalView{
			ID: pgconv.UUIDString(row.ID), Agent: row.Agent, Action: row.Action, Tier: row.Tier, Reason: row.Reason,
			Status: row.Status, ProposedAt: pgconv.RFC3339(row.ProposedAt), ExpiresAt: pgconv.RFC3339(row.ExpiresAt),
			FinishedAt: pgconv.RFC3339(row.FinishedAt),
		}
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

func (h *Handler) Proposal(w http.ResponseWriter, r *http.Request) {
	id, ok := proposalID(w, r)
	if !ok {
		return
	}
	row, err := h.Svc.Proposal(r.Context(), id)
	if errors.Is(err, ErrUnknownProposal) {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "proposal lookup failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, proposalDetailView{
		proposalView: proposalView{
			ID: pgconv.UUIDString(row.ID), Agent: row.Agent, Action: row.Action, Tier: row.Tier, Reason: row.Reason,
			Status: row.Status, ProposedAt: pgconv.RFC3339(row.ProposedAt), ExpiresAt: pgconv.RFC3339(row.ExpiresAt),
			FinishedAt: pgconv.RFC3339(row.FinishedAt),
		},
		RunID: pgconv.UUIDString(row.RunID), Cites: row.Cites, Preview: row.Preview,
		DecidedUserID: pgconv.UUIDString(row.DecidedBy), DecidedAt: pgconv.RFC3339(row.DecidedAt),
		DecisionNote: deref(row.DecisionNote), StartedAt: pgconv.RFC3339(row.StartedAt), Outcome: deref(row.Outcome),
	})
}

var decisions = map[string]decision{"approve": approve, "reject": reject}

func (h *Handler) DecideProposal(w http.ResponseWriter, r *http.Request) {
	id, ok := proposalID(w, r)
	if !ok {
		return
	}
	var body struct {
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if !decode(w, r, &body) {
		return
	}
	d, known := decisions[body.Decision]
	if !known {
		httpx.WriteError(w, http.StatusBadRequest, "decision must be approve or reject")
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
	err := h.Svc.DecideProposal(r.Context(), id, d, operator, note)
	switch {
	case errors.Is(err, ErrUnknownProposal):
		httpx.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, ErrProposalClosed):
		httpx.WriteError(w, http.StatusConflict, "the proposal is no longer waiting for a decision")
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "proposal update failed")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func proposalID(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "proposal id must be a UUID")
		return id, false
	}
	return id, true
}
