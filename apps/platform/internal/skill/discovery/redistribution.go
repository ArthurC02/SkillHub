package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

var provenanceRedistribution = map[string]string{
	string(RedistributionSelfSupplied): "self_supplied is not a verdict anyone can assert: it records that this " +
		"workspace supplied the bytes, and only the import path can establish that",
	string(RedistributionGenerated): "generated is not a verdict anyone can assert: it records that the platform " +
		"wrote these bytes for this workspace, and only the generation path can establish that",
}

type redistributionRequest struct {
	Value             string `json:"value"`
	Note              string `json:"note"`
	LicenseExpression string `json:"license_expression"`
	LicenseSource     string `json:"license_source"`
}

type redistributionView struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Note  string `json:"note"`
}

type redistributionChangeResponse struct {
	SkillID        string             `json:"skill_id"`
	Redistribution redistributionView `json:"redistribution"`
	PreviousValue  string             `json:"previous_value"`
}

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
	user, ok := sessionActor(w, r)
	if !ok {
		return
	}

	previous, err := h.Svc.SetRedistribution(r.Context(), skillID, user.ID, body.Value, body.Note,
		registry.LicenseClaim{Expression: body.LicenseExpression, Source: body.LicenseSource})
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

	httpx.WriteJSON(w, http.StatusOK, redistributionChangeResponse{
		SkillID: pgconv.UUIDString(skillID), Redistribution: redistributionView{
			Value: string(value), Label: display.Label, Note: display.Note,
		},
		PreviousValue: previous,
	})
}

func (s *Service) SetRedistribution(
	ctx context.Context, skillID, actor pgtype.UUID, value, note string, claim registry.LicenseClaim,
) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", restrictionInputError("value is required")
	}
	requested := registry.Redistribution(value)
	if reason, refused := requested.OperatorRefusal(); refused {
		return "", redistributionRefusal(registry.Refused{Reason: reason}, value)
	}
	note, err := validRestrictionNote(note)
	if err != nil {
		return "", err
	}
	if requested == registry.RedistributionAllowed && !claim.Complete() {
		return "", redistributionRefusal(registry.Refused{Reason: registry.RefusedLicenseClaimMissing}, value)
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	before, err := registry.SetRedistribution(ctx, tx, skillID, value, claim)
	var refused registry.Refused
	switch {
	case errors.Is(err, registry.ErrNotFound):
		return "", errSkillNotFound
	case errors.As(err, &refused):
		return "", redistributionRefusal(refused, value)
	case err != nil:
		return "", err
	}

	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        actor,
		Workspace:    before.WorkspaceID,
		Action:       audit.ActionSkillRedistribution,
		ResourceType: audit.ResourceSkill,
		ResourceID:   skillID,
		Metadata:     redistributionMetadata(before.Redistribution, value, note, before.Verified),
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return before.Redistribution, nil
}

func redistributionRefusal(refused registry.Refused, value string) error {
	switch refused.Reason {
	case registry.RefusedProvenanceNotAssertable:
		return restrictionInputError(provenanceRedistribution[value])
	case registry.RefusedUnknownRedistribution:
		return restrictionInputError(
			"unknown redistribution value; settable values: allowed, blocked, unknown")
	case registry.RefusedLicenseClaimMissing:
		return restrictionInputError(
			"releasing a skill requires the licence evidence it relies on: license_expression and " +
				"license_source, as recorded on the skill's newest version (05 R-3b: a confirmation " +
				"box is not evidence)")
	case registry.RefusedNoLicenseRecorded:
		return restrictionInputError(
			"this skill's newest version records no licence, so there is no evidence to rely on; " +
				"re-import it if the package carries one (writing a tier onto an existing " +
				"row would be inventing the evidence rather than reading it)")
	case registry.RefusedLicenseMismatch:
		return restrictionInputError(fmt.Sprintf(
			"licence evidence does not match what this skill's newest version records (recorded: "+
				"%s from %s); releasing it would assert a licence the platform never read",
			refused.Recorded.Expression, refused.Recorded.Source))
	case registry.RefusedGeneratedIsPermanent:
		return restrictionInputError(
			"this skill was generated by the platform, and `generated` is the only record of that. " +
				"Overwriting it would put the skill back into its workspace's search results and into " +
				"the enrichment queue (GEN-007 keys on this column). To stop it being read, run or " +
				"packaged, set access_restriction instead — that holds the content without erasing " +
				"where it came from")
	}
	return refused
}

func redistributionMetadata(before, after, note string, verified registry.LicenseClaim) map[string]any {
	m := map[string]any{"before": before, "after": after, "note": note}
	if after == string(RedistributionAllowed) {
		m["license_expression"] = verified.Expression
		m["license_source"] = verified.Source
	}
	return m
}
