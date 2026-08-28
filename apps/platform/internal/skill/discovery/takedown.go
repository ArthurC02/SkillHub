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

// 02:SEC-011 動作 ①, the half that reaches other people's workspaces.
//
// The owner-scoped takedown has existed since INGEST-010: a curator withdraws
// content from the workspace they own. What had no path at all was the other
// case — an abuse report or a DMCA notice about a fork sitting in somebody
// else's workspace. registry.go has carried a comment saying so, and saying
// exactly how to fix it, since the method was written; 02:SEC-011 has named the
// actor since 2026-08-16. What was missing was one statement and one route.
//
// It is deliberately not a second flow. Same `takedown_at`, so the same 410
// Gone answers the detail view and the same predicate keeps it out of search —
// neither of those reads asks who set it. 02:533 forbids operators a second
// takedown mechanism, and sharing the column is what makes that structural
// rather than a rule somebody has to remember.
//
// Not idempotent, and this is the one place these operator routes diverge from
// the restriction and redistribution ones. Those write a value; this one records
// an event that happened at a time — `takedown_at` is a timestamp, and letting a
// repeat overwrite it would move the date a review committee is going to ask
// about. The owner-scoped path answers 409 for the same reason and this shares
// its wording.
//
// There is no restore route here, and there is none on the owner-scoped path
// either. Clearing the flag is the easy half; putting the search document back
// is not, because IndexSkill writes only name and summary and would silently
// drop the enrichment, the embedding and the scan the row used to carry. Today
// the answer is `maintenance reindex` after clearing the column. Recorded in
// `04` 丙-80 rather than half-built here.
type takedownRequest struct {
	Reason string `json:"reason"`
}

// Takedown handles PUT /admin/skills/{id}/takedown.
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
	user, ok := operatorUser(w, r)
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

// Takedown writes the flag, drops the search document and records the event, in
// one transaction. Every invariant is here rather than in the handler so a
// non-HTTP caller cannot skip one.
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
	// Same transaction as the flag, so the content can never be down in the
	// registry and still listed in search. This is catalog's own table, so
	// unlike the owner-scoped path there is nothing to inject.
	if err := RemoveSkillFromIndex(ctx, tx, skillID); err != nil {
		return err
	}
	// The reason travels in the metadata as well as onto the row, which is where
	// this differs from the owner-scoped takedown's identifiers-only event.
	// 02:SEC-011 requires an operator action to record its reason in the audit
	// event and requires it to be non-empty; an operator's own sentence about
	// why they acted is not package content, so PDM-006 §6「不含內容」is not what
	// forbids it. workspace_id is the affected skill's, which is how a
	// cross-workspace action stays reviewable per workspace — recording it reads
	// nothing about that workspace's contents.
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
