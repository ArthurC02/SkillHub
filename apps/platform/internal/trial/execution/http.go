package run

import (
	"encoding/json"
	"errors"
	"fmt"
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

// Handler exposes the run endpoints (contracts/openapi/public.yaml). Every route
// requires a session and takes its workspace from it (iron rule 3).
type Handler struct {
	Svc      *Service
	Identity *identity.Service
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
	// SkillID and TestCaseID are not columns of `runs`; they are filled by
	// fillLinkage. They are served because their consumers cannot derive them and
	// should not have to be told them out of band: applying suggestions posts to
	// /skills/{id}/versions/from-suggestions, and a re-run needs the editable test
	// case rather than the snapshot it was frozen into. Neither is permission to do
	// either thing (TEST-009 still stands).
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

// labelled is the contract's Labelled: an enum value with the words for it.
//
// cleanup_status is served this way (04 丙-29 ②) because of how it failed. The
// contract said `cleaning` while the database type said `cleaning_up`, the
// handler put the database value on the wire unmapped, and the client's
// enum→中文 table had no entry — so the one state this field exists to report,
// **the sandbox is being torn down right now**, rendered as a blank row (04
// 丙-28). A client-side table can only fail that way. A served label cannot.
type labelled struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Note  string `json:"note"`
}

// cleanupWords is the run_cleanup_status enum (0004_test_lab_and_runs.sql) in
// words. RUN-007's teardown is tracked apart from the run outcome, so every note
// here says what it is *not* saying about the run itself.
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

// cleanupWord wraps the raw value in its words, keeping the raw value as the
// label when it is unrecognised — showing a word the reader has to look up beats
// the blank row that made this change necessary.
func cleanupWord(v string) labelled {
	if w, ok := cleanupWords[v]; ok {
		return labelled{Value: v, Label: w[0], Note: w[1]}
	}
	return labelled{
		Value: v, Label: v,
		Note: "這個平台版本沒有這個清理狀態的說明,值照原樣顯示,不猜測它的意思。",
	}
}

// failureClassWords is `runs.failure_class` in words — the platform's own
// six-value vocabulary, fixed by a CHECK constraint in
// db/migrations/0018_run_scheduling.sql. NOT the provider's ten `class` values,
// which are per attempt on run_attempts.error_class.
//
// Served with its words for the reason `cleanupWords` above exists (04 丙-29 ②),
// and because this field spent its whole life being interpolated raw into
// Chinese sentences on four screens — 「失敗類別 capability_mismatch」 and
// 「（分類：capability_mismatch）」 (04 丙-115 ②). `RUN_STATUS_LABEL` in the web app
// covers `status`; nothing ever covered this one.
//
// Each note says what the class does NOT mean, because that is where the four
// screens showing it get read wrong: a workload_error is the skill failing at
// its own job and reads as a platform fault, and a capability_mismatch is a
// refusal before anything ran and reads as a crash.
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

// failureClassWord wraps the raw value in its words, keeping the raw value as
// the label when it is unrecognised — the same call cleanupWord makes, for the
// same reason: a word the reader has to look up beats a blank.
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

// attemptView is the RUN-003 mapping as the API exposes it: the platform ids are
// the identity, the provider's id is an attribute of one attempt.
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

// fillLinkage adds the two ids the runs row does not carry. One lookup for all
// three run responses rather than each handler assembling its own from whatever it
// happens to have in scope — a `skill_id` that is present on one response and
// absent on another is a field no client can rely on.
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

// Create handles POST /skills/{id}/runs (RUN-001).
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
		// The summary the user confirmed (02:TEST-005). Absent or stale is a 422
		// below; it is never inferred from a previous run.
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
	// SEC-012: the fleet is held for a P1. 503 and not 422, which is the whole rest
	// of this list: every refusal below says the request may not proceed while the
	// platform is fine, and this one says the platform is not. It carries no
	// incident detail — the person who cannot start a run is not the person the
	// incident is about.
	if errors.Is(err, ErrDispatchHalted) {
		httpx.WriteError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	// SEC-002 gate B. 422, like the capability refusal: the request is well formed
	// and the platform is working, it simply may not proceed until the user has
	// agreed to the current permissions.
	if errors.Is(err, ErrPermissionsNotConfirmed) {
		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	// RUN-005: no configured provider can run this, and the message says which
	// requirement each one failed. 422, not 500: the request is well-formed and the
	// platform is working, it simply cannot be satisfied.
	if errors.Is(err, ErrNoCompatibleProvider) {
		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	// SEC-002 gate B's other two blocking conditions (internal/run/gateb.go): a
	// blocking or unperformable static scan, and the workspace concurrency ceiling.
	// Same 422 as the two above, for the same reason - nothing about the request is
	// malformed, it simply may not proceed. The messages say which, and what to do.
	// Same 422 for the 0023 licensing hold: the request is fine, the platform is
	// fine, and this content may not be copied into a sandbox until the review
	// concludes.
	//
	// PDM-010's allowance joins them (ADR-028 決策 2): out of runs for this window
	// is not a malformed request either, and 422 keeps the whole of gate B on one
	// status code. The message carries the reset time, because "come back later"
	// without a time is the version of this screen nobody can act on.
	if errors.Is(err, ErrScanBlocked) || errors.Is(err, ErrRunLimitReached) ||
		errors.Is(err, ErrAccessRestricted) || errors.Is(err, policy.ErrQuotaExceeded) {
		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "run creation failed")
		return
	}
	resp := toRunResponse(run)
	if !h.fillLinkage(w, r, ws.ID, run.ID, &resp) {
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, resp)
}

// Quota handles GET /me/quota (PDM-010). It reports the counters
// POST /skills/{id}/runs enforces, and computes nothing of its own — a display
// with its own arithmetic is free to disagree with the rule, and the direction it
// would disagree in is the generous one (04 乙-2).
//
// Mounted only where an allowance is actually enforced (see apiserver.NewRouter),
// so a deployment with no allowance answers 404 here rather than serving numbers
// nothing applies.
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

// runListItem is one row of the Run history (WS-004). Deliberately narrower than
// runResponse: a history page needs what happened, to which skill, and when, and
// serving the transitions and attempts of fifty runs would make the list the
// heaviest read in the API for information nobody reads fifty of.
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
	// The second axis (ADR-025, 設計系統 §2.5, 04 丙-32). Never omitempty: a run
	// with no evaluation carries 未評估, and an absent field is the one rendering
	// §2.9 forbids — an empty column beside 「執行完成」 reads as a pass.
	Evaluation json.RawMessage `json:"evaluation"`
}

// List handles GET /runs (WS-004): the workspace's Run history, optionally the
// history of one test case (`?test_case_id=`).
//
// It did not exist until now, which is why 02:WS-002 第 1 條's "Run 歷史" had no
// surface at any layer — not a missing screen but a missing endpoint.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ws, _, ok := h.workspace(w, r)
	if !ok {
		return
	}
	// Paging is parsed before the filter, and ahead of the empty-list early
	// return below: a `limit` or `offset` outside its schema is refused whatever
	// the filter says, rather than being answered with a page the caller then
	// reads as their whole history.
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
			// An empty list, not the unfiltered history: a caller who asked for one
			// test case's runs must not be handed every run in the workspace because
			// the server could not read their filter.
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
	// Not wired is a 500, not a list with one axis. §2.5 wants the verdict on the
	// row and ahead of the execution status; a page that silently lost it is a
	// column of 「執行完成」 with nothing to qualify it, which is the exact reading
	// ADR-025 exists to prevent. app_test.go asserts the wiring.
	if h.Svc.RunVerdicts == nil {
		httpx.WriteError(w, http.StatusInternalServerError, "run list failed")
		return
	}
	runIDs := make([]pgtype.UUID, 0, len(rows))
	for _, row := range rows {
		runIDs = append(runIDs, row.ID)
	}
	verdicts, err := h.Svc.RunVerdicts(r.Context(), ws.ID, runIDs)
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

// artifactView is one Run output as its owner sees it: the manifest row, never
// the bytes. The control plane does not open what a sandbox produced (iron rule
// 1), and this endpoint exists so the owner can see what there is to delete.
type artifactView struct {
	ArtifactID  string `json:"artifact_id"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentHash string `json:"content_hash"`
	CreatedAt   string `json:"created_at"`
	ExpiresAt   string `json:"expires_at,omitempty"`
	// Purged says the bytes are gone while the row remains — retention expiry or a
	// reconciler finding them missing. "It expired" and "it never existed" are
	// different answers and the list gives the right one (0028).
	Purged bool `json:"purged"`
}

// Artifacts handles GET /runs/{id}/artifacts (WS-004, 02:SEC-006).
func (h *Handler) Artifacts(w http.ResponseWriter, r *http.Request) {
	ws, _, ok := h.workspace(w, r)
	if !ok {
		return
	}
	runID, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	rows, err := h.Svc.Artifacts(r.Context(), ws.ID, runID)
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
	}{out})
}

// DeleteArtifact handles DELETE /runs/{id}/artifacts/{artifactId}
// (02:WS-002 第 3 條, 02:SEC-006 第 1 條).
//
// Idempotent and therefore 204 for an id that was never there, matching the
// download package's delete: the caller asked for the file not to exist, and
// answering 404 to a repeat of a delete that worked reports success as failure.
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

// pathUUID parses one path id. A malformed one answers 404 for the reason every
// unknown id does: the caller learns nothing either way.
func pathUUID(w http.ResponseWriter, r *http.Request, name string) (id pgtype.UUID, ok bool) {
	if err := id.Scan(r.PathValue(name)); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return id, false
	}
	return id, true
}

// parseLimit reads the `limit` GET /runs declares in public.yaml:
// `{ type: integer, minimum: 1, maximum: 200, default: 50 }`, which is the same
// pair Service.List clamps to. Absent is the default; present is the schema or
// it is a 400. Both bounds are inclusive, as JSON Schema's minimum/maximum are,
// and `limit=` (present, empty) is refused too since allowEmptyValue is not set.
//
// This is the fourth handler in the family that swallowed an out-of-schema
// paging parameter (discovery's parseLimit and testlab's parseListLimit,
// 2026-08-25). `limit=abc`, `limit=-1` and `limit=0` all became 0 here, which
// Service.List then turned into 50. A caller who asked for 500 got 50 rows and
// read that as the size of their run history — a ceiling they never asked for
// presenting itself as a result count (ADR-042 決策 3).
//
// The bounds come from the service constants rather than repeating 1 and 200:
// the clamp and the refusal have to agree, and one of them moving alone is how
// a handler starts disagreeing with its own contract again.
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

// parseOffset reads the `offset` GET /runs declares:
// `{ type: integer, minimum: 0, default: 0 }`. Same rule as parseLimit — absent
// is the default, present is the schema or it is a 400.
//
// A negative offset is refused rather than floored to 0, and it is not a milder
// mistake than `offset=abc`: both are values the schema does not describe, and
// both arrive the same way, from client-side page arithmetic that went wrong.
// Serving page 1 to a caller who asked for offset -50 hands them rows they did
// not ask for while looking exactly like a correct answer, which is the failure
// this whole family is about. `abc` at least cannot be mistaken for a page.
//
// The schema names no maximum, but int32 is a real ceiling here: the offset
// reaches Postgres as the statement's int4 OFFSET. ParseInt with a 32-bit size
// answers both the non-numeric and the too-large case in one call, so the
// ceiling is enforced rather than silently wrapped.
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

// Get handles GET /runs/{id} (RUN-002: current status and how it got there).
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

// Cancel handles POST /runs/{id}/cancel (RUN-004). It records intent and says so;
// the run keeps its current status until the workload is actually down (RUN-006).
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
	}{body, "cancellation requested; the run keeps its current status " +
		"until the workload has actually stopped"}
	httpx.WriteJSON(w, http.StatusAccepted, resp)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
