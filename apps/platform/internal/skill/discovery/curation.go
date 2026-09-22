package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

type curationRequest struct {
	Value string `json:"value"`
	Note  string `json:"note"`
}

type curationChangeResponse struct {
	SkillID          string  `json:"skill_id"`
	Tier             string  `json:"tier"`
	CuratedVersionID *string `json:"curated_version_id"`
	PreviousTier     string  `json:"previous_tier"`
}

func (h *Handler) SetCurationTier(w http.ResponseWriter, r *http.Request) {
	var body curationRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with a value and a note")
		return
	}
	var skillID pgtype.UUID
	if err := skillID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, errSkillNotFound.Error())
		return
	}
	user, ok := sessionActor(w, r)
	if !ok {
		return
	}

	change, err := h.Svc.SetCurationTier(r.Context(), skillID, user.ID, body.Value, body.Note)
	var inputErr restrictionInputError
	switch {
	case errors.As(err, &inputErr):
		httpx.WriteError(w, http.StatusBadRequest, inputErr.Error())
		return
	case errors.Is(err, errSkillNotFound):
		httpx.WriteError(w, http.StatusNotFound, errSkillNotFound.Error())
		return
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "curation change failed")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, curationChangeResponse{
		SkillID: pgconv.UUIDString(skillID), Tier: string(change.After),
		CuratedVersionID: optionalUUID(change.VersionID), PreviousTier: string(change.Before),
	})
}

func (s *Service) SetCurationTier(
	ctx context.Context, skillID, actor pgtype.UUID, value, note string,
) (registry.CurationChange, error) {
	note, err := validRestrictionNote(note)
	if err != nil {
		return registry.CurationChange{}, err
	}
	catalogues, err := s.catalogWorkspaceIDs(ctx)
	if err != nil {
		return registry.CurationChange{}, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return registry.CurationChange{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	change, err := registry.SetCuration(ctx, tx, skillID, registry.CurationTier(strings.TrimSpace(value)), catalogues)
	var refused registry.Refused
	switch {
	case errors.Is(err, registry.ErrNotFound):
		return registry.CurationChange{}, errSkillNotFound
	case errors.As(err, &refused):
		return registry.CurationChange{}, curationRefusal(refused)
	case err != nil:
		return registry.CurationChange{}, err
	}

	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        actor,
		Workspace:    change.WorkspaceID,
		Action:       audit.ActionSkillCuration,
		ResourceType: audit.ResourceSkill,
		ResourceID:   skillID,
		Metadata: map[string]any{
			"before": string(change.Before), "before_version_id": optionalUUID(change.BeforeVersionID),
			"after": string(change.After), "version_id": optionalUUID(change.VersionID),
			"note": note,
		},
	}); err != nil {
		return registry.CurationChange{}, err
	}
	if err := s.RefreshListing(ctx, tx, skillID); err != nil {
		return registry.CurationChange{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return registry.CurationChange{}, err
	}
	return change, nil
}

func curationRefusal(refused registry.Refused) error {
	switch refused.Reason {
	case registry.RefusedUnknownCurationTier:
		return restrictionInputError("unknown tier; settable values: curated, indexed")
	case registry.RefusedCurationOutsideCatalog:
		return restrictionInputError(
			"only a skill in a catalogue workspace can be curated: a review is of the catalogue's copy, " +
				"and it does not travel to forks or to skills users imported themselves")
	}
	return refused
}

func optionalUUID(id pgtype.UUID) *string {
	if !id.Valid {
		return nil
	}
	value := pgconv.UUIDString(id)
	return &value
}
