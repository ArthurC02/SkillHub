package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

const maxOperatorNoteBytes = 1000

type restrictionRequest struct {
	Reason string `json:"reason"`
	Note   string `json:"note"`
}

func (h *Handler) SetRestriction(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeRestrictionRequest(w, r)
	if !ok {
		return
	}
	h.changeRestriction(w, r, &body.Reason, body.Note)
}

func (h *Handler) ClearRestriction(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeRestrictionRequest(w, r)
	if !ok {
		return
	}
	h.changeRestriction(w, r, nil, body.Note)
}

func sessionActor(w http.ResponseWriter, r *http.Request) (identity.User, bool) {
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return user, false
	}
	return user, true
}

func (h *Handler) changeRestriction(w http.ResponseWriter, r *http.Request, reason *string, note string) {
	var skillID pgtype.UUID
	if err := skillID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, errSkillNotFound.Error())
		return
	}
	user, ok := sessionActor(w, r)
	if !ok {
		return
	}

	var (
		previous *string
		err      error
	)
	if reason == nil {
		previous, err = h.Svc.ClearRestriction(r.Context(), skillID, user.ID, note)
	} else {
		previous, err = h.Svc.SetRestriction(r.Context(), skillID, user.ID, *reason, note)
	}
	var inputErr restrictionInputError
	if errors.As(err, &inputErr) {
		httpx.WriteError(w, http.StatusBadRequest, inputErr.Error())
		return
	}
	if errors.Is(err, errSkillNotFound) {
		httpx.WriteError(w, http.StatusNotFound, errSkillNotFound.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "restriction change failed")
		return
	}

	if reason == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	normalizedReason := strings.TrimSpace(*reason)

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"skill_id": pgconv.UUIDString(skillID),
		"access_restriction": accessRestriction{
			Reason: normalizedReason,
			Note:   restrictionNotes[normalizedReason],
		},
		"previous_reason": nullableString(previous),
	})
}

type restrictionInputError string

func (e restrictionInputError) Error() string { return string(e) }

func (s *Service) SetRestriction(ctx context.Context, skillID, actor pgtype.UUID, reason, note string) (*string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, restrictionInputError("reason is required")
	}

	if _, known := restrictionNotes[reason]; !known {
		return nil, restrictionInputError("unknown reason code; known codes: " + strings.Join(knownRestrictionReasons(), ", "))
	}
	note, err := validRestrictionNote(note)
	if err != nil {
		return nil, err
	}
	return s.changeRestriction(ctx, skillID, actor, &reason, note)
}

func (s *Service) ClearRestriction(ctx context.Context, skillID, actor pgtype.UUID, note string) (*string, error) {
	note, err := validRestrictionNote(note)
	if err != nil {
		return nil, err
	}
	return s.changeRestriction(ctx, skillID, actor, nil, note)
}

func validRestrictionNote(note string) (string, error) {
	note = strings.TrimSpace(note)
	if note == "" {
		return "", restrictionInputError("note is required: an operator action nobody can explain later is not a decision")
	}
	if len(note) > maxOperatorNoteBytes {
		return "", restrictionInputError("note is too long")
	}
	return note, nil
}

func (s *Service) changeRestriction(ctx context.Context, skillID, actor pgtype.UUID, reason *string, note string) (*string, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	before, err := registry.SetAccessRestriction(ctx, tx, skillID, reason)
	if errors.Is(err, registry.ErrNotFound) {
		return nil, errSkillNotFound
	}
	if err != nil {
		return nil, err
	}

	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        actor,
		Workspace:    before.WorkspaceID,
		Action:       restrictionAuditAction(reason),
		ResourceType: audit.ResourceSkill,
		ResourceID:   skillID,
		Metadata: map[string]any{
			"before": nullableString(before.AccessRestriction),
			"after":  nullableString(reason),
			"note":   note,
		},
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return before.AccessRestriction, nil
}

func restrictionAuditAction(reason *string) string {
	if reason == nil {
		return audit.ActionSkillUnrestrict
	}
	return audit.ActionSkillRestrict
}

func decodeRestrictionRequest(w http.ResponseWriter, r *http.Request) (restrictionRequest, bool) {
	var body restrictionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with a note")
		return restrictionRequest{}, false
	}
	return body, true
}

func nullableString(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func knownRestrictionReasons() []string {
	out := make([]string, 0, len(restrictionNotes))
	for code := range restrictionNotes {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}
