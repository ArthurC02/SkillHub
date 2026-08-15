package testlab

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/services/platform/internal/identity"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/httpx"
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

func (h *Handler) workspace(w http.ResponseWriter, r *http.Request) (gen.Workspace, bool) {
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return gen.Workspace{}, false
	}
	ws, err := h.Identity.PersonalWorkspace(r.Context(), user)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "workspace lookup failed")
		return gen.Workspace{}, false
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
	CreatedAt          string      `json:"created_at"`
	UpdatedAt          string      `json:"updated_at"`
}

func toTestCaseResponse(tc gen.TestCase) testCaseResponse {
	criteria, err := DecodeCriteria(tc.AcceptanceCriteria)
	if err != nil {
		criteria = []Criterion{}
	}
	return testCaseResponse{
		TestCaseID:         uuidString(tc.ID),
		SkillID:            uuidString(tc.SkillID),
		Name:               tc.Name,
		UserPrompt:         tc.UserPrompt,
		AcceptanceCriteria: criteria,
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
		DatasetID:   uuidString(d.ID),
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

// List handles GET /test-cases (TEST-001, WS-004).
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	rows, err := h.Svc.ListTestCases(r.Context(), ws)
	if err != nil {
		fail(w, err, "list failed")
		return
	}
	out := make([]testCaseResponse, 0, len(rows))
	for _, tc := range rows {
		out = append(out, toTestCaseResponse(tc))
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
	// cannot blank the name by omission.
	body := struct {
		Name       *string `json:"name"`
		UserPrompt *string `json:"user_prompt"`
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
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	tc, err := h.Svc.AddCriterion(r.Context(), ws, id, body.Text)
	if err != nil {
		fail(w, err, "add criterion failed")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toTestCaseResponse(tc))
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
//
// TODO(TEST-008/009): the pre-run permission summary (datasets, scripts, tools,
// network, secrets, provider and resource caps) and its re-confirmation on
// change are a different surface. They depend on the run permission model, so
// they belong with 03:TEST-008 and 03:TEST-009, not here.
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
	datasetID, ok := pathUUID(w, r, "datasetId")
	if !ok {
		return
	}
	ds, err := h.Svc.DeleteDataset(r.Context(), ws, datasetID)
	if err != nil {
		fail(w, err, "delete failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"deleted":    true,
		"dataset_id": uuidString(ds.ID),
		"note": "the stored file is removed; snapshots of past runs keep its name " +
			"and content hash so those runs stay traceable",
	})
}
