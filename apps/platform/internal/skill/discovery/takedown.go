package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

type takedownRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) Takedown(w http.ResponseWriter, r *http.Request) {
	var body takedownRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with a reason")
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

	err := h.Svc.Takedown(r.Context(), skillID, user.ID, body.Reason)
	var inputErr restrictionInputError
	switch {
	case errors.As(err, &inputErr):
		httpx.WriteError(w, http.StatusBadRequest, inputErr.Error())
		return
	case errors.Is(err, errSkillNotFound):
		httpx.WriteError(w, http.StatusNotFound, errSkillNotFound.Error())
		return
	case errors.Is(err, registry.ErrAlreadyTakenDown):
		httpx.WriteError(w, http.StatusConflict, "this skill is already taken down")
		return
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "takedown failed")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"skill_id":   pgconv.UUIDString(skillID),
		"taken_down": true,
	})
}

func (s *Service) Takedown(ctx context.Context, skillID, actor pgtype.UUID, reason string) error {
	reason, err := validRestrictionNote(reason)
	if err != nil {
		return err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	before, err := registry.SetTakedown(ctx, tx, skillID, reason)
	if errors.Is(err, registry.ErrNotFound) {
		return errSkillNotFound
	}
	if err != nil {
		return err
	}

	if err := RemoveSkillFromIndex(ctx, tx, before.WorkspaceID, skillID); err != nil {
		return err
	}

	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        actor,
		Workspace:    before.WorkspaceID,
		Action:       audit.ActionSkillTakedown,
		ResourceType: audit.ResourceSkill,
		ResourceID:   skillID,
		Metadata:     map[string]any{"reason": reason, "scope": "operator"},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
