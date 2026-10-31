package governance

import (
	"context"
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
	string(registry.RedistributionSelfSupplied): "自行提供不是可以手動指定的判定：它記錄的是這個工作區自己提供了套件內容，" +
		"只有匯入流程能確立",
	string(registry.RedistributionGenerated): "平台生成不是可以手動指定的判定：它記錄的是平台替這個工作區寫出了內容，" +
		"只有生成流程能確立",
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
	if !httpx.DecodeJSON(w, r, maxJSONBodyBytes, &body, "body must be JSON with a value and a note") {
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

	previous, err := h.Svc.SetRedistribution(r.Context(), skillID, user.ID, RedistributionChange{
		Value: body.Value, Note: body.Note,
		Claim: registry.LicenseClaim{Expression: body.LicenseExpression, Source: body.LicenseSource},
	})
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

	value := strings.TrimSpace(body.Value)
	label, note := h.Svc.RedistributionDisplay(value)

	httpx.WriteJSON(w, http.StatusOK, redistributionChangeResponse{
		SkillID: pgconv.UUIDString(skillID), Redistribution: redistributionView{
			Value: value, Label: label, Note: note,
		},
		PreviousValue: previous,
	})
}

type RedistributionChange struct {
	Value string
	Note  string
	Claim registry.LicenseClaim
}

func (s *Service) SetRedistribution(
	ctx context.Context, skillID, actor pgtype.UUID, change RedistributionChange,
) (string, error) {
	note, claim := change.Note, change.Claim
	value := strings.TrimSpace(change.Value)
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
		return restrictionInputError("不認得的再散布判定；可以設定的只有 allowed、blocked、unknown")
	case registry.RefusedLicenseClaimMissing:
		return restrictionInputError(
			"判定可以再散布要附上依據的授權證據：授權條款與出處，須與這個小工具最新版本記錄的相同")
	case registry.RefusedNoLicenseRecorded:
		return restrictionInputError(
			"這個小工具的最新版本沒有記錄任何授權，沒有證據可以依據；套件裡有授權檔的話，請重新匯入")
	case registry.RefusedLicenseMismatch:
		return restrictionInputError(fmt.Sprintf(
			"授權證據與最新版本記錄的不一致（記錄的是 %s，出處 %s）；照這樣放行等於宣稱一個平台沒讀到的授權",
			refused.Recorded.Expression, refused.Recorded.Source))
	case registry.RefusedGeneratedIsPermanent:
		return restrictionInputError(
			"這個小工具是平台生成的，「平台生成」是它來源的唯一紀錄，覆寫會讓它回到搜尋結果與摘要佇列；" +
				"要讓它不再被讀取、試跑或打包，改用「設定受限展示」")
	}
	return refused
}

func redistributionMetadata(before, after, note string, verified registry.LicenseClaim) map[string]any {
	m := map[string]any{auditBefore: before, auditAfter: after, auditNote: note}
	if after == string(registry.RedistributionAllowed) {
		m["license_expression"] = verified.Expression
		m["license_source"] = verified.Source
	}
	return m
}
