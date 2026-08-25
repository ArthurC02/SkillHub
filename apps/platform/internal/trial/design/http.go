package testlab

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

// Handler exposes the test lab endpoints (contracts/openapi/public.yaml). Every
// route requires a session; the workspace comes from it (iron rule 3).
type Handler struct {
	Svc      *Service
	Identity *identity.Service
}

// maxJSONBytes caps the JSON request bodies. Generous next to MaxPromptBytes so
// an over-long prompt gets the specific error rather than a truncated read.
const maxJSONBytes = 64 << 10

func (h *Handler) workspace(w http.ResponseWriter, r *http.Request) (identity.Workspace, bool) {
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return identity.Workspace{}, false
	}
	ws, err := h.Identity.PersonalWorkspace(r.Context(), user)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "workspace lookup failed")
		return identity.Workspace{}, false
	}
	return ws, true
}

// pathUUID reads a path parameter as a UUID. A malformed id is answered exactly
// like an id that belongs to someone else (WS-006).
func pathUUID(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(r.PathValue(name)); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return pgtype.UUID{}, false
	}
	return id, true
}

// fail maps a domain error to a status. ErrLimitExceeded and ErrUnsupportedType
// carry their own message through to the client because 02:TEST-002 requires the
// user to understand why an upload was refused; every other internal failure
// answers with a fixed string instead (不洩漏系統資訊).
func fail(w http.ResponseWriter, err error, generic string) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrInvalid):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrUnsupportedType):
		httpx.WriteError(w, http.StatusUnsupportedMediaType, err.Error())
	case errors.Is(err, ErrLimitExceeded):
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, err.Error())
	// TEST-002 is an optional enhancement, so its absence is a 503 that names the
	// manual path rather than a failure of the request. The wrapped cause stays in
	// the log; the client is told only that it is unavailable.
	case errors.Is(err, ErrSuggestUnavailable):
		httpx.WriteError(w, http.StatusServiceUnavailable, ErrSuggestUnavailable.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, generic)
	}
}

type testCaseResponse struct {
	TestCaseID         string      `json:"test_case_id"`
	SkillID            string      `json:"skill_id"`
	Name               string      `json:"name"`
	UserPrompt         string      `json:"user_prompt"`
	AcceptanceCriteria []Criterion `json:"acceptance_criteria"`
	// Absent means this test case has no rubric — not that it uses a default one
	// (CONTENT-007). Same distinction the evaluation report already makes.
	Rubric    *Rubric `json:"rubric,omitempty"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

func toTestCaseResponse(tc gen.TestCase) testCaseResponse {
	criteria, err := DecodeCriteria(tc.AcceptanceCriteria)
	if err != nil {
		criteria = []Criterion{}
	}
	rubric, err := DecodeRubric(tc.Rubric)
	if err != nil {
		rubric = nil
	}
	return testCaseResponse{
		TestCaseID:         pgconv.UUIDString(tc.ID),
		SkillID:            pgconv.UUIDString(tc.SkillID),
		Name:               tc.Name,
		UserPrompt:         tc.UserPrompt,
		AcceptanceCriteria: criteria,
		Rubric:             rubric,
		CreatedAt:          tc.CreatedAt.Time.UTC().Format(time.RFC3339),
		UpdatedAt:          tc.UpdatedAt.Time.UTC().Format(time.RFC3339),
	}
}

type datasetResponse struct {
	DatasetID   string `json:"dataset_id"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentHash string `json:"content_hash"`
	ExpiresAt   string `json:"expires_at"`
}

func toDatasetResponse(d gen.Dataset) datasetResponse {
	return datasetResponse{
		DatasetID:   pgconv.UUIDString(d.ID),
		FileName:    d.FileName,
		ContentType: d.ContentType,
		SizeBytes:   d.SizeBytes,
		ContentHash: d.ContentHash,
		ExpiresAt:   d.ExpiresAt.Time.UTC().Format(time.RFC3339),
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBytes)).Decode(v); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be valid JSON")
		return false
	}
	return true
}

// Create handles POST /test-cases (TEST-001).
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var body struct {
		SkillID    string `json:"skill_id"`
		Name       string `json:"name"`
		UserPrompt string `json:"user_prompt"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	var skillID pgtype.UUID
	if err := skillID.Scan(body.SkillID); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	tc, err := h.Svc.CreateTestCase(r.Context(), ws, skillID, body.Name, body.UserPrompt)
	if err != nil {
		fail(w, err, "create failed")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toTestCaseResponse(tc))
}

// testCaseListItem is one row of GET /test-cases. The counts and has_rubric are
// derived from the draft the row already carries; they are served so a list of
// fifty rows is not fifty client-side reductions, and so nothing has to decide
// twice what "confirmed" means.
type testCaseListItem struct {
	testCaseResponse
	SkillName         string `json:"skill_name"`
	CriteriaConfirmed int    `json:"criteria_confirmed"`
	CriteriaTotal     int    `json:"criteria_total"`
	HasRubric         bool   `json:"has_rubric"`
}

// parseListLimit reads the `limit` GET /test-cases declares in public.yaml:
// `{ type: integer, minimum: 1, maximum: 101, default: 51 }`. Absent is the
// default; present is the schema or it is a 400.
//
// The 51/101 look like off-by-ones and are not: the web client asks for 51 to
// get 50 rows plus the sentinel that tells it there is another page
// (apps/web/src/api/testcases.ts), so the ceiling is the same trick at 100.
// Code and contract agree on both bounds.
//
// What did not agree was the refusal. This was the third handler swallowing an
// out-of-schema limit after both search endpoints were fixed (discovery's
// parseLimit, 2026-08-25): `limit=0`, `limit=500` and `limit=abc` all quietly
// became 51. A caller who asked for 500 and got 51 rows reads that as the size
// of their library.
func parseListLimit(r *http.Request) (int32, error) {
	q := r.URL.Query()
	if !q.Has("limit") {
		return 51, nil
	}
	// Both bounds inclusive, as JSON Schema's minimum/maximum are; `limit=`
	// (present, empty) is refused too, since allowEmptyValue is not set.
	n, err := strconv.Atoi(q.Get("limit"))
	if err != nil || n < 1 || n > 101 {
		return 0, errors.New("query parameter limit must be a whole number between 1 and 101")
	}
	return int32(n), nil
}

// parseListOffset reads the `offset` GET /test-cases declares beside it:
// `{ type: integer, minimum: 0, default: 0 }`. Same rule as parseListLimit —
// absent is the default, present is the schema or it is a 400.
//
// It was the last half of the family left swallowing: `offset=abc`,
// `offset=-1` and `offset=` all became 0 while the `limit` next to them had
// already learned to refuse, so one handler gave two different answers to the
// same question. A negative offset is not a milder mistake than a non-numeric
// one — both are values the schema does not describe, and both arrive from
// client-side page arithmetic that went wrong. Quietly serving page 1 to a
// caller who asked for offset -50 hands them rows they did not ask for while
// looking like a correct answer.
//
// The schema names no maximum; int32 is one anyway, because the value reaches
// Postgres as the statement's int4 OFFSET. ParseInt with a 32-bit size answers
// the non-numeric and the too-large case in one call.
func parseListOffset(r *http.Request) (int32, error) {
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

// List handles GET /test-cases (TEST-001, WS-004). `skill_id` narrows it to one
// skill; a malformed one answers an empty list rather than the unfiltered one —
// a filter the server could not read must not silently become "no filter", so it
// is parsed strictly and only a valid UUID filters.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	limit, err := parseListLimit(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	offset, err := parseListOffset(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	var skillID pgtype.UUID
	if raw := r.URL.Query().Get("skill_id"); raw != "" {
		if err := skillID.Scan(raw); err != nil {
			// An unparseable filter answers an empty list, not the unfiltered one:
			// showing every test case to a caller who asked for one skill's is the
			// wrong direction to fail in.
			httpx.WriteJSON(w, http.StatusOK, map[string]any{"test_cases": []testCaseListItem{}})
			return
		}
	}
	rows, err := h.Svc.ListTestCases(r.Context(), ws, skillID, limit, offset)
	if err != nil {
		fail(w, err, "list failed")
		return
	}
	out := make([]testCaseListItem, 0, len(rows))
	for _, row := range rows {
		item := testCaseListItem{
			testCaseResponse: toTestCaseResponse(row.TestCase),
			SkillName:        row.SkillName,
		}
		item.CriteriaTotal = len(item.AcceptanceCriteria)
		for _, c := range item.AcceptanceCriteria {
			if c.ConfirmedAt != nil {
				item.CriteriaConfirmed++
			}
		}
		item.HasRubric = item.Rubric != nil
		out = append(out, item)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"test_cases": out})
}

// Get handles GET /test-cases/{id}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	tc, err := h.Svc.GetTestCase(r.Context(), ws, id)
	if err != nil {
		fail(w, err, "read failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toTestCaseResponse(tc))
}

// Update handles PATCH /test-cases/{id} (TEST-001).
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	current, err := h.Svc.GetTestCase(r.Context(), ws, id)
	if err != nil {
		fail(w, err, "read failed")
		return
	}
	// Absent fields keep their stored value, so a client editing only the prompt
	// cannot blank the name by omission. `rubric` is three-valued for the same
	// reason: absent keeps it, an object replaces it, and an explicit null removes
	// it. A json.RawMessage and not a *Rubric because those are three states and a
	// decoded struct only carries two — and not a *json.RawMessage either, which
	// encoding/json collapses back to nil on an explicit null.
	body := struct {
		Name       *string         `json:"name"`
		UserPrompt *string         `json:"user_prompt"`
		Rubric     json.RawMessage `json:"rubric"`
	}{}
	if !decodeJSON(w, r, &body) {
		return
	}
	name, prompt := current.Name, current.UserPrompt
	if body.Name != nil {
		name = *body.Name
	}
	if body.UserPrompt != nil {
		prompt = *body.UserPrompt
	}
	tc, err := h.Svc.UpdateTestCase(r.Context(), ws, id, name, prompt)
	if err != nil {
		fail(w, err, "update failed")
		return
	}
	if len(body.Rubric) > 0 {
		var rubric *Rubric
		if raw := strings.TrimSpace(string(body.Rubric)); raw != "null" {
			rubric = &Rubric{}
			if err := json.Unmarshal(body.Rubric, rubric); err != nil {
				httpx.WriteError(w, http.StatusBadRequest, "rubric must be an object with a version and items")
				return
			}
		}
		if tc, err = h.Svc.SetRubric(r.Context(), ws, id, rubric); err != nil {
			fail(w, err, "rubric update failed")
			return
		}
	}
	httpx.WriteJSON(w, http.StatusOK, toTestCaseResponse(tc))
}

// Delete handles DELETE /test-cases/{id} (WS-002 刪除範圍).
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	res, err := h.Svc.DeleteTestCase(r.Context(), ws, id)
	if err != nil {
		fail(w, err, "delete failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"deleted":          true,
		"datasets_deleted": res.DatasetsDeleted,
		"note": "test case and its uploaded files are removed, files included; " +
			"snapshots of past runs are retained and keep the prompt, the acceptance " +
			"criteria and each file's name and content hash",
	})
}

// AddCriterion handles POST /test-cases/{id}/criteria (TEST-003).
func (h *Handler) AddCriterion(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Text string `json:"text"`
		// Absent means the user wrote it. `suggested` is for text adopted verbatim
		// from POST .../criteria/suggest; the service refuses to read anything else
		// as a label, so an invented value cannot get into the stored criterion.
		Source string `json:"source"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	tc, err := h.Svc.AddCriterion(r.Context(), ws, id, body.Text, body.Source)
	if err != nil {
		fail(w, err, "add criterion failed")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toTestCaseResponse(tc))
}

// SuggestCriteria handles POST /test-cases/{id}/criteria/suggest (TEST-002).
// It returns proposals and writes nothing: adopting one is AddCriterion, which
// is the user's decision to make (TEST-001 自動建議為可選強化).
func (h *Handler) SuggestCriteria(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	suggestions, err := h.Svc.SuggestCriteria(r.Context(), ws, id)
	if err != nil {
		fail(w, err, "suggestion failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"suggestions": suggestions})
}

// UpdateCriterion handles PATCH /test-cases/{id}/criteria/{criterionId}: edit
// the text, confirm it, or both (TEST-003).
func (h *Handler) UpdateCriterion(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	var body struct {
		Text      *string `json:"text"`
		Confirmed *bool   `json:"confirmed"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Text == nil && body.Confirmed == nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must set text, confirmed, or both")
		return
	}
	tc, err := h.Svc.UpdateCriterion(r.Context(), ws, id, r.PathValue("criterionId"), body.Text, body.Confirmed)
	if err != nil {
		fail(w, err, "update criterion failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toTestCaseResponse(tc))
}

// DeleteCriterion handles DELETE /test-cases/{id}/criteria/{criterionId}.
func (h *Handler) DeleteCriterion(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	tc, err := h.Svc.DeleteCriterion(r.Context(), ws, id, r.PathValue("criterionId"))
	if err != nil {
		fail(w, err, "delete criterion failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toTestCaseResponse(tc))
}

// Limits handles GET /test-cases/limits: the upload rules 02:TEST-002 requires
// to be shown *before* the upload, not discovered by being refused.
// Run permission summaries and re-confirmation are served by the separate
// preflight surface.
func (h *Handler) Limits(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"max_file_bytes":          int64(MaxFileBytes),
		"max_test_case_bytes":     int64(MaxTestCaseBytes),
		"max_files_per_test_case": MaxFilesPerTestCase,
		"retention_days":          int(DatasetRetention / (24 * time.Hour)),
		"allowed_kinds": []string{
			"text (.txt .md .csv .tsv .json .jsonl .xml .yaml .yml)",
			"documents (.pdf .docx .xlsx .pptx)",
			"images (.png .jpg .webp)",
			"archives (.zip, single layer)",
		},
		"note": "file type is decided by content, not by file extension; uploaded " +
			"files are readable only by runs of this test case and are deleted after " +
			"the retention window or whenever you delete them",
	})
}

// UploadDataset handles POST /test-cases/{id}/datasets (TEST-004), multipart
// with one "file" part.
func (h *Handler) UploadDataset(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}

	// Two guards, deliberately: MaxBytesReader stops the transfer at the wire so
	// an oversized upload is never buffered, and the length check below turns
	// "the body ended early" into the specific size error. The slack covers the
	// multipart framing around a file that is exactly at the limit.
	r.Body = http.MaxBytesReader(w, r.Body, MaxFileBytes+(1<<20))
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge,
			"file is larger than "+humanMB(MaxFileBytes)+" or the upload is malformed")
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "multipart body must carry one 'file' part")
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, MaxFileBytes+1))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "could not read the uploaded file")
		return
	}

	ds, err := h.Svc.UploadDataset(r.Context(), ws, id, header.Filename, data)
	if err != nil {
		fail(w, err, "upload failed")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toDatasetResponse(ds))
}

// ListDatasets handles GET /test-cases/{id}/datasets.
func (h *Handler) ListDatasets(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	rows, err := h.Svc.ListDatasets(r.Context(), ws, id)
	if err != nil {
		fail(w, err, "list failed")
		return
	}
	out := make([]datasetResponse, 0, len(rows))
	var total int64
	for _, d := range rows {
		out = append(out, toDatasetResponse(d))
		total += d.SizeBytes
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"datasets":    out,
		"total_bytes": total,
	})
}

// DeleteDataset handles DELETE /test-cases/{id}/datasets/{datasetId}.
func (h *Handler) DeleteDataset(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	datasetID, ok := pathUUID(w, r, "datasetId")
	if !ok {
		return
	}
	ds, err := h.Svc.DeleteDataset(r.Context(), ws, id, datasetID)
	if err != nil {
		fail(w, err, "delete failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"deleted":    true,
		"dataset_id": pgconv.UUIDString(ds.ID),
		"note": "the stored file is removed; snapshots of past runs keep its name " +
			"and content hash so those runs stay traceable",
	})
}
