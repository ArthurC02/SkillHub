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

// writeTooLarge is the one place an upload refused for its size is worded, and
// the one place it is counted (03:INGEST-016). Both upload doors reach it, so
// the two do not drift into saying different things about the same ceiling.
//
// Two numbers, because the ceiling alone is not actionable: a creator told only
// "over the limit" cannot tell 1 KB over from 10 MB over, and only one of those
// is worth an afternoon. The actual size is the request's declared
// Content-Length and is printed ONLY when that declaration is itself over the
// ceiling — http.MaxBytesReader stops at limit+1 bytes and never learns how much
// more there was, so a chunked upload, or one that under-declares and overruns
// while being read, gets the ceiling and nothing else. A guessed number would be
// worse than no number.
//
// Nothing the caller sent appears here (NFR-001): not the file name, not a path,
// not a URL. Two integers the platform already knew, one of them a header field.
func writeTooLarge(w http.ResponseWriter, r *http.Request) {
	metrics.PackageSizeRefused.WithLabelValues(metrics.CeilingUpload).Inc()
	msg := "套件超過平台的上傳上限 " + skillpkg.HumanMB(skillpkg.MaxZipBytes) + "。"
	if r.ContentLength > skillpkg.MaxZipBytes {
		msg += "這一次送出的是 " + skillpkg.HumanMB(r.ContentLength) + "。"
	}
	httpx.WriteError(w, http.StatusRequestEntityTooLarge, msg)
}

// writeBadBody is what both upload doors answer when the request body itself
// could not be read (any failure other than the size ceiling, which
// writeTooLarge already covers): a truncated upload, a dropped connection, a
// client that closed early. There is nothing to blame on the content, because
// no content was ever fully received.
func writeBadBody(w http.ResponseWriter) {
	httpx.WriteError(w, http.StatusBadRequest, "讀不到上傳的內容，請重新選擇檔案再試一次")
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
			writeTooLarge(w, r)
			return
		}
		writeBadBody(w)
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
	// 04 丙-138. The sentence goes out without the sentinel that classified it:
	// `ErrFetch`'s own text is a Go identifier ("fetch failed"), and leaving it
	// on produced 「匯入失敗：fetch failed: …」 — the client already says which
	// action failed, so the prefix was a second 「失敗」 in a second language.
	// `errors.Is` above did the classification; nothing downstream needs the
	// word. The messages themselves are Chinese now (fetch.go), which is the
	// standard `writeTooLarge` in this same file already set.
	if errors.Is(err, ErrFetch) {
		httpx.WriteError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), ErrFetch.Error()+": "))
		return
	}
	// ErrBadArchive is deliberately NOT given the same treatment here. Its ~20
	// messages live in `shared/skillpkg`, they are shared with the packaging and
	// clean-mode paths, and several of them name a zip internal (entry name,
	// compression method) that is closer to a finding than to a sentence. That
	// is its own piece of work, not a side effect of this one — 04 丙-140.
	if errors.Is(err, skillpkg.ErrBadArchive) {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	// The upload direction of the generated-name guard. Without this branch it
	// fell into the 500 below — the user's own naming clash reported as the
	// platform being broken, with no next step (§2.2: a refusal says what to do).
	//
	// Two doors reach this now: an import whose manifest name collides with a
	// generated skill, and SaveVersion, where the caller picked the generated
	// skill by id and no name collided at all. So the rule comes first and the
	// remedy is named per door — telling the SaveVersion caller to 「改掉套件裡的
	// name」 would send them to do something that cannot work.
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
		Diagram         *struct {
			MediaType string `json:"media_type"`
			Data      string `json:"data"`
		} `json:"diagram"`
		ReferenceSkillIDs []string `json:"reference_skill_ids"`
	}
	// 6 MiB, up from 16 KiB: a decoded diagram is capped at 4 MB
	// (generateMaxDiagramBytes) and standard base64 inflates that to ~5.4 MB, so
	// the old text-only ceiling — which predates GEN-005 — would reject every
	// diagram upload as an oversized body before the request is even parsed.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 6<<20)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with a task_description, a diagram, or both")
		return
	}

	in := GenerateInput{TaskDescription: body.TaskDescription}
	if body.Diagram != nil {
		// Rejected on the RAW base64 text before decoding, so an oversized claim
		// cannot force an allocation the size cap exists to prevent — the same
		// reasoning llmclient's own MaxResponseBytes states for the response side.
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
		// Same discipline DISC-001 already applies to an empty search: say what
		// to add, do not just refuse (02:GEN-001).
		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"請描述你要完成的任務：做什麼、輸入是什麼、預期產出是什麼。")
	case errors.Is(err, ErrGenerateNoInput):
		// 02:GEN-005: neither input was given. A different sentence from
		// ErrGenerateBlank's on purpose — a caller who typed nothing and attached
		// nothing is not the same caller as one who typed three characters.
		// Still says what to add (02:GEN-001's advice rule, held by
		// TestABlankTaskDescriptionIsRefusedWithAdvice), plus the second way in.
		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"請描述你要完成的任務：做什麼、輸入是什麼、預期產出是什麼；或上傳一張流程圖／示意圖。兩者至少要有一項。")
	case errors.Is(err, ErrDiagramInvalid):
		// 400, not 422: a malformed request, the same class as the base64 checks
		// above, worded the same way so the two 400s do not read as two rules.
		httpx.WriteError(w, http.StatusBadRequest,
			"diagram 不是可用的圖片。接受的格式：PNG、JPEG、WebP，解碼後大小不超過 4 MB。")
	case errors.Is(err, ErrTooManyReferences):
		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"參考的 Skill 最多三個，請減少後再試一次。")
	case errors.Is(err, ErrReferenceUnavailable):
		// Never echoes which id, or why (not found vs. taken down vs. access
		// restricted vs. redistribution blocked): NFR-002/iron rule 3 both say not
		// to describe another workspace's private skill by name in a refusal.
		httpx.WriteError(w, http.StatusUnprocessableEntity,
			"其中一個參考的 Skill 無法使用，請換一個再試一次。")
	case errors.Is(err, policy.ErrGenerateQuotaExceeded):
		// 422 like every other allowance refusal: the request is well formed and
		// the platform is working, the account has nothing left in this window.
		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, policy.ErrAllowanceUnavailable):
		// 503, not the 502 below: the thing that failed is our own database, not
		// the model gateway, and the failure record says exactly that
		// (FailureUnavailable) — the live answer and the record must tell one
		// story. Static string: the wrapped pgx error is not for a response body.
		httpx.WriteError(w, http.StatusServiceUnavailable,
			"目前算不出這個工作區剩下的生成額度，所以沒有呼叫模型、也沒有花錢。稍後再試。")
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
			"這個工作區已經有一個同名的 Skill。"+
				"請先刪除它（或改掉它的名字），再生成一次——"+
				"生成永遠建立一個新的 Skill 的第一個版本，不會接在既有的 Skill 後面；"+
				"同一段任務描述再生成一次通常會取到同一個名字，改寫描述也會讓模型換名字。")
	case err != nil:
		httpx.WriteError(w, http.StatusBadGateway, "模型服務這一次沒有給出可用的結果。沒有建立任何版本，可以再試一次。")
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

// generateFailureLimit is how far back the failure list goes.
//
// A ceiling and not a pager: 02:GEN-003 asks for a record that can be looked at,
// and what a user looks for is "what happened the last few times". A page-two
// control on a list nobody scrolls is a second thing to get right.
const generateFailureLimit = 20 // one-number: generateFailureLimit

// GenerateFailure is one refused generation, as the workspace reads it back.
type GenerateFailure struct {
	// OccurredAt is when, and it is the only field guaranteed to be there.
	OccurredAt time.Time `json:"occurred_at"`
	// Failure is what went wrong, in the vocabulary GenerateSkill writes:
	// quota, unavailable, gateway, unpackageable, rejected, blocked. Empty when
	// a row's metadata could not be decoded — the row still happened.
	Failure string `json:"failure"`
	// Codes are the blocking finding codes, present only for `blocked`. Codes
	// and never values: a finding's message never carries the matched text
	// (iron rule 11) and this must not become the place that reintroduces it.
	Codes []string `json:"codes,omitempty"`
	// Attempts is how many gateway calls that failure cost. 0 for a refusal
	// that never reached the gateway — quota and unavailable.
	Attempts int `json:"attempts"`
	// Truncated and Collision distinguish the two failures a user can act on
	// from the ones they cannot: shorten the task, rename the other skill.
	Truncated bool `json:"truncated,omitempty"`
	Collision bool `json:"collision,omitempty"`
}

// GenerateFailures handles GET /skills/generate/failures (GEN-003).
//
// The read half of 「在工作區留下可查的失敗紀錄」. The write half has existed
// since the first pass; a row only the person with a database connection can see
// is not a record left in the workspace.
//
// Mounted on the same flag as Generate, because a failure list is a generation
// surface: a deployment that has not exposed generation has nothing to list, and
// a route that answers 200 with an empty array is still an answer about a
// feature ADR-052 says must not be discoverable.
//
// Workspace-scoped from the session, never from the request (iron rule 3).
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

// generateFailureFrom reads back what auditGenerateFailure wrote.
//
// Every field is optional on the way in. These rows are 400-day history written
// by whatever version of the code was running at the time, so a missing or
// re-typed key has to produce a row with less in it rather than an error: the
// alternative is a screen that goes blank because of something that happened
// last year.
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
