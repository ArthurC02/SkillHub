package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

type ObjectStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
}

var errSkillNotFound = errors.New("找不到這個 Skill")

var errOwnerReadNotConfigured = errors.New("catalog: owner read is not configured")

const maxSkillMDBytes = 1 << 20

type labelled struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Note  string `json:"note"`
}

type sourceInfo struct {
	Type          string `json:"type"`
	URL           string `json:"url,omitempty"`
	SourceVersion string `json:"source_version,omitempty"`
	FetchedAt     string `json:"fetched_at,omitempty"`
	ContentHash   string `json:"content_hash,omitempty"`

	LastCheckedAt    string `json:"last_checked_at,omitempty"`
	UnavailableSince string `json:"unavailable_since,omitempty"`

	TaskDescription        string `json:"task_description,omitempty"`
	GeneratorModel         string `json:"generator_model,omitempty"`
	GeneratorPromptVersion string `json:"generator_prompt_version,omitempty"`

	GenerationInputs json.RawMessage `json:"generation_inputs,omitempty"`
	Trust            labelled        `json:"trust"`
}

type licenseInfo struct {
	Expression string `json:"expression,omitempty"`

	Source     string   `json:"source,omitempty"`
	SourceNote string   `json:"source_note,omitempty"`
	Status     labelled `json:"status"`
}

var licenseSourceNotes = map[string]string{
	"manifest":                 "作者在 SKILL.md frontmatter 自行宣告。",
	"manifest-referenced-file": "frontmatter 未直接宣告授權,而是指向套件內的檔案(如 `SEE LICENSE IN LICENSE.txt`);此結果讀自該檔案的文字。",
	"package-license-file":     "套件內附 LICENSE 檔案,涵蓋此套件本身。",
	"repo-license-file":        "來自 repo 根目錄的 LICENSE,涵蓋整個 repo,不必然涵蓋此子目錄的內容。",
}

type severityCounts struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Infos    int `json:"infos"`
}

type riskSummary struct {
	ScanStatus string         `json:"scan_status"`
	Counts     severityCounts `json:"counts"`

	Highlights []skillpkg.Finding `json:"highlights"`
	InfoCounts map[string]int     `json:"info_counts"`

	Disclosures []disclosure `json:"disclosures"`

	Note string `json:"note"`
}

type compatibility struct {
	SpecValidation labelled `json:"spec_validation"`
	Capability     labelled `json:"capability"`
	Runtime        labelled `json:"runtime"`
	RuntimeImage   string   `json:"runtime_image,omitempty"`
	MeasuredAt     string   `json:"measured_at,omitempty"`
	Note           string   `json:"note"`
}

type axisWords map[string][2]string

var (
	specWords = axisWords{
		"passed":     {"通過", ""},
		"failed":     {"未通過", "套件格式或引用有問題,匯入時已標記。"},
		"unverified": {"未驗證", ""},
	}
	capabilityWords = axisWords{

		"activated": {"已啟用",
			"該次試跑的 Prompt **點名了這個 Skill**(`02:CONTENT-007` 的要求),所以這一格說的是" +
				"「被點名時載得起來」,不是「Agent 會自己想到要用它」。後者平台量過:自主觸發基準率是 0(PDM-011)。"},
		"not_activated": {"未被啟用",
			"該次試跑完成了,Trace 顯示這個 Skill 被提供但用了別的。" +
				"**不是從「沒有事件」推定的**——SDK 訊息流表達不了「可用但沒被叫」(TRACE-002)。"},
		"unverified": {"未驗證", ""},
	}
	runtimeWords = axisWords{

		"native": {"映像提供了它宣告的執行環境",
			"套件腳本宣告的 Runtime 這個映像有,所以腳本可以是真正執行的那個東西。" +
				"**這是一條規則的結論,不是一次觀察**——平台沒有查那次 Run 裡腳本有沒有真的跑、跑成功沒有。"},
		"transpiled": {"腳本未執行,由模型轉譯",
			"套件宣告的 Runtime 這個映像沒有,所以那次 Run 的產出來自模型重寫程式碼、不是執行它。" +
				"那次 Run 有產出,但做事的不是 Skill 自己的程式碼——這一格決定你拿到的是不是你以為的東西。" +
				"**同樣是規則的結論**:判準是映像有沒有那個 Runtime,不是觀察到模型在改寫。"},
		"failed":     {"腳本無法執行", "套件宣告的 Runtime 這個映像沒有,該次 Run 因此失敗。"},
		"unverified": {"未驗證", ""},
	}
)

func axis(w axisWords, value string) labelled {
	if v, ok := w[value]; ok {
		return labelled{Value: value, Label: v[0], Note: v[1]}
	}
	return labelled{
		Value: value,
		Label: value,
		Note:  "這個平台版本沒有這個值的說明,值照原樣顯示,不猜測它的意思。",
	}
}

func unverifiedCompat() compatibility {
	return compatibility{
		SpecValidation: axis(specWords, "unverified"),
		Capability:     axis(capabilityWords, "unverified"),
		Runtime:        axis(runtimeWords, "unverified"),
		Note:           compatUnverifiedNote,
	}
}

type enrichmentInfo struct {
	Status        string               `json:"status"`
	Summary       string               `json:"summary,omitempty"`
	TaskExamples  []string             `json:"task_examples,omitempty"`
	Tags          *llmclient.SkillTags `json:"tags,omitempty"`
	Model         string               `json:"model,omitempty"`
	PromptVersion string               `json:"prompt_version,omitempty"`
	Note          string               `json:"note"`
}

type limitation struct {
	Text   string `json:"text"`
	Source string `json:"source"`
}

const (
	limitSourceModel = "model"
	limitSourceScan  = "scan"
)

var scanLimitations = map[string]string{
	"external-url":    "套件內含外部連結,執行時可能需要對外網路存取。",
	"script-file":     "套件內含 Script,需可執行 Script 的環境,且應先自行檢視內容。",
	"embedded-script": "SKILL.md 內嵌可執行程式碼,需可執行該語言的環境,且應先自行檢視內容。",
	"dependency-file": "套件宣告外部套件相依,使用前需自行安裝。",
	"binary-file":     "套件內含編譯後的二進位檔,內容無法以文字檢視,且可能綁定特定平台。",
}

type versionInfo struct {
	VersionID     string `json:"version_id"`
	VersionNumber int32  `json:"version_number"`
	ContentHash   string `json:"content_hash"`
	CreatedAt     string `json:"created_at"`
}

type derivationInfo struct {
	IsFork              bool   `json:"is_fork"`
	ForkedFromSkillID   string `json:"forked_from_skill_id,omitempty"`
	ForkedFromVersionID string `json:"forked_from_version_id,omitempty"`
	Label               string `json:"label"`
	Note                string `json:"note"`
}

type accessRestriction struct {
	Reason string `json:"reason"`
	Note   string `json:"note"`
}

type skillDetail struct {
	SkillID string `json:"skill_id"`
	Name    string `json:"name"`

	Summary string `json:"summary"`

	Scope string   `json:"scope"`
	Tier  labelled `json:"tier"`

	Category   labelled       `json:"category"`
	Enrichment enrichmentInfo `json:"enrichment"`

	Limitations []limitation `json:"limitations"`
	Version     *versionInfo `json:"version,omitempty"`
	Source      *sourceInfo  `json:"source,omitempty"`
	License     licenseInfo  `json:"license"`

	Redistribution labelled       `json:"redistribution"`
	Derivation     derivationInfo `json:"derivation"`
	AllowedTools   []string       `json:"allowed_tools,omitempty"`
	Risk           riskSummary    `json:"risk"`
	Compat         compatibility  `json:"compatibility"`

	Restriction *accessRestriction `json:"access_restriction,omitempty"`
}

type fileEntry struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	IsScript bool   `json:"is_script"`
}

type skillFiles struct {
	SkillID        string      `json:"skill_id"`
	VersionID      string      `json:"version_id"`
	VersionNumber  int32       `json:"version_number"`
	SkillMD        string      `json:"skill_md"`
	SkillMDTrunc   bool        `json:"skill_md_truncated"`
	Tree           []fileEntry `json:"tree"`
	EmbeddedScript *string     `json:"embedded_script_note,omitempty"`
	Note           string      `json:"note"`
}

func (h *Handler) SkillDetail(w http.ResponseWriter, r *http.Request) {
	skill, scope, ok := h.resolveSkill(w, r)
	if !ok {
		return
	}
	out, err := h.Svc.SkillDetail(r.Context(), skill)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "skill detail failed")
		return
	}

	out.Scope = scope
	h.recordDetailView(r, skill.ID)
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (s *Service) SkillDetail(ctx context.Context, skill SkillFacts) (skillDetail, error) {
	q := gen.New(s.Pool)

	out := skillDetail{
		SkillID: pgconv.UUIDString(skill.ID),
		Name:    skill.Name,

		Tier: tierLabel(TierIndexed),

		Category:    categoryLabel(skill.Category, skill.CategorySource),
		Limitations: []limitation{},
		Derivation:  derivation(skill),
		License:     licenseInfo{Status: statusLabel(LicenseStatusUnknown)},

		Redistribution: redistributionLabel(skill.Redistribution),
		Risk: riskSummary{
			ScanStatus:  "unavailable",
			InfoCounts:  map[string]int{},
			Highlights:  []skillpkg.Finding{},
			Disclosures: []disclosure{},
			Note:        riskNote,
		},
		Compat: unverifiedCompat(),
	}
	if skill.Summary != nil {
		out.Summary = *skill.Summary
	}
	out.Restriction = restrictionOf(skill)

	if e, err := q.GetSkillEnrichment(ctx, gen.GetSkillEnrichmentParams{
		SkillID: skill.ID, WorkspaceID: skill.WorkspaceID,
	}); err == nil {
		out.Enrichment = enrichmentFrom(e)
		out.Limitations = modelLimitations(e.Limitations)
		if out.Summary == "" {
			out.Summary = e.Summary
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return skillDetail{}, err
	}

	if s.ReadLatestVersion == nil {
		return skillDetail{}, errOwnerReadNotConfigured
	}
	ver, found, err := s.ReadLatestVersion(ctx, skill.WorkspaceID, skill.ID)
	if !found && err == nil {

		return out, nil
	}
	if err != nil {
		return skillDetail{}, err
	}
	out.Version = &versionInfo{
		VersionID:     pgconv.UUIDString(ver.ID),
		VersionNumber: ver.VersionNumber,
		ContentHash:   ver.ContentHash,
		CreatedAt:     timeString(ver.CreatedAt),
	}
	out.License = licenseFrom(ver)
	out.Tier = tierLabel(curationTier(skill, ver.ID))

	if s.ReadRuntimeCompatibility == nil {
		return skillDetail{}, errOwnerReadNotConfigured
	}
	if c, found, err := s.ReadRuntimeCompatibility(ctx, ver.ID); err == nil && found {
		out.Compat.Capability = axis(capabilityWords, c.Capability)
		out.Compat.Runtime = axis(runtimeWords, c.Runtime)
		out.Compat.RuntimeImage = c.RuntimeImage
		out.Compat.MeasuredAt = timeString(c.MeasuredAt)
		out.Compat.Note = compatMeasuredNote
	} else if err != nil {
		return skillDetail{}, err
	}

	if ver.SourceID.Valid {
		if s.SourceByID == nil {
			return skillDetail{}, errOwnerReadNotConfigured
		}
		src, found, err := s.SourceByID(ctx, ver.WorkspaceID, ver.SourceID)
		if err == nil && found {
			out.Source = sourceFrom(src)
		} else if err != nil {
			return skillDetail{}, err
		}
	}

	if report, ok := s.scanPackage(ctx, ver.PackageObjectKey); ok {
		out.Risk = summarizeRisk(report)
		out.Compat.SpecValidation = axis(specWords, specValidation(report))
		out.Limitations = append(out.Limitations, scanDerivedLimitations(report)...)
		if report.Manifest != nil {
			out.AllowedTools = report.Manifest.AllowedTools
		}
	}
	return out, nil
}

func (h *Handler) recordDetailView(r *http.Request, skillID pgtype.UUID) {
	if !h.Svc.Analytics.Enabled() {
		return
	}

	if r.URL.Query().Get("view") == "embedded" {
		return
	}
	var workspace pgtype.UUID
	if user, ok := identity.SessionUser(r.Context()); ok {
		if ws, err := h.Identity.PersonalWorkspace(r.Context(), user); err == nil {
			workspace = ws.ID
		}
	}

	h.Svc.Analytics.SkillDetailViewed(r.Context(), workspace, skillID)
}

func (h *Handler) SkillFiles(w http.ResponseWriter, r *http.Request) {
	skill, _, ok := h.resolveSkill(w, r)
	if !ok {
		return
	}

	if rest := restrictionOf(skill); rest != nil {
		httpx.WriteError(w, http.StatusForbidden, rest.Note)
		return
	}
	out, err := h.Svc.SkillFiles(r.Context(), skill)
	switch {
	case errors.Is(err, errNoSavedVersion):
		httpx.WriteError(w, http.StatusNotFound, errNoSavedVersion.Error())
		return
	case errors.Is(err, errPackageUnreadable):
		httpx.WriteError(w, http.StatusServiceUnavailable, errPackageUnreadable.Error())
		return
	case err != nil:
		httpx.WriteError(w, http.StatusInternalServerError, "skill files failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

var (
	errNoSavedVersion    = errors.New("這個 Skill 還沒有保存的版本")
	errPackageUnreadable = errors.New("儲存的套件目前讀不到，稍後再試一次")
)

func (s *Service) SkillFiles(ctx context.Context, skill SkillFacts) (skillFiles, error) {
	if s.ReadLatestVersion == nil {
		return skillFiles{}, errOwnerReadNotConfigured
	}
	ver, found, err := s.ReadLatestVersion(ctx, skill.WorkspaceID, skill.ID)
	if !found && err == nil {
		return skillFiles{}, errNoSavedVersion
	}
	if err != nil {
		return skillFiles{}, err
	}

	data, err := s.storeGet(ctx, ver.PackageObjectKey)
	if err != nil {
		return skillFiles{}, errPackageUnreadable
	}
	fsys, err := skillpkg.PackageFS(data)
	if err != nil {
		return skillFiles{}, errPackageUnreadable
	}

	out := skillFiles{
		SkillID:       pgconv.UUIDString(skill.ID),
		VersionID:     pgconv.UUIDString(ver.ID),
		VersionNumber: ver.VersionNumber,
		Tree:          fileTree(fsys),
		Note:          filesNote,
	}
	if md, err := fs.ReadFile(fsys, "SKILL.md"); err == nil {
		if len(md) > maxSkillMDBytes {
			md, out.SkillMDTrunc = md[:maxSkillMDBytes], true
		}

		out.SkillMD = strings.ToValidUTF8(string(md), "")
	}

	for _, f := range skillpkg.Validate(fsys).Findings {
		if f.Code == "embedded-script" {
			msg := f.Message
			out.EmbeddedScript = &msg
			break
		}
	}
	return out, nil
}

func restrictionOf(s SkillFacts) *accessRestriction {
	if s.AccessRestriction == nil || strings.TrimSpace(*s.AccessRestriction) == "" {
		return nil
	}
	note, ok := restrictionNotes[*s.AccessRestriction]
	if !ok {

		note = restrictionNoteDefault
	}
	return &accessRestriction{Reason: *s.AccessRestriction, Note: note}
}

var restrictionNotes = map[string]string{
	"license-review": "此 Skill 的來源授權正在審查中:在審查結論出來前,平台不提供 " +
		"SKILL.md 全文與套件檔案樹,也不接受在平台上試跑。摘要、限制、依賴、來源與授權資訊照常顯示," +
		"原文請至上方來源連結所指的位置檢視。這是對授權條款的保守處置,不代表此 Skill 有安全或品質問題。",
}

const restrictionNoteDefault = "此 Skill 目前因授權因素受限:不提供 SKILL.md 全文與套件檔案樹," +
	"也不接受在平台上試跑。摘要與來源資訊照常顯示。"

const (
	riskNote = "以上為靜態掃描結果:匯入與掃描期間不執行套件內任何程式碼。" +
		"通過掃描不等於安全或有效,請自行檢視。"
	compatUnverifiedNote = "規格驗證只檢查套件格式。能力相容與執行環境相容要等這個版本在 Sandbox 跑過一次才有結果," +
		"沒跑過一律標示為未驗證,不代表相容。"

	compatMeasuredNote = "這兩軸的來源不同:**能力相容**來自該次 Run 的 Trace,是觀察到的事件;" +
		"**執行環境相容**來自一條規則——「這個映像有沒有提供套件腳本宣告的執行環境」——" +
		"平台沒有觀察腳本是否真的執行成功。兩者都只對下方 runtime_image 成立,換一個執行映像要重新判定。" +
		"執行環境相容為「模型轉譯」時,做事的是模型對套件腳本的改寫,不是腳本本身。"
	filesNote = "tree 為套件內檔案清單與大小;目前僅回傳 SKILL.md 全文。" +
		"其他單檔內容的讀取端點屬 DISC-007 後續工作項,尚未實作。"
	enrichPendingNote = "尚未產生模型摘要;顯示的是套件自身的 frontmatter description。"

	enrichedNote = "本區塊由模型產生(非套件作者撰寫),僅供理解用途。" +
		"**你的 Agent 讀的不是這一段**——它讀的是套件自己的 `description`(上方「摘要」)," +
		"而下載回去的套件裡也不會有這一段。這段寫得好不會讓 Agent 更願意用它。"
)

func (h *Handler) resolveSkill(w http.ResponseWriter, r *http.Request) (SkillFacts, string, bool) {
	var id pgtype.UUID
	if err := id.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, errSkillNotFound.Error())
		return SkillFacts{}, "", false
	}
	ctx := r.Context()

	skill, found, err := h.Svc.CatalogSkill(ctx, id)
	if err == nil && found {

		if skill.TakedownAt.Valid {
			httpx.WriteError(w, http.StatusGone, "這個 Skill 已從目錄下架")
			return SkillFacts{}, "", false
		}
		return skill, "catalog", true
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Skill 讀取失敗，稍後再試一次")
		return SkillFacts{}, "", false
	}

	user, ok := identity.SessionUser(ctx)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, errSkillNotFound.Error())
		return SkillFacts{}, "", false
	}
	ws, err := h.Identity.PersonalWorkspace(ctx, user)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "工作區讀取失敗，稍後再試一次")
		return SkillFacts{}, "", false
	}
	skill, found, err = h.Svc.WorkspaceSkill(ctx, id, ws.ID)
	if !found && err == nil {
		httpx.WriteError(w, http.StatusNotFound, errSkillNotFound.Error())
		return SkillFacts{}, "", false
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "Skill 讀取失敗，稍後再試一次")
		return SkillFacts{}, "", false
	}
	return skill, "private", true
}

func (s *Service) CatalogSkill(ctx context.Context, id pgtype.UUID) (SkillFacts, bool, error) {
	if s.ReadCatalogSkill == nil {
		return SkillFacts{}, false, errOwnerReadNotConfigured
	}
	return s.ReadCatalogSkill(ctx, id)
}

func (s *Service) WorkspaceSkill(ctx context.Context, id, workspaceID pgtype.UUID) (SkillFacts, bool, error) {
	if s.ReadWorkspaceSkill == nil {
		return SkillFacts{}, false, errOwnerReadNotConfigured
	}
	return s.ReadWorkspaceSkill(ctx, workspaceID, id)
}

func (s *Service) storeGet(ctx context.Context, key string) ([]byte, error) {
	if s.Store == nil {
		return nil, errors.New("catalog: no object store configured")
	}
	return s.Store.Get(ctx, key)
}

func (s *Service) scanPackage(ctx context.Context, key string) (skillpkg.Report, bool) {
	data, err := s.storeGet(ctx, key)
	if err != nil {
		return skillpkg.Report{}, false
	}
	fsys, err := skillpkg.PackageFS(data)
	if err != nil {
		return skillpkg.Report{}, false
	}
	return skillpkg.Validate(fsys), true
}

func tierLabel(t Tier) labelled {
	d := t.Display()
	return labelled{Value: string(t), Label: d.Badge, Note: d.TrustIndicator}
}

func curationTier(skill SkillFacts, latestVersionID pgtype.UUID) Tier {
	if skill.CurationTier != string(TierCurated) {
		return TierIndexed
	}
	if !skill.CuratedVersionID.Valid || !latestVersionID.Valid {
		return TierIndexed
	}
	if skill.CuratedVersionID.Bytes != latestVersionID.Bytes {
		return TierIndexed
	}
	return TierCurated
}

func statusLabel(s LicenseStatus) labelled {
	d := s.Display()
	return labelled{Value: string(s), Label: d.Label, Note: d.Note}
}

func redistributionLabel(v string) labelled {
	d := Redistribution(v).Display()
	return labelled{Value: v, Label: d.Label, Note: d.Note}
}

func trustLabel(t SourceTrust) labelled {
	d := t.Display()
	return labelled{Value: string(t), Label: d.Label, Note: d.Note}
}

func derivation(s SkillFacts) derivationInfo {
	isFork := s.ForkedFromSkillID.Valid
	b := Derivation(isFork)
	out := derivationInfo{IsFork: isFork, Label: b.Label, Note: b.Note}
	if isFork {
		out.ForkedFromSkillID = pgconv.UUIDString(s.ForkedFromSkillID)
		out.ForkedFromVersionID = pgconv.UUIDString(s.ForkedFromVersionID)
	}
	return out
}

func enrichmentFrom(e gen.GetSkillEnrichmentRow) enrichmentInfo {
	out := enrichmentInfo{Status: e.EnrichmentStatus, Note: enrichPendingNote}
	if e.EnrichmentStatus == "enriched" {
		out.Note = enrichedNote
	}
	out.Summary = e.EnrichedSummary
	out.TaskExamples = nonEmptyLines(e.TaskExamples)

	var tags llmclient.SkillTags
	if len(e.Tags) > 0 && json.Unmarshal(e.Tags, &tags) == nil && tags.Inputs != nil {
		out.Tags = &tags
	}
	if e.EnrichmentModel != nil {
		out.Model = *e.EnrichmentModel
	}
	if e.EnrichmentPromptVersion != nil {
		out.PromptVersion = *e.EnrichmentPromptVersion
	}
	return out
}

func modelLimitations(stored string) []limitation {
	lines := nonEmptyLines(stored)
	out := make([]limitation, 0, len(lines))
	for _, l := range lines {
		out = append(out, limitation{Text: l, Source: limitSourceModel})
	}
	return out
}

func scanDerivedLimitations(r skillpkg.Report) []limitation {
	var out []limitation
	seen := make(map[string]bool)
	for _, f := range r.Findings {
		text, ok := scanLimitations[f.Code]
		if !ok || seen[f.Code] {
			continue
		}
		seen[f.Code] = true
		out = append(out, limitation{Text: text, Source: limitSourceScan})
	}
	return out
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func licenseFrom(v VersionFacts) licenseInfo {
	if v.LicenseExpression == nil || *v.LicenseExpression == "" {
		return licenseInfo{Status: statusLabel(LicenseStatusUnknown)}
	}

	out := licenseInfo{Expression: *v.LicenseExpression, Status: statusLabel(LicenseStatusDeclared)}
	if v.LicenseSource != nil {
		out.Source = *v.LicenseSource
		out.SourceNote = licenseSourceNotes[*v.LicenseSource]
	}
	return out
}

func sourceFrom(s SourceFacts) *sourceInfo {
	out := &sourceInfo{
		Type:             s.SourceType,
		ContentHash:      s.ContentHash,
		FetchedAt:        timeString(s.FetchedAt),
		LastCheckedAt:    timeString(s.LastCheckedAt),
		UnavailableSince: timeString(s.UnavailableSince),
	}
	if s.SourceURL != nil {
		out.URL = *s.SourceURL
	}
	if s.SourceRef != nil {
		out.SourceVersion = *s.SourceRef
	}
	if s.TaskDescription != nil {
		out.TaskDescription = *s.TaskDescription
	}
	if s.GeneratorModel != nil {
		out.GeneratorModel = *s.GeneratorModel
	}
	if s.GeneratorPromptVersion != nil {
		out.GeneratorPromptVersion = *s.GeneratorPromptVersion
	}
	if len(s.GenerationInputs) > 0 && string(s.GenerationInputs) != "null" {
		out.GenerationInputs = json.RawMessage(s.GenerationInputs)
	}
	trust := SourceTrustUnknown
	switch {
	case s.SourceType == "git" && out.URL != "":
		trust = SourceTrustTraceable
	case s.SourceType == "generated":

		trust = SourceTrustGenerated
	}
	out.Trust = trustLabel(trust)
	return out
}

func summarizeRisk(r skillpkg.Report) riskSummary {
	out := riskSummary{
		ScanStatus: "scanned",
		Highlights: []skillpkg.Finding{},
		InfoCounts: map[string]int{},
		Note:       riskNote,
	}
	codes := map[string]bool{}
	for _, f := range r.Findings {
		switch f.Severity {
		case skillpkg.SeverityError:
			out.Counts.Errors++
			out.Highlights = append(out.Highlights, f)
		case skillpkg.SeverityWarning:
			out.Counts.Warnings++
			out.Highlights = append(out.Highlights, f)
		default:
			out.Counts.Infos++
			out.InfoCounts[f.Code]++
		}
		codes[f.Code] = true
	}
	out.Disclosures = disclosuresFor(codes)
	return out
}

func specValidation(r skillpkg.Report) string {
	if r.Blocked {
		return "failed"
	}
	return "passed"
}

func fileTree(fsys fs.FS) []fileEntry {
	out := []fileEntry{}
	_ = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // unreadable entries are skipped, not fatal
		}
		e := fileEntry{Path: path, IsScript: skillpkg.IsScriptPath(path)}
		if info, err := d.Info(); err == nil {
			e.Size = info.Size()
		}
		out = append(out, e)
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func timeString(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format("2006-01-02T15:04:05Z")
}
