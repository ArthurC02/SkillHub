package ingest

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

// Handler exposes skill import endpoints (contracts/openapi/public.yaml).
type Handler struct {
	Svc      *Service
	Identity *identity.Service
}

// UploadResult is public.yaml's UploadResult. Exported because
// POST /skills/{id}/versions/from-suggestions answers with this shape plus two
// fields (contract: `allOf`), and a second copy of it in internal/eval is exactly
// how two endpoints start describing the same version differently.
type UploadResult struct {
	SkillID       string                       `json:"skill_id"`
	VersionID     string                       `json:"version_id"`
	VersionNumber int32                        `json:"version_number"`
	ContentHash   string                       `json:"content_hash"`
	Duplicate     bool                         `json:"duplicate"`
	Findings      skillpkg.CategorizedFindings `json:"findings"`
}

// NewUploadResult renders one stored version the way every creation path does.
func NewUploadResult(res Result) UploadResult {
	return UploadResult{
		SkillID:       pgconv.UUIDString(res.Skill.ID),
		VersionID:     pgconv.UUIDString(res.Version.ID),
		VersionNumber: res.Version.VersionNumber,
		ContentHash:   res.Version.ContentHash,
		Duplicate:     res.Duplicate,
		Findings:      res.Report.Categorize(),
	}
}

// Upload handles POST /skills/import/upload. Wrap with RequireSession; the
// workspace is derived from the session, never from the client (iron rule 3).
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	ws, err := h.Identity.PersonalWorkspace(r.Context(), user)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "workspace lookup failed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, skillpkg.MaxZipBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "package exceeds the upload size limit")
			return
		}
		httpx.WriteError(w, http.StatusBadRequest, "could not read request body")
		return
	}

	res, err := h.Svc.UploadZip(r.Context(), ws, data)
	h.respond(w, res, err)
}

// SaveVersion handles POST /skills/{id}/versions (WS-002). Wrap with
// RequireSession.
func (h *Handler) SaveVersion(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	ws, err := h.Identity.PersonalWorkspace(r.Context(), user)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "workspace lookup failed")
		return
	}

	var skillID pgtype.UUID
	if err := skillID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, ErrSkillNotFound.Error())
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, skillpkg.MaxZipBytes)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			httpx.WriteError(w, http.StatusRequestEntityTooLarge, "package exceeds the upload size limit")
			return
		}
		httpx.WriteError(w, http.StatusBadRequest, "could not read request body")
		return
	}

	res, err := h.Svc.SaveVersion(r.Context(), ws, skillID, data)
	if errors.Is(err, ErrSkillNotFound) {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	h.respond(w, res, err)
}

// ImportURL handles POST /skills/import/url. Wrap with RequireSession.
func (h *Handler) ImportURL(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	ws, err := h.Identity.PersonalWorkspace(r.Context(), user)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "workspace lookup failed")
		return
	}

	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil || body.URL == "" {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with a non-empty url")
		return
	}

	res, err := h.Svc.ImportURL(r.Context(), ws, body.URL)
	h.respond(w, res, err)
}

func (h *Handler) respond(w http.ResponseWriter, res Result, err error) {
	if errors.Is(err, skillpkg.ErrBadArchive) || errors.Is(err, ErrFetch) {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "import failed")
		return
	}

	if res.Report.Blocked {
		// SKILL-001/INGEST-008: failure result carries error/warning/info
		// findings as separate lists, not one undifferentiated feed.
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, res.Report.Categorize())
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, NewUploadResult(res))
}

// GenerateResponse is public.yaml's GenerateSkillResult: one accepted package
// plus the two facts about how it got here that the page has to say out loud.
type GenerateResponse struct {
	UploadResult
	// Attempts is 1 or 2. Shown because 02:GEN-003 forbids the UI promising a
	// success rate, and "it took two goes" is the honest version of the same
	// information — measured, one retry moves 80% to 90%, not to nothing.
	Attempts int `json:"attempts"`
	// Model and PromptVersion are the service's own, not the model's.
	Model         string `json:"generator_model"`
	PromptVersion string `json:"generator_prompt_version"`
}

// GenerateRejected is the 422 for a package that failed validation: the same
// findings a failed import returns, plus how many attempts produced them.
//
// Deliberately a superset of the import shape rather than a different one — the
// web renders both with one component, and 02:GEN-003 requires the findings
// 逐字 on both paths.
type GenerateRejected struct {
	skillpkg.CategorizedFindings
	Attempts int `json:"attempts"`
}

// Generate handles POST /skills/generate (GEN-001, GEN-008).
//
// Mounted only where the exposure flag is on (ADR-052): a beta participant who
// can see "搜不到 → 生成一個" changes what the funnel's first segment measures,
// and that number has one chance with twelve people. Off, this route does not
// exist and /me does not advertise it.
//
// Synchronous, and that is a product fact the page has to state rather than
// hide: there is no job and no run_id, so a closed tab cancels the request. Wrap
// with RequireSession; the workspace comes from the session (iron rule 3).
func (h *Handler) Generate(w http.ResponseWriter, r *http.Request) {
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	ws, err := h.Identity.PersonalWorkspace(r.Context(), user)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "workspace lookup failed")
		return
	}

	var body struct {
		TaskDescription string `json:"task_description"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with a task_description")
		return
	}

	res, err := h.Svc.GenerateSkill(r.Context(), ws, body.TaskDescription)
	switch {
	case errors.Is(err, ErrGenerateBlank):
		// Same discipline DISC-001 already applies to an empty search: say what
		// to add, do not just refuse (02:GEN-001).
		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"請描述你要完成的任務：做什麼、輸入是什麼、預期產出是什麼。")
	case errors.Is(err, policy.ErrGenerateQuotaExceeded):
		// 422 like every other allowance refusal: the request is well formed and
		// the platform is working, the account has nothing left in this window.
		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, llmclient.ErrGenerateTruncated):
		// 02:GEN-001: tell the user this specific thing, and do not retry it.
		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"這件事的內容超過一次生成的上限，已經停下來，沒有建立任何版本。"+
				"把任務拆小一點再試一次會有幫助。")
	case errors.Is(err, ErrGenerateTooLong):
		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"這段任務描述超過一次生成能吃下的長度。請留下要做什麼、輸入是什麼、預期產出是什麼，其餘可以省略。")
	case errors.Is(err, ErrGenerateInFlight):
		// 409 rather than 422: nothing is wrong with the request, it is the
		// workspace that is busy, and the same request will work in a moment.
		httpx.WriteError(w, http.StatusConflict,
			"這個工作區已經有一次生成正在進行。等它結束再送出——同時跑兩次會付兩次錢。")
	case errors.Is(err, ErrGenerateNotForCatalogue):
		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrGeneratedNameCollision):
		// 422 and not 500: the request was fine, the workspace is in a state the
		// platform will not silently merge. Nothing was written.
		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"這個工作區已經有一個同名的 Skill，而它不是生成的。"+
				"請先改掉那一個的名字或刪除它，再生成一次——"+
				"平台不會把生成的內容接在別的 Skill 後面當成新版本。")
	case err != nil:
		httpx.WriteError(w, http.StatusBadGateway, "generation failed")
	case res.Report.Blocked:
		// The findings verbatim, exactly as a failed import gets them
		// (02:GEN-003, SKILL-002). Nothing was written: prepare returns before the
		// object store and importZip before the transaction.
		//
		// `attempts` rides along because the failure screen has a sentence about
		// the automatic retry, and that sentence is FALSE for a `possible-secret`
		// finding, which ADR-048 says is not retried. Without this the page cannot
		// tell the two apart and says "we already tried twice" about one attempt.
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, GenerateRejected{
			CategorizedFindings: res.Report.Categorize(),
			Attempts:            res.Attempts,
		})
	default:
		httpx.WriteJSON(w, http.StatusCreated, GenerateResponse{
			UploadResult:  NewUploadResult(res.Result),
			Attempts:      res.Attempts,
			Model:         res.Model,
			PromptVersion: res.PromptVersion,
		})
	}
}
