package apiserver

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/activity"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/delivery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/publishing"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

type activityHandler struct {
	Svc      *activity.Service
	Identity *identity.Service
}

type activityUnavailableResponse struct {
	Complete           bool              `json:"complete"`
	UnavailableSources []activity.Source `json:"unavailable_sources"`
	Error              string            `json:"error"`
}

func (h *activityHandler) List(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			httpx.WriteError(w, http.StatusBadRequest, "limit must be between 1 and 100")
			return
		}
		limit = value
	}
	ws, err := h.Identity.PersonalWorkspace(r.Context(), user)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "workspace lookup failed")
		return
	}
	page, err := h.Svc.List(r.Context(), ws.ID, r.URL.Query().Get("cursor"), limit)
	if errors.Is(err, activity.ErrInvalidCursor) {
		httpx.WriteError(w, http.StatusBadRequest, "cursor must be one this endpoint returned")
		return
	}
	var unavailable *activity.UnavailableError
	if errors.As(err, &unavailable) {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, activityUnavailableResponse{
			Complete: false, UnavailableSources: unavailable.Sources,
			Error: "完整活動目前無法讀取，請稍後再試。",
		})
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "activity lookup failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, page)
}

func newActivityService(
	runs *run.Service,
	evaluations *eval.Service,
	creations *creation.Service,
	artifacts *packaging.Service,
	publications *publishing.Service,
) *activity.Service {
	service := &activity.Service{}
	service.ReadRuns = func(ctx context.Context, workspaceID pgtype.UUID) ([]activity.RunFact, error) {
		facts, err := runs.ActivityFacts(ctx, workspaceID)
		out := make([]activity.RunFact, len(facts))
		for i, fact := range facts {
			out[i] = activity.RunFact{
				RunID: pgconv.UUIDString(fact.RunID), SkillID: pgconv.UUIDString(fact.SkillID),
				SkillName: fact.SkillName, SkillVersionID: pgconv.UUIDString(fact.SkillVersionID),
				TestCaseID:     pgconv.UUIDString(fact.TestCaseID),
				Classification: activity.Classification(fact.Classification),
				Status:         activity.Status{Value: fact.Status, Label: fact.StatusLabel}, ActivityAt: fact.ActivityAt,
			}
		}
		return out, err
	}
	service.ReadEvaluations = func(ctx context.Context, workspaceID pgtype.UUID) ([]activity.EvaluationFact, error) {
		facts, err := evaluations.ActivityFacts(ctx, workspaceID)
		out := make([]activity.EvaluationFact, len(facts))
		for i, fact := range facts {
			out[i] = activity.EvaluationFact{
				RunID: pgconv.UUIDString(fact.RunID), Classification: activity.Classification(fact.Classification),
				Status: activity.Status{Value: fact.Status, Label: fact.StatusLabel}, ActivityAt: fact.ActivityAt,
			}
		}
		return out, err
	}
	service.ReadCreations = func(ctx context.Context, workspaceID pgtype.UUID) ([]activity.CreationFact, error) {
		facts, err := creations.ActivityFacts(ctx, identity.Workspace{ID: workspaceID})
		out := make([]activity.CreationFact, len(facts))
		for i, fact := range facts {
			out[i] = activity.CreationFact{
				SessionID: pgconv.UUIDString(fact.SessionID), Summary: fact.Summary,
				Classification: activity.Classification(fact.Classification),
				Status:         activity.Status{Value: fact.Status, Label: fact.StatusLabel}, ActivityAt: fact.ActivityAt,
			}
		}
		return out, err
	}
	service.ReadPackaging = func(ctx context.Context, workspaceID pgtype.UUID) ([]activity.PackagingFact, error) {
		facts, err := artifacts.ActivityFacts(ctx, identity.Workspace{ID: workspaceID})
		out := make([]activity.PackagingFact, len(facts))
		for i, fact := range facts {
			out[i] = activity.PackagingFact{
				ArtifactID: pgconv.UUIDString(fact.ArtifactID), SkillVersionID: pgconv.UUIDString(fact.SkillVersionID),
				FileName: fact.FileName, Classification: activity.Classification(fact.Classification),
				Status: activity.Status{Value: fact.Status, Label: fact.StatusLabel}, ActivityAt: fact.ActivityAt,
			}
		}
		return out, err
	}
	service.ReadPublishing = func(ctx context.Context, workspaceID pgtype.UUID) ([]activity.PublishingFact, error) {
		facts, err := publications.ActivityFacts(ctx, identity.Workspace{ID: workspaceID})
		out := make([]activity.PublishingFact, len(facts))
		for i, fact := range facts {
			out[i] = activity.PublishingFact{
				PublicationID: pgconv.UUIDString(fact.PublicationID), SkillID: pgconv.UUIDString(fact.SkillID),
				LatestVersionID: pgconv.UUIDString(fact.LatestVersionID), Publisher: fact.Publisher, Name: fact.Name,
				Classification: activity.Classification(fact.Classification),
				Status:         activity.Status{Value: fact.Status, Label: fact.StatusLabel}, ActivityAt: fact.ActivityAt,
			}
		}
		return out, err
	}
	return service
}
