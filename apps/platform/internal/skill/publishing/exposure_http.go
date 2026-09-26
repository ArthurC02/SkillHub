package publishing

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

type exposureReleaseView struct {
	ReleaseID     string `json:"release_id"`
	VersionID     string `json:"version_id"`
	VersionNumber int32  `json:"version_number"`
	ContentHash   string `json:"content_hash"`
	ReleasedAt    string `json:"released_at"`
}

type exposureQueueEntry struct {
	Publisher     string              `json:"publisher"`
	Name          string              `json:"name"`
	Address       string              `json:"address"`
	Release       exposureReleaseView `json:"release"`
	Sequence      int32               `json:"sequence"`
	ReviewedAgain bool                `json:"reviewed_again"`
}

type searchSnapshotView struct {
	VersionID       string          `json:"version_id"`
	Current         bool            `json:"current"`
	Name            string          `json:"name"`
	Summary         string          `json:"summary"`
	EnrichedSummary string          `json:"enriched_summary"`
	TaskExamples    string          `json:"task_examples"`
	Tags            json.RawMessage `json:"tags"`
	Limitations     string          `json:"limitations"`
	Enriched        bool            `json:"enriched"`
	Digest          string          `json:"digest"`
}

type exposureReviewView struct {
	Sequence       int32  `json:"sequence"`
	ReleaseID      string `json:"release_id"`
	ContentHash    string `json:"content_hash"`
	SnapshotDigest string `json:"snapshot_digest"`
	Decision       string `json:"decision"`
	Reason         string `json:"reason"`
	ReviewerUserID string `json:"reviewer_user_id"`
	ReviewedAt     string `json:"reviewed_at"`
}

type exposureCaseView struct {
	Publisher string               `json:"publisher"`
	Name      string               `json:"name"`
	Address   string               `json:"address"`
	Status    string               `json:"status"`
	Release   exposureReleaseView  `json:"release"`
	Sequence  int32                `json:"sequence"`
	Exposed   bool                 `json:"exposed"`
	Snapshot  *searchSnapshotView  `json:"snapshot,omitempty"`
	History   []exposureReviewView `json:"history"`
}

func exposureRelease(state ExposureState) exposureReleaseView {
	return exposureReleaseView{
		ReleaseID: pgconv.UUIDString(state.ReleaseID), VersionID: pgconv.UUIDString(state.VersionID),
		VersionNumber: state.VersionNumber, ContentHash: state.ContentHash, ReleasedAt: timestamp(state.ReleasedAt),
	}
}

func exposureCaseViewOf(c ExposureCase) exposureCaseView {
	view := exposureCaseView{
		Publisher: c.State.Publisher, Name: c.State.Name, Address: address(c.State.Publisher, c.State.Name),
		Status: string(c.State.Status), Release: exposureRelease(c.State), Sequence: c.State.Sequence,
		Exposed: c.Exposed, History: make([]exposureReviewView, 0, len(c.History)),
	}
	if snap := c.Snapshot; snap != nil {
		view.Snapshot = &searchSnapshotView{
			VersionID: pgconv.UUIDString(snap.VersionID), Current: snap.VersionID == c.State.VersionID,
			Name: snap.Name, Summary: snap.Summary, EnrichedSummary: snap.EnrichedSummary,
			TaskExamples: snap.TaskExamples, Tags: snap.Tags, Limitations: snap.Limitations,
			Enriched: snap.Enriched, Digest: snap.Digest,
		}
	}
	for _, r := range c.History {
		view.History = append(view.History, exposureReviewView{
			Sequence: r.Sequence, ReleaseID: pgconv.UUIDString(r.ReleaseID), ContentHash: r.ContentHash,
			SnapshotDigest: r.SnapshotDigest, Decision: string(r.Decision), Reason: r.Reason,
			ReviewerUserID: pgconv.UUIDString(r.ReviewerUserID), ReviewedAt: timestamp(r.ReviewedAt),
		})
	}
	return view
}

func (h *Handler) ExposureQueue(w http.ResponseWriter, r *http.Request) {
	queue, err := h.Svc.ExposureQueue(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "exposure queue lookup failed")
		return
	}
	out := make([]exposureQueueEntry, 0, len(queue))
	for _, state := range queue {
		out = append(out, exposureQueueEntry{
			Publisher: state.Publisher, Name: state.Name, Address: address(state.Publisher, state.Name),
			Release: exposureRelease(state), Sequence: state.Sequence, ReviewedAgain: state.Concluded,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"publications": out})
}

func (h *Handler) ExposureCase(w http.ResponseWriter, r *http.Request) {
	c, found, err := h.Svc.ExposureCase(r.Context(), r.PathValue("publisher"), r.PathValue("name"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "exposure case lookup failed")
		return
	}
	if !found {
		httpx.WriteError(w, http.StatusNotFound, "no Skill publication has this address")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, exposureCaseViewOf(c))
}

func (h *Handler) ReviewExposure(w http.ResponseWriter, r *http.Request) {
	reviewer, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var req struct {
		ReleaseID        string `json:"release_id"`
		ExpectedSequence int32  `json:"expected_sequence"`
		Decision         string `json:"decision"`
		Reason           string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "the body must be a JSON object")
		return
	}
	var releaseID pgtype.UUID
	if err := releaseID.Scan(req.ReleaseID); err != nil {
		writeReason(w, http.StatusConflict, string(ExposureStale), exposureProblemWords[ExposureStale])
		return
	}
	c, err := h.Svc.ReviewExposure(r.Context(), reviewer, r.PathValue("publisher"), r.PathValue("name"), ExposureInput{
		ReleaseID: releaseID, ExpectedSequence: req.ExpectedSequence,
		Decision: ExposureDecision(req.Decision), Reason: req.Reason,
	})
	var problem *ExposureError
	switch {
	case err == nil:
		httpx.WriteJSON(w, http.StatusOK, exposureCaseViewOf(c))
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "no Skill publication has this address")
	case errors.As(err, &problem) && problem.Problem == ExposureStale:
		writeReason(w, http.StatusConflict, string(problem.Problem), problem.Error())
	case errors.As(err, &problem):
		writeReason(w, http.StatusUnprocessableEntity, string(problem.Problem), problem.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "exposure review failed")
	}
}
