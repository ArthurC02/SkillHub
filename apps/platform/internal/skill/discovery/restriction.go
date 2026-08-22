package catalog

// The operator surface for the 0023 licensing hold (02:SEC-011 追加小節): set a
// hold on a skill's materials, and lift it again.
//
// It lives in this package, next to the reads it governs, because the reason
// codes and the sentences shown for them are here (detail.go, restrictionNotes)
// and a hold set with a code nothing can render is a hold nobody can explain.
// The alternative — a write endpoint in registry with the note map exported —
// buys a tidier package boundary and pays for it with two places that can
// disagree about which codes exist.
//
// 2026-08-20 (ADR-033 clearance path 4) split that finer than "all here or all
// there", and the objection above survives untouched: the reason codes, the
// sentences, the two HTTP routes, the authorization check and the audit event
// are all still in this file, so there is still exactly one place that knows
// which codes exist. What moved is the UPDATE of skills.access_restriction,
// which is now registry.SetAccessRestriction — `skills` is registry's table, and
// writing one column of it is not a policy anyone can disagree with this file
// about. It takes this package's transaction, so the lock, the write and the
// audit event are still one commit.
//
// 2026-08-21 (DDD-031, ADR-035 B 組) moved the other half of the same statement
// pair: the SELECT ... FOR UPDATE was still issued here, one line before the
// call, so registry could not tell whether the row it was about to write had
// been locked. It is now inside registry.SetAccessRestriction too, and the
// before-state this file records in the audit event is that function's return
// value rather than something this file read for itself. Same transaction, same
// commit, same division of decisions — this file just stopped holding a lock on
// somebody else's invariant.
//
// Scope of what this replaces: before it, setting or lifting a hold meant a
// reviewer running tools/content/restrict-anthropic-sa-display.sql by hand. That
// path had no authorization check and left no audit event, which is precisely
// what 02:SEC-011 exists to stop. The SQL script stays for what it is good at —
// applying the original decision in bulk, forks of forks included.
//
// Deliberately not covered here: the other two operator actions SEC-011
// enumerates (disabling a Skill Version, whitelist changes). Neither has a
// mechanism to drive yet, and an endpoint for an action nothing can perform is a
// promise, not a feature. See 03:SEC-011 for the honest state.

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

// maxOperatorNoteBytes caps the free-text note. It is the one piece of operator
// prose that reaches the audit trail, and the trail is meant to stay small
// enough that 400 day retention is cheap (see internal/foundation/observability/audit).
const maxOperatorNoteBytes = 1000

// restrictionRequest is the body of both operator routes. Both fields are
// required on PUT; DELETE reads only Note, because there is no code to name when
// the answer is "no hold at all" — but it still has to say why (02:SEC-011
// 「理由（必填，空字串不成立）」).
type restrictionRequest struct {
	Reason string `json:"reason"`
	Note   string `json:"note"`
}

// SetRestriction handles PUT /admin/skills/{id}/restriction: put the named
// licensing hold on a skill, or change the reason of one already in place.
// Idempotent — applying the same code twice is a second audit event and no
// change to the row, not a conflict, because an operator repeating an action is
// not an error and 409 here would just invite retry loops.
func (h *Handler) SetRestriction(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeRestrictionRequest(w, r)
	if !ok {
		return
	}
	h.changeRestriction(w, r, &body.Reason, body.Note)
}

// ClearRestriction handles DELETE /admin/skills/{id}/restriction: the "終判允許
// 後把欄位設回 NULL" half of 0023. Idempotent by construction — lifting a hold
// that was never there writes the same audit event with a null before-state and
// answers 204, because the caller's intent (this skill must not be held) is
// satisfied either way.
func (h *Handler) ClearRestriction(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeRestrictionRequest(w, r)
	if !ok {
		return
	}
	h.changeRestriction(w, r, nil, body.Note)
}

// changeRestriction is the shared handler half: decode the route identity and
// map the application result onto HTTP. The Service owns every restriction
// invariant, including for callers that do not enter through HTTP.
func (h *Handler) changeRestriction(w http.ResponseWriter, r *http.Request, reason *string, note string) {
	var skillID pgtype.UUID
	if err := skillID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, errSkillNotFound.Error())
		return
	}
	user, _ := identity.SessionUser(r.Context())

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
	// The response echoes the sentence the public detail view will now show, so
	// the operator sees what the user sees without having to fetch the skill.
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"skill_id": pgconv.UUIDString(skillID),
		"access_restriction": accessRestriction{
			Reason: normalizedReason,
			Note:   restrictionNotes[normalizedReason],
		},
		"previous_reason": nullableString(previous),
	})
}

// restrictionInputError marks caller input that maps to HTTP 400 while keeping
// the validation available to every Service caller.
type restrictionInputError string

func (e restrictionInputError) Error() string { return string(e) }

// SetRestriction validates and normalizes a hold before choosing the audit
// action. Callers cannot forge a different action for this domain operation.
func (s *Service) SetRestriction(ctx context.Context, skillID, actor pgtype.UUID, reason, note string) (*string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, restrictionInputError("reason is required")
	}
	// Reads fail closed for unknown stored codes, but writes reject them so the
	// public explanation never silently falls back to a generic sentence.
	if _, known := restrictionNotes[reason]; !known {
		return nil, restrictionInputError("unknown reason code; known codes: " + strings.Join(knownRestrictionReasons(), ", "))
	}
	note, err := validRestrictionNote(note)
	if err != nil {
		return nil, err
	}
	return s.changeRestriction(ctx, skillID, actor, &reason, note)
}

// ClearRestriction validates the operator explanation and chooses the only
// audit action valid for lifting a hold.
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

// changeRestriction is the shared write: change the column through its owner
// (which locks the row and hands back the before-state), write the audit event,
// commit. One transaction, so a hold can never be in force
// without the event that explains it, or explained without being in force
// (iron rule 9). It returns the reason that was in force before, which is what
// the response echoes back to the operator.
func (s *Service) changeRestriction(ctx context.Context, skillID, actor pgtype.UUID, reason *string, note string) (*string, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// registry owns the column; this package owns the decision (see the file
	// comment). Same transaction, so the audit event below still commits with it.
	//
	// The lock that makes `before` the true before-state is taken inside that
	// call, not here (DDD-031). This package used to take it one statement
	// earlier, which meant the owner of the write could not guarantee its own
	// writes were serialised — see registry.SetAccessRestriction. What this file
	// lost is one query call; what it kept is every decision it had.
	before, err := registry.SetAccessRestriction(ctx, tx, skillID, reason)
	if errors.Is(err, registry.ErrNotFound) {
		return nil, errSkillNotFound
	}
	if err != nil {
		return nil, err
	}
	// The note is operator prose about a platform decision, not user content, so
	// it belongs in the event rather than beside it: 02:SEC-011 requires the
	// reason to be *in* the audit event, and the skills row has no column for one
	// (unlike a takedown, whose reason lives on the row).
	//
	// workspace_id is the affected skill's, which is how a cross-workspace action
	// stays reviewable per workspace. Recording it is not scope: nothing about
	// that workspace was read.
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

// nullableString keeps "no hold" as JSON null rather than "". An empty string in
// an audit event's before-state would read as a hold with a blank reason, which
// is a state 0023's CHECK constraint makes impossible.
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
