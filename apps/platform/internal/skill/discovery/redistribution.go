package catalog

// The operator surface for the redistribution verdict (02:SEC-007, 0027 extended
// by 0036). `04` 乙-17 / `05` R-3c.
//
// WHY THIS EXISTS. Two columns decide whether the platform will hand a skill's
// bytes to somebody: access_restriction (a reviewer's temporary hold) and
// redistribution (whether the licence permits copying at all). They gate the
// same download. Until now the first had a route, an operator check and an audit
// event, and the second had none of it — changing it meant running UPDATE by
// hand, leaving no record of who decided or why.
//
// The weaker reason was the governed one. That is the asymmetry this closes, and
// closing it does not depend on the two questions still open next to it: who may
// call this (`05` R-3a) and what evidence a release requires (R-3b). Whatever
// those answers turn out to be, the change should leave a record — so the record
// is not worth waiting for them.
//
// WHAT IT DELIBERATELY DOES NOT DECIDE. The route is operator-only, which is the
// narrower of R-3a's two options and therefore the one that cannot foreclose the
// other: widening it later adds callers, where starting wide and narrowing later
// would take something away. And the request carries a free-text note, exactly
// like a restriction change — NOT the source-tier evidence R-3b may come to
// require. Encoding a guess at that ruling here would be the more expensive
// mistake, because a half-enforced evidence rule reads like an enforced one.
//
// Lives in this package for the reason the file next door gives: the sentences a
// reader sees for each value are here (trust.go, redistributionDisplays), and a
// value written through a path that cannot render it is a verdict nobody can
// explain.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

// operatorSettableRedistribution is the subset of the column's four values an
// operator may assert.
//
// `self_supplied` is absent, and its absence is the interesting part: 0036
// defines it as a fact about who supplied the bytes, established by the import
// path at the moment the workspace uploaded them. It is not a verdict about a
// licence, so there is nobody who could be right in asserting it after the fact
// — an operator who typed it would be claiming the platform received bytes it
// did not receive. The column's CHECK still permits it, because import must
// write it; this route does not.
//
// Setting a self_supplied skill to `blocked` is allowed, and that direction is
// deliberate: content the owner uploaded can still turn out to be something the
// platform must stop handing back.
var operatorSettableRedistribution = map[string]struct{}{
	string(RedistributionAllowed): {},
	string(RedistributionBlocked): {},
	string(RedistributionUnknown): {},
}

// redistributionRequest is the body of PUT /admin/skills/{id}/redistribution.
// Both fields are required: an operator action nobody can explain later is not
// a decision (02:SEC-011's rule for the neighbouring route, applied here for the
// same reason).
type redistributionRequest struct {
	Value string `json:"value"`
	Note  string `json:"note"`
}

// SetRedistribution handles PUT /admin/skills/{id}/redistribution.
//
// Idempotent: writing the value a skill already has is a second audit event and
// no change to the row. Same choice as the restriction routes — an operator
// repeating an action is not an error, and a 409 would only invite retry loops.
func (h *Handler) SetRedistribution(w http.ResponseWriter, r *http.Request) {
	var body redistributionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with a value and a note")
		return
	}

	var skillID pgtype.UUID
	if err := skillID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, errSkillNotFound.Error())
		return
	}
	user, _ := identity.SessionUser(r.Context())

	previous, err := h.Svc.SetRedistribution(r.Context(), skillID, user.ID, body.Value, body.Note)
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
		httpx.WriteError(w, http.StatusInternalServerError, "redistribution change failed")
		return
	}

	value := Redistribution(strings.TrimSpace(body.Value))
	display := value.Display()
	// The response echoes the sentence the public detail view will now show, so
	// the operator can see what a reader will see without fetching the skill.
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"skill_id": pgconv.UUIDString(skillID),
		"redistribution": map[string]any{
			"value": string(value),
			"label": display.Label,
			"note":  display.Note,
		},
		"previous_value": previous,
	})
}

// SetRedistribution validates the verdict and the explanation, then writes both
// the column and the event that accounts for it. Every invariant is here rather
// than in the handler so a non-HTTP caller cannot skip one.
func (s *Service) SetRedistribution(ctx context.Context, skillID, actor pgtype.UUID, value, note string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", restrictionInputError("value is required")
	}
	if _, ok := operatorSettableRedistribution[value]; !ok {
		// self_supplied is named in the message rather than lumped in with
		// typos: a caller who tried it made a category error, not a spelling
		// mistake, and telling them the list of valid values would not explain
		// why the one they picked is missing from it.
		if value == string(RedistributionSelfSupplied) {
			return "", restrictionInputError(
				"self_supplied is not a verdict anyone can assert: it records that this " +
					"workspace supplied the bytes, and only the import path can establish that")
		}
		return "", restrictionInputError(
			"unknown redistribution value; settable values: allowed, blocked, unknown")
	}
	note, err := validRestrictionNote(note)
	if err != nil {
		return "", err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	before, err := registry.SetRedistribution(ctx, tx, skillID, value)
	if errors.Is(err, registry.ErrNotFound) {
		return "", errSkillNotFound
	}
	if err != nil {
		return "", err
	}
	// One transaction with the write above. This gate decides whether content
	// leaves the platform, so "released, and no record of who released it" is
	// the one outcome that must be impossible (iron rule 9).
	//
	// workspace_id is the affected skill's, which is how a cross-workspace
	// operator action stays reviewable per workspace. Recording it is not a
	// scope violation: nothing about that workspace's contents was read.
	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        actor,
		Workspace:    before.WorkspaceID,
		Action:       audit.ActionSkillRedistribution,
		ResourceType: audit.ResourceSkill,
		ResourceID:   skillID,
		Metadata: map[string]any{
			"before": before.Redistribution,
			"after":  value,
			"note":   note,
		},
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return before.Redistribution, nil
}
