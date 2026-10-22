package governance

import (
	"context"
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

type takedownResponse struct {
	SkillID   string `json:"skill_id"`
	TakenDown bool   `json:"taken_down"`
}

func (h *Handler) Takedown(w http.ResponseWriter, r *http.Request) {
	var body takedownRequest
	if !httpx.DecodeJSON(w, r, maxJSONBodyBytes, &body, "body must be JSON with a reason") {
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

	httpx.WriteJSON(w, http.StatusOK, takedownResponse{SkillID: pgconv.UUIDString(skillID), TakenDown: true})
}

func (h *Handler) LiftTakedown(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeRestrictionRequest(w, r)
	if !ok {
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
	err := h.Svc.LiftTakedown(r.Context(), skillID, user.ID, body.Note)
	var inputErr restrictionInputError
	switch {
	case errors.As(err, &inputErr):
		httpx.WriteError(w, http.StatusBadRequest, inputErr.Error())
	case errors.Is(err, errSkillNotFound):
		httpx.WriteError(w, http.StatusNotFound, errSkillNotFound.Error())
	case errors.Is(err, registry.ErrNotTakenDown):
		httpx.WriteError(w, http.StatusConflict, "this skill is not taken down")
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "lifting the takedown failed")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) LiftTakedown(ctx context.Context, skillID, actor pgtype.UUID, note string) error {
	if s.Registry == nil || s.Registry.IndexSkill == nil {
		return errHookMissing
	}
	note, err := validRestrictionNote(note)
	if err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	before, err := registry.LiftTakedown(ctx, tx, s.Registry, skillID)
	if errors.Is(err, registry.ErrNotFound) {
		return errSkillNotFound
	}
	if err != nil {
		return err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        actor,
		Workspace:    before.WorkspaceID,
		Action:       audit.ActionSkillRestore,
		ResourceType: audit.ResourceSkill,
		ResourceID:   skillID,
		Metadata:     map[string]any{"scope": audit.ScopeOperator, auditNote: note},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
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

	if err := s.removeFromIndex(ctx, tx, before.WorkspaceID, skillID); err != nil {
		return err
	}

	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        actor,
		Workspace:    before.WorkspaceID,
		Action:       audit.ActionSkillTakedown,
		ResourceType: audit.ResourceSkill,
		ResourceID:   skillID,
		Metadata:     map[string]any{"reason": reason, "scope": audit.ScopeOperator},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
