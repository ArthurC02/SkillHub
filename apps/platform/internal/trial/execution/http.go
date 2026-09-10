package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
)

type Handler struct {
	Svc      *Service
	Identity *identity.Service

	RunVerdicts func(context.Context, pgtype.UUID, []pgtype.UUID) (map[string]json.RawMessage, error)
}

func (h *Handler) workspace(w http.ResponseWriter, r *http.Request) (identity.Workspace, identity.User, bool) {
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return identity.Workspace{}, identity.User{}, false
	}
	ws, err := h.Identity.PersonalWorkspace(r.Context(), user)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "workspace lookup failed")
		return identity.Workspace{}, identity.User{}, false
	}
	return ws, user, true
}

type runResponse struct {
	RunID        string `json:"run_id"`
	Status       string `json:"status"`
	StatusReason string `json:"status_reason,omitempty"`

	SkillID           string           `json:"skill_id"`
	SkillVersionID    string           `json:"skill_version_id"`
	TestCaseSnapshot  string           `json:"test_case_snapshot_id"`
	TestCaseID        string           `json:"test_case_id,omitempty"`
	Provider          string           `json:"provider"`
	FailureClass      *labelled        `json:"failure_class,omitempty"`
	CleanupStatus     labelled         `json:"cleanup_status"`
	CancelRequestedAt string           `json:"cancel_requested_at,omitempty"`
	CreatedAt         string           `json:"created_at"`
	StartedAt         string           `json:"started_at,omitempty"`
	FinishedAt        string           `json:"finished_at,omitempty"`
	Transitions       []transitionView `json:"transitions,omitempty"`
	Attempts          []attemptView    `json:"attempts,omitempty"`
}

type labelled struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Note  string `json:"note"`
}

var cleanupWords = map[string][2]string{
	"pending": {"待清理",
		"沙箱還沒有被拆除。與這次 Run 的成敗無關——那是上面那一列。"},
	"cleaning_up": {"清理中",
		"沙箱正在拆除。這是暫時狀態,會自己結束。"},
	"cleaned": {"已清理",
		"沙箱與其資源已回收。"},
	"failed": {"清理失敗",
		"沙箱沒有被成功拆除,平台會重試;殘留由對帳器接手(RUN-007 冪等清理)。" +
			"這不代表這次 Run 失敗。"},
}

func cleanupWord(v string) labelled {
	if w, ok := cleanupWords[v]; ok {
		return labelled{Value: v, Label: w[0], Note: w[1]}
	}
	return labelled{
		Value: v, Label: v,
		Note: "這個平台版本沒有這個清理狀態的說明,值照原樣顯示,不猜測它的意思。",
	}
}

var failureClassWords = map[string][2]string{
	"provider_error": {"Provider 錯誤",
		"執行沙箱的那一側沒能承載這次嘗試。這不是 Skill 的問題,也是唯一一類平台會自己重試的失敗。"},
	"workload_error": {"工作負載失敗",
		"工作負載跑起來了,而且自己回報失敗。這是 Skill 在它自己的工作上失敗,不是平台故障;重試只會再花一次錢得到同一個答案。"},
	"timeout": {"逾時",
		"Provider 回報的軟性上限,或平台看門狗的硬性上限。工作到哪裡為止見執行紀錄。"},
	"cancelled": {"已取消",
		"是使用者要求停止的,不是失敗。"},
	"capability_mismatch": {"沒有能跑這個請求的環境",
		"在任何東西被執行之前就被拒絕了——沒有一個已設定的 Provider 能承接這個請求。這不是崩潰,沙箱從來沒有被建立。"},
	"platform_error": {"平台自己的錯誤",
		"控制平面這一側的問題,不是 Skill 也不是 Provider 的問題。"},
}

func failureClassWord(v string) *labelled {
	if v == "" {
		return nil
	}
	if w, ok := failureClassWords[v]; ok {
		return &labelled{Value: v, Label: w[0], Note: w[1]}
	}
	return &labelled{
		Value: v, Label: v,
		Note: "這個平台版本沒有這個失敗類別的說明,值照原樣顯示,不猜測它的意思。",
	}
}

type transitionView struct {
	From       string `json:"from_status,omitempty"`
	To         string `json:"to_status"`
	Reason     string `json:"reason,omitempty"`
	OccurredAt string `json:"occurred_at"`
}

type attemptView struct {
	RunAttemptID  string `json:"run_attempt_id"`
	AttemptNumber int32  `json:"attempt_number"`
	Provider      string `json:"provider"`
	ProviderRunID string `json:"provider_run_id,omitempty"`
	ErrorClass    string `json:"error_class,omitempty"`
	ErrorMessage  string `json:"error_message,omitempty"`
	StartedAt     string `json:"started_at,omitempty"`
	FinishedAt    string `json:"finished_at,omitempty"`
}

func toRunResponse(run gen.Run) runResponse {
	return runResponse{
		RunID:             pgconv.UUIDString(run.ID),
		Status:            string(run.Status),
		StatusReason:      deref(run.StatusReason),
		SkillVersionID:    pgconv.UUIDString(run.SkillVersionID),
		TestCaseSnapshot:  pgconv.UUIDString(run.TestCaseSnapshotID),
		Provider:          run.Provider,
		FailureClass:      failureClassWord(deref(run.FailureClass)),
		CleanupStatus:     cleanupWord(string(run.CleanupStatus)),
		CancelRequestedAt: pgconv.RFC3339(run.CancelRequestedAt),
		CreatedAt:         pgconv.RFC3339(run.CreatedAt),
		StartedAt:         pgconv.RFC3339(run.StartedAt),
		FinishedAt:        pgconv.RFC3339(run.FinishedAt),
	}
}

func (h *Handler) fillLinkage(
	w http.ResponseWriter, r *http.Request, ws, runID pgtype.UUID, resp *runResponse,
) bool {
	link, err := h.Svc.Linkage(r.Context(), ws, runID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "run lookup failed")
		return false
	}
	resp.SkillID = pgconv.UUIDString(link.SkillID)
	resp.TestCaseID = pgconv.UUIDString(link.TestCaseID)
	return true
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	ws, user, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var skillID pgtype.UUID
	if err := skillID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	var body struct {
		VersionID  string `json:"version_id"`
		TestCaseID string `json:"test_case_id"`

		ConfirmedSummaryHash string `json:"confirmed_summary_hash"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with version_id and test_case_id")
		return
	}
	var versionID, testCaseID pgtype.UUID
	if versionID.Scan(body.VersionID) != nil || testCaseID.Scan(body.TestCaseID) != nil {
		httpx.WriteError(w, http.StatusBadRequest, "version_id and test_case_id must be UUIDs")
		return
	}

	run, err := h.Svc.Create(r.Context(), CreateParams{
		WorkspaceID: ws.ID, Actor: user.ID,
		SkillID: skillID, VersionID: versionID, TestCaseID: testCaseID,
		ConfirmedSummaryHash: body.ConfirmedSummaryHash,
	})

	if errors.Is(err, ErrDispatchHalted) {
		httpx.WriteError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrPreflightTargetNotFound) {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}

	if errors.Is(err, ErrPermissionsNotConfirmed) {
		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	if errors.Is(err, ErrNoCompatibleProvider) {
		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	if errors.Is(err, ErrCreditBalance) || errors.Is(err, ErrScanBlocked) || errors.Is(err, ErrRunLimitReached) ||
		errors.Is(err, ErrAccessRestricted) || errors.Is(err, policy.ErrQuotaExceeded) {
		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err != nil {

		slog.Error("run creation failed", "error", err)
		httpx.WriteError(w, http.StatusInternalServerError, "run creation failed")
		return
	}
	resp := toRunResponse(run)
	if !h.fillLinkage(w, r, ws.ID, run.ID, &resp) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (h *Handler) Quota(w http.ResponseWriter, r *http.Request) {
	ws, _, ok := h.workspace(w, r)
	if !ok {
		return
	}
	state, enforced, err := h.Svc.QuotaFor(r.Context(), ws.ID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "quota lookup failed")
		return
	}
	if !enforced {
		httpx.WriteError(w, http.StatusNotFound, "no run allowance is configured")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, state.View())
}

type runListItem struct {
	RunID          string    `json:"run_id"`
	Status         string    `json:"status"`
	StatusReason   string    `json:"status_reason,omitempty"`
	SkillID        string    `json:"skill_id"`
	SkillName      string    `json:"skill_name"`
	SkillVersionID string    `json:"skill_version_id"`
	TestCaseID     string    `json:"test_case_id,omitempty"`
	Provider       string    `json:"provider"`
	FailureClass   *labelled `json:"failure_class,omitempty"`
	CleanupStatus  labelled  `json:"cleanup_status"`
	CreatedAt      string    `json:"created_at"`
	StartedAt      string    `json:"started_at,omitempty"`
	FinishedAt     string    `json:"finished_at,omitempty"`

	Evaluation json.RawMessage `json:"evaluation"`
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ws, _, ok := h.workspace(w, r)
	if !ok {
		return
	}

	limit, err := parseLimit(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	offset, err := parseOffset(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var testCaseID pgtype.UUID
	if raw := r.URL.Query().Get("test_case_id"); raw != "" {
		if err := testCaseID.Scan(raw); err != nil {

			httpx.WriteJSON(w, http.StatusOK, struct {
				Runs []runListItem `json:"runs"`
			}{[]runListItem{}})
			return
		}
	}
	rows, err := h.Svc.List(r.Context(), ws.ID, testCaseID, limit, offset)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "run list failed")
		return
	}

	if h.RunVerdicts == nil {
		httpx.WriteError(w, http.StatusInternalServerError, "run list failed")
		return
	}
	runIDs := make([]pgtype.UUID, 0, len(rows))
	for _, row := range rows {
		runIDs = append(runIDs, row.ID)
	}
	verdicts, err := h.RunVerdicts(r.Context(), ws.ID, runIDs)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "run list failed")
		return
	}

	out := make([]runListItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, runListItem{
			Evaluation: verdicts[pgconv.UUIDString(row.ID)],
			RunID:      pgconv.UUIDString(row.ID), Status: string(row.Status),
			StatusReason:   deref(row.StatusReason),
			SkillID:        pgconv.UUIDString(row.SkillID),
			SkillName:      row.SkillName,
			SkillVersionID: pgconv.UUIDString(row.SkillVersionID),
			TestCaseID:     pgconv.UUIDString(row.TestCaseID),
			Provider:       row.Provider,
			FailureClass:   failureClassWord(deref(row.FailureClass)),
			CleanupStatus:  cleanupWord(string(row.CleanupStatus)),
			CreatedAt:      pgconv.RFC3339(row.CreatedAt),
			StartedAt:      pgconv.RFC3339(row.StartedAt),
			FinishedAt:     pgconv.RFC3339(row.FinishedAt),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, struct {
		Runs []runListItem `json:"runs"`
	}{out})
}

type artifactView struct {
	ArtifactID  string `json:"artifact_id"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentHash string `json:"content_hash"`
	CreatedAt   string `json:"created_at"`
	ExpiresAt   string `json:"expires_at,omitempty"`

	Purged bool `json:"purged"`
}

func (h *Handler) Artifacts(w http.ResponseWriter, r *http.Request) {
	ws, _, ok := h.workspace(w, r)
	if !ok {
		return
	}
	runID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	rows, truncated, err := h.Svc.Artifacts(r.Context(), ws.ID, runID)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "artifact list failed")
		return
	}
	out := make([]artifactView, 0, len(rows))
	for _, a := range rows {
		out = append(out, artifactView{
			ArtifactID: pgconv.UUIDString(a.ID), FileName: a.FileName, ContentType: a.ContentType,
			SizeBytes: a.SizeBytes, ContentHash: a.ContentHash,
			CreatedAt: pgconv.RFC3339(a.CreatedAt), ExpiresAt: pgconv.RFC3339(a.ExpiresAt),
			Purged: a.PurgedAt.Valid,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, struct {
		Artifacts []artifactView `json:"artifacts"`
		Truncated bool           `json:"truncated"`
	}{out, truncated})
}

func (h *Handler) DeleteArtifact(w http.ResponseWriter, r *http.Request) {
	ws, _, ok := h.workspace(w, r)
	if !ok {
		return
	}
	runID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	artifactID, ok := pathUUID(w, r, "artifactId")
	if !ok {
		return
	}
	if err := h.Svc.DeleteArtifact(r.Context(), ws, runID, artifactID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (id pgtype.UUID, ok bool) {
	if err := id.Scan(r.PathValue(name)); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return id, false
	}
	return id, true
}

func parseLimit(r *http.Request) (int32, error) {
	q := r.URL.Query()
	if !q.Has("limit") {
		return defaultRunPageSize, nil
	}
	n, err := strconv.Atoi(q.Get("limit"))
	if err != nil || n < 1 || n > maxRunPageSize {
		return 0, fmt.Errorf("query parameter limit must be a whole number between 1 and %d", maxRunPageSize)
	}
	return int32(n), nil
}

func parseOffset(r *http.Request) (int32, error) {
	q := r.URL.Query()
	if !q.Has("offset") {
		return 0, nil
	}
	n, err := strconv.ParseInt(q.Get("offset"), 10, 32)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("query parameter offset must be a whole number between 0 and %d", math.MaxInt32)
	}
	return int32(n), nil
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	ws, _, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var runID pgtype.UUID
	if err := runID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}

	run, err := h.Svc.Get(r.Context(), ws.ID, runID)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "run lookup failed")
		return
	}

	resp := toRunResponse(run)
	if !h.fillLinkage(w, r, ws.ID, runID, &resp) {
		return
	}
	transitions, err := h.Svc.History(r.Context(), ws.ID, runID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "run history lookup failed")
		return
	}
	for _, t := range transitions {
		v := transitionView{To: string(t.ToStatus), Reason: deref(t.Reason), OccurredAt: pgconv.RFC3339(t.OccurredAt)}
		if t.FromStatus != nil {
			v.From = string(*t.FromStatus)
		}
		resp.Transitions = append(resp.Transitions, v)
	}
	attempts, err := h.Svc.Attempts(r.Context(), ws.ID, runID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "run attempt lookup failed")
		return
	}
	for _, a := range attempts {
		resp.Attempts = append(resp.Attempts, attemptView{
			RunAttemptID:  pgconv.UUIDString(a.ID),
			AttemptNumber: a.AttemptNumber,
			Provider:      a.Provider,
			ProviderRunID: deref(a.ProviderRunID),
			ErrorClass:    deref(a.ErrorClass),
			ErrorMessage:  deref(a.ErrorMessage),
			StartedAt:     pgconv.RFC3339(a.StartedAt),
			FinishedAt:    pgconv.RFC3339(a.FinishedAt),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	ws, user, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var runID pgtype.UUID
	if err := runID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}

	run, err := h.Svc.RequestCancel(r.Context(), ws.ID, runID, user.ID)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, ErrRunFinished):
		httpx.WriteError(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "cancel failed")
		return
	}

	body := toRunResponse(run)
	if !h.fillLinkage(w, r, ws.ID, runID, &body) {
		return
	}
	resp := struct {
		runResponse
		Note string `json:"note"`
	}{body, "已送出取消要求；在工作負載真的停下來之前，這個 Run 會維持目前的狀態。"}
	httpx.WriteJSON(w, http.StatusAccepted, resp)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
