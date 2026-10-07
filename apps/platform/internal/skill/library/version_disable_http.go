package registry

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

type operatorVersionResponse struct {
	VersionID     string `json:"version_id"`
	VersionNumber int32  `json:"version_number"`
	Disabled      bool   `json:"disabled"`
}

func operatorVersionID(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	var versionID pgtype.UUID
	if err := versionID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "找不到這個 Skill 版本")
		return pgtype.UUID{}, false
	}
	return versionID, true
}

func operatorVersionView(status OperatorVersionStatus) operatorVersionResponse {
	return operatorVersionResponse{
		VersionID: pgconv.UUIDString(status.ID), VersionNumber: status.VersionNumber, Disabled: status.Disabled,
	}
}

func (h *Handler) OperatorVersionStatus(w http.ResponseWriter, r *http.Request) {
	versionID, ok := operatorVersionID(w, r)
	if !ok {
		return
	}
	status, err := h.Svc.OperatorVersionStatus(r.Context(), versionID)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "找不到這個 Skill 版本")
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "無法查詢 Skill 版本狀態")
	default:
		httpx.WriteJSON(w, http.StatusOK, operatorVersionView(status))
	}
}

func (h *Handler) DisableVersion(w http.ResponseWriter, r *http.Request) {
	versionID, ok := operatorVersionID(w, r)
	if !ok {
		return
	}
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "請提供停用理由")
		return
	}
	status, err := h.Svc.DisableVersion(r.Context(), versionID, user.ID, body.Reason)
	switch {
	case errors.Is(err, ErrDisableReasonRequired), errors.Is(err, ErrDisableReasonTooLong):
		httpx.WriteError(w, http.StatusBadRequest, "請填寫 1 到 1000 bytes 的停用理由")
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "找不到這個 Skill 版本")
	case errors.Is(err, ErrVersionAlreadyDisabled):
		httpx.WriteError(w, http.StatusConflict, "這個 Skill 版本已停用；重複操作已記錄")
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "停用 Skill 版本未完成，請查詢目前狀態")
	default:
		httpx.WriteJSON(w, http.StatusOK, operatorVersionView(status))
	}
}
