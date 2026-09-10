package ingest

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

type Handler struct {
	Svc      *Service
	Identity *identity.Service
}

type UploadResult struct {
	SkillID       string                       `json:"skill_id"`
	VersionID     string                       `json:"version_id"`
	VersionNumber int32                        `json:"version_number"`
	ContentHash   string                       `json:"content_hash"`
	Duplicate     bool                         `json:"duplicate"`
	Findings      skillpkg.CategorizedFindings `json:"findings"`
}

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

func writeTooLarge(w http.ResponseWriter, r *http.Request) {
	metrics.PackageSizeRefused.WithLabelValues(metrics.CeilingUpload).Inc()
	msg := "套件超過平台的上傳上限 " + skillpkg.HumanMB(skillpkg.MaxZipBytes) + "。"
	if r.ContentLength > skillpkg.MaxZipBytes {
		msg += "這一次送出的是 " + skillpkg.HumanMB(r.ContentLength) + "。"
	}
	httpx.WriteError(w, http.StatusRequestEntityTooLarge, msg)
}

func writeBadBody(w http.ResponseWriter) {
	httpx.WriteError(w, http.StatusBadRequest, "讀不到上傳的內容，請重新選擇檔案再試一次")
}

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
			writeTooLarge(w, r)
			return
		}
		writeBadBody(w)
		return
	}

	res, err := h.Svc.UploadZip(r.Context(), ws, data)
	h.respond(w, res, err)
}

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
			writeTooLarge(w, r)
			return
		}
		writeBadBody(w)
		return
	}

	res, err := h.Svc.SaveVersion(r.Context(), ws, skillID, data)
	if errors.Is(err, ErrSkillNotFound) {
		httpx.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	h.respond(w, res, err)
}

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

	if errors.Is(err, ErrFetch) {
		httpx.WriteError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), ErrFetch.Error()+": "))
		return
	}

	if errors.Is(err, skillpkg.ErrBadArchive) {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if errors.Is(err, ErrGeneratedNameCollision) {
		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"平台不會把上傳的內容接在生成的 Skill 後面當成新版本——"+
				"接上去的版本會沿用生成 Skill 的搜尋排除，連你自己都再也搜不到它。"+
				"要為生成的內容加你自己的版本，請把它匯入成一個新的 Skill；"+
				"如果是匯入時撞到同名的生成 Skill，請先刪除它，或改掉套件裡的 name。")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "import failed")
		return
	}

	if res.Report.Blocked {

		httpx.WriteJSON(w, http.StatusUnprocessableEntity, res.Report.Categorize())
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, NewUploadResult(res))
}

type GenerateResponse struct {
	UploadResult

	Attempts int `json:"attempts"`

	Model         string `json:"generator_model"`
	PromptVersion string `json:"generator_prompt_version"`
}

type GenerateRejected struct {
	skillpkg.CategorizedFindings
	Attempts int `json:"attempts"`
}

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
		Diagram         *struct {
			MediaType string `json:"media_type"`
			Data      string `json:"data"`
		} `json:"diagram"`
		ReferenceSkillIDs []string `json:"reference_skill_ids"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 6<<20)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with a task_description, a diagram, or both")
		return
	}

	in := GenerateInput{TaskDescription: body.TaskDescription}
	if body.Diagram != nil {

		if len(body.Diagram.Data) > base64.StdEncoding.EncodedLen(generateMaxDiagramBytes)+4 {
			httpx.WriteError(w, http.StatusBadRequest,
				"diagram 超過 4 MB 上限，或不是合法的 base64。接受的格式：PNG、JPEG、WebP。")
			return
		}
		decoded, err := base64.StdEncoding.DecodeString(body.Diagram.Data)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "diagram.data 不是合法的 base64。")
			return
		}
		in.Diagram = &GenerateDiagram{MediaType: body.Diagram.MediaType, Data: decoded}
	}
	for _, raw := range body.ReferenceSkillIDs {
		var id pgtype.UUID
		if err := id.Scan(raw); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "reference_skill_ids 裡有不合法的 id。")
			return
		}
		in.ReferenceSkillIDs = append(in.ReferenceSkillIDs, id)
	}

	res, err := h.Svc.GenerateSkill(r.Context(), ws, in)
	switch {
	case errors.Is(err, ErrGenerateBlank):

		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"請描述你要完成的任務：做什麼、輸入是什麼、預期產出是什麼。")
	case errors.Is(err, ErrGenerateNoInput):

		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"請描述你要完成的任務：做什麼、輸入是什麼、預期產出是什麼；或上傳一張流程圖／示意圖。兩者至少要有一項。")
	case errors.Is(err, ErrDiagramInvalid):

		httpx.WriteError(w, http.StatusBadRequest,
			"diagram 不是可用的圖片。接受的格式：PNG、JPEG、WebP，解碼後大小不超過 4 MB。")
	case errors.Is(err, ErrTooManyReferences):
		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"參考的 Skill 最多三個，請減少後再試一次。")
	case errors.Is(err, ErrReferenceUnavailable):

		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"其中一個參考的 Skill 無法使用，請換一個再試一次。")
	case errors.Is(err, policy.ErrGenerateQuotaExceeded):

		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, policy.ErrAllowanceUnavailable):

		httpx.WriteError(w, http.StatusServiceUnavailable,
			"目前算不出這個工作區剩下的生成額度，所以沒有呼叫模型、也沒有花錢。稍後再試。")
	case errors.Is(err, llmclient.ErrGenerateTruncated):

		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"這件事的內容超過一次生成的上限，已經停下來，沒有建立任何版本。"+
				"把任務拆小一點再試一次會有幫助。")
	case errors.Is(err, ErrGenerateTooLong):
		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"這段任務描述超過一次生成能吃下的長度。請留下要做什麼、輸入是什麼、預期產出是什麼，其餘可以省略。")
	case errors.Is(err, ErrGenerateInFlight):

		httpx.WriteError(w, http.StatusConflict,
			"這個工作區已經有一次生成正在進行。等它結束再送出——同時跑兩次會付兩次錢。")
	case errors.Is(err, ErrCreditThreshold):
		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"點數不足，無法開始這次生成，沒有呼叫模型。請聯絡管理者為這個帳號加點。")
	case errors.Is(err, ErrGenerateNotForCatalogue):
		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrGeneratedNameCollision):

		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"這個工作區已經有一個同名的 Skill。"+
				"請先刪除它（或改掉它的名字），再生成一次——"+
				"生成永遠建立一個新的 Skill 的第一個版本，不會接在既有的 Skill 後面；"+
				"同一段任務描述再生成一次通常會取到同一個名字，改寫描述也會讓模型換名字。")
	case err != nil:
		httpx.WriteError(w, http.StatusBadGateway, "模型服務這一次沒有給出可用的結果。沒有建立任何版本，可以再試一次。")
	case res.Report.Blocked:

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

const generateFailureLimit = 20 // one-number: generateFailureLimit

type GenerateFailure struct {
	OccurredAt time.Time `json:"occurred_at"`

	Failure string `json:"failure"`

	Codes []string `json:"codes,omitempty"`

	Attempts int `json:"attempts"`

	Truncated bool `json:"truncated,omitempty"`
	Collision bool `json:"collision,omitempty"`
}

func (h *Handler) GenerateFailures(w http.ResponseWriter, r *http.Request) {
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
	records, err := audit.ListForWorkspace(r.Context(), h.Svc.Pool, ws.ID,
		[]string{audit.ActionSkillGenerateFailed}, generateFailureLimit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failure history could not be read")
		return
	}
	out := make([]GenerateFailure, 0, len(records))
	for _, rec := range records {
		out = append(out, generateFailureFrom(rec))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"failures": out})
}

func generateFailureFrom(rec audit.Record) GenerateFailure {
	f := GenerateFailure{OccurredAt: rec.OccurredAt}
	if s, ok := rec.Metadata["failure"].(string); ok {
		f.Failure = s
	}
	if n, ok := rec.Metadata["attempts"].(float64); ok {
		f.Attempts = int(n)
	}
	f.Truncated, _ = rec.Metadata["truncated"].(bool)
	f.Collision, _ = rec.Metadata["collision"].(bool)
	if raw, ok := rec.Metadata["codes"].([]any); ok {
		for _, c := range raw {
			if s, ok := c.(string); ok {
				f.Codes = append(f.Codes, s)
			}
		}
	}
	return f
}
