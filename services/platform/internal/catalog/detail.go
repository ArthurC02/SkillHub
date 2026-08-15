package catalog

// Public skill detail and package-file views (DISC-006/007/008/010).
//
// One handler pair serves both scopes. The catalog read has its scope baked into
// the SQL (GetCatalogSkill, catalog workspaces only) and needs no session; when
// the id is not in the catalog and a session is present, the same assembly runs
// against the caller's own workspace. Splitting this into a public and a private
// endpoint would mean two places to forget a field, and the difference between
// them is only which row the read is allowed to see. Scope never comes from the
// request (iron rule 3).
//
// Risk, license provenance, and the file tree are recomputed from the stored
// package on read rather than persisted: skillpkg.Validate is the single
// definition of what the platform discloses, and a stored copy would be a second
// one that drifts. Reading a package is pure static analysis — nothing inside is
// executed (iron rule 1).
//
// ponytail: one object-store read plus a full re-scan per detail request. Cache
// the Report by content hash (it is immutable, so the hash is the cache key) if
// detail latency ever shows up against NFR-004.

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/services/platform/internal/identity"
	"github.com/ArthurC02/skillhub/services/platform/internal/ingest"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/httpx"
	"github.com/ArthurC02/skillhub/services/platform/internal/skillpkg"
)

// ObjectStore is the slice of object storage the detail views need.
type ObjectStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
}

// errSkillNotFound is the single answer for "no such skill" and "not visible to
// you" (WS-006): the two must be indistinguishable or the 404 becomes an
// existence oracle for other people's private content.
var errSkillNotFound = errors.New("skill not found")

// maxSkillMDBytes caps the SKILL.md text returned to the browser. The unpacked
// cap on import is 256 MiB, so "it came from a validated package" is not a bound
// on any single file.
// ponytail: flat cap, truncation is reported; paginate only if a real package
// ever needs it.
const maxSkillMDBytes = 1 << 20 // 1 MiB

// --- response shapes -------------------------------------------------------

type labelled struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Note  string `json:"note"`
}

// sourceInfo is the DISC-003 provenance record: what was fetched, from where,
// when, and the hash of what arrived. Fields absent from the record stay absent
// rather than being filled with a plausible value (DISC-004 缺少資料顯示未知).
type sourceInfo struct {
	Type          string   `json:"type"` // git | upload
	URL           string   `json:"url,omitempty"`
	SourceVersion string   `json:"source_version,omitempty"` // commit sha, tag, or branch
	FetchedAt     string   `json:"fetched_at,omitempty"`
	ContentHash   string   `json:"content_hash,omitempty"`
	Trust         labelled `json:"trust"`
}

// licenseInfo carries the ADR-021 two-axis answer: which license the package
// evidences, and how strong that evidence is. The expression alone cannot tell
// "the author declared MIT" from "the monorepo root had an MIT file", and
// DISC-003 forbids presenting the second as the first.
type licenseInfo struct {
	Expression string `json:"expression,omitempty"` // absent = unknown
	// Source is the ADR-021 provenance tier: manifest | package-license-file |
	// repo-license-file. Absent for pre-ADR-021 versions, whose tier was never
	// recorded and must not be invented.
	Source     string   `json:"source,omitempty"`
	SourceNote string   `json:"source_note,omitempty"`
	Status     labelled `json:"status"` // unknown | declared (confirmed needs a reviewer)
}

// licenseSourceNotes says what each provenance tier does and does not claim.
var licenseSourceNotes = map[string]string{
	"manifest":             "作者在 SKILL.md frontmatter 自行宣告。",
	"package-license-file": "套件內附 LICENSE 檔案,涵蓋此套件本身。",
	"repo-license-file":    "來自 repo 根目錄的 LICENSE,涵蓋整個 repo,不必然涵蓋此子目錄的內容。",
}

type severityCounts struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Infos    int `json:"infos"`
}

// riskSummary is the DISC-008 risk and disclosure block. There is deliberately
// no single "safe" verdict (NFR-001): the counts, the individual findings, and
// the flags below are shown side by side so a reader draws their own conclusion.
type riskSummary struct {
	// ScanStatus is scanned | unavailable. Unavailable means the stored package
	// could not be read, which is reported as "unknown", never as "clean"
	// (DISC-004 不得自行推定為通過).
	ScanStatus string         `json:"scan_status"`
	Counts     severityCounts `json:"counts"`
	// Highlights are every error- and warning-level finding, verbatim. Info-level
	// disclosures are counted by code instead: one seed package produced 321 URL
	// findings, and a list nobody reads hides the ones that matter.
	Highlights []skillpkg.Finding `json:"highlights"`
	InfoCounts map[string]int     `json:"info_counts"`

	HasScripts         bool `json:"has_scripts"`
	HasEmbeddedScript  bool `json:"has_embedded_script"`
	HasExternalURLs    bool `json:"has_external_urls"`
	HasPossibleSecrets bool `json:"has_possible_secrets"`
	HasBinaries        bool `json:"has_binaries"`

	Note string `json:"note"`
}

// compatibility keeps the three DISC-008 axes apart. Only the first has an
// answer before M2, and the other two say "unverified" rather than being
// omitted: a missing field reads as "fine", an explicit 未驗證 does not.
type compatibility struct {
	SpecValidation string `json:"spec_validation"` // passed | failed | unverified
	Capability     string `json:"capability"`      // unverified until M2
	Runtime        string `json:"runtime"`         // unverified until M2
	Note           string `json:"note"`
}

// enrichmentInfo labels the model-written fields as model-written (ADR-013).
type enrichmentInfo struct {
	Status        string   `json:"status"` // pending | enriched
	Summary       string   `json:"summary,omitempty"`
	TaskExamples  []string `json:"task_examples,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Model         string   `json:"model,omitempty"`
	PromptVersion string   `json:"prompt_version,omitempty"`
	Note          string   `json:"note"`
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

// skillDetail is the GET /api/skills/{id} body (DISC-006/008).
type skillDetail struct {
	SkillID string `json:"skill_id"`
	Name    string `json:"name"`
	// Summary is the package's own frontmatter description. The model's
	// plain-language rewrite lives under Enrichment and is labelled there, so a
	// reader can always tell which text the author wrote.
	Summary string `json:"summary"`
	// Scope is catalog | private: which of the two reads answered.
	Scope        string         `json:"scope"`
	Tier         labelled       `json:"tier"`
	Enrichment   enrichmentInfo `json:"enrichment"`
	Version      *versionInfo   `json:"version,omitempty"`
	Source       *sourceInfo    `json:"source,omitempty"`
	License      licenseInfo    `json:"license"`
	Derivation   derivationInfo `json:"derivation"`
	AllowedTools []string       `json:"allowed_tools,omitempty"`
	Risk         riskSummary    `json:"risk"`
	Compat       compatibility  `json:"compatibility"`
}

type fileEntry struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	IsScript bool   `json:"is_script"`
}

// skillFiles is the GET /api/skills/{id}/files body (DISC-007).
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

// --- handlers --------------------------------------------------------------

// SkillDetail handles GET /api/skills/{id} (DISC-006/008/010). Mount behind
// OptionalSession: anonymous callers get the catalog, a signed-in caller
// additionally gets their own workspace.
func (h *Handler) SkillDetail(w http.ResponseWriter, r *http.Request) {
	skill, scope, ok := h.resolveSkill(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	q := gen.New(h.Pool)

	out := skillDetail{
		SkillID:    uuidString(skill.ID),
		Name:       skill.Name,
		Scope:      scope,
		Tier:       tierLabel(),
		Derivation: derivation(skill),
		License:    licenseInfo{Status: statusLabel(LicenseStatusUnknown)},
		Risk:       riskSummary{ScanStatus: "unavailable", InfoCounts: map[string]int{}, Highlights: []skillpkg.Finding{}, Note: riskNote},
		Compat:     compatibility{SpecValidation: "unverified", Capability: "unverified", Runtime: "unverified", Note: compatNote},
	}
	if skill.Summary != nil {
		out.Summary = *skill.Summary
	}

	if e, err := q.GetSkillEnrichment(ctx, gen.GetSkillEnrichmentParams{
		SkillID: skill.ID, WorkspaceID: skill.WorkspaceID,
	}); err == nil {
		out.Enrichment = enrichmentFrom(e)
		if out.Summary == "" {
			out.Summary = e.Summary
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		httpx.WriteError(w, http.StatusInternalServerError, "skill detail failed")
		return
	}

	ver, err := q.GetLatestSkillVersion(ctx, gen.GetLatestSkillVersionParams{
		SkillID: skill.ID, WorkspaceID: skill.WorkspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// A skill with no version yet (a fork created ahead of its content) is a
		// real state, not an error. Everything version-derived stays absent.
		httpx.WriteJSON(w, http.StatusOK, out)
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "skill detail failed")
		return
	}
	out.Version = &versionInfo{
		VersionID:     uuidString(ver.ID),
		VersionNumber: ver.VersionNumber,
		ContentHash:   ver.ContentHash,
		CreatedAt:     timeString(ver.CreatedAt),
	}
	out.License = licenseFrom(ver)

	if ver.SourceID.Valid {
		src, err := q.GetSkillSource(ctx, gen.GetSkillSourceParams{ID: ver.SourceID, WorkspaceID: ver.WorkspaceID})
		if err == nil {
			out.Source = sourceFrom(src)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			httpx.WriteError(w, http.StatusInternalServerError, "skill detail failed")
			return
		}
	}

	if report, ok := h.scanPackage(ctx, ver.PackageObjectKey); ok {
		out.Risk = summarizeRisk(report)
		out.Compat.SpecValidation = specValidation(report)
		if report.Manifest != nil {
			out.AllowedTools = report.Manifest.AllowedTools
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// SkillFiles handles GET /api/skills/{id}/files (DISC-007): the SKILL.md text
// and the package file tree, with scripts marked. Same scope rules as
// SkillDetail.
func (h *Handler) SkillFiles(w http.ResponseWriter, r *http.Request) {
	skill, _, ok := h.resolveSkill(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	ver, err := gen.New(h.Pool).GetLatestSkillVersion(ctx, gen.GetLatestSkillVersionParams{
		SkillID: skill.ID, WorkspaceID: skill.WorkspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "skill has no saved version")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "skill files failed")
		return
	}

	data, err := h.storeGet(ctx, ver.PackageObjectKey)
	if err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "stored package is not readable")
		return
	}
	fsys, err := ingest.PackageFS(data)
	if err != nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "stored package is not readable")
		return
	}

	out := skillFiles{
		SkillID:       uuidString(skill.ID),
		VersionID:     uuidString(ver.ID),
		VersionNumber: ver.VersionNumber,
		Tree:          fileTree(fsys),
		Note:          filesNote,
	}
	if md, err := fs.ReadFile(fsys, "SKILL.md"); err == nil {
		if len(md) > maxSkillMDBytes {
			md, out.SkillMDTrunc = md[:maxSkillMDBytes], true
		}
		// Truncation can land mid-rune, and invalid UTF-8 does not survive JSON.
		out.SkillMD = strings.ToValidUTF8(string(md), "")
	}
	// The file tree cannot show code that lives inside SKILL.md, which is exactly
	// how 5 seed packages shipped ~180 lines of Python while reporting no scripts
	// (SKILL-003). Carry the disclosure onto the view where that absence shows.
	for _, f := range skillpkg.Validate(fsys).Findings {
		if f.Code == "embedded-script" {
			msg := f.Message
			out.EmbeddedScript = &msg
			break
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

const (
	riskNote = "以上為靜態掃描結果:匯入與掃描期間不執行套件內任何程式碼。" +
		"通過掃描不等於安全或有效,請自行檢視。"
	compatNote = "規格驗證只檢查套件格式。能力相容與實測相容需要 Sandbox 試跑(M2)," +
		"在此之前一律標示為未驗證,不代表相容。"
	filesNote = "tree 為套件內檔案清單與大小;目前僅回傳 SKILL.md 全文。" +
		"其他單檔內容的讀取端點屬 DISC-007 後續工作項,尚未實作。"
	enrichPendingNote = "尚未產生模型摘要;顯示的是套件自身的 frontmatter description。"
	enrichedNote      = "本區塊由模型產生(非套件作者撰寫),僅供理解用途。"
)

// --- scope resolution ------------------------------------------------------

// resolveSkill reads the skill in the widest scope the caller actually has, and
// writes the error response itself when there is none. Catalog first because it
// is the only scope an anonymous caller has and the one both callers share; the
// caller's own workspace is tried second and only with a session. Neither read
// takes a scope from the request.
func (h *Handler) resolveSkill(w http.ResponseWriter, r *http.Request) (gen.Skill, string, bool) {
	var id pgtype.UUID
	if err := id.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, errSkillNotFound.Error())
		return gen.Skill{}, "", false
	}
	ctx := r.Context()
	q := gen.New(h.Pool)

	skill, err := q.GetCatalogSkill(ctx, id)
	if err == nil {
		// INGEST-010: a taken-down skill was public and its URL is still in
		// circulation, so it answers 410 rather than 404 — the content existed
		// and was withdrawn, which is a different fact from never existing.
		if skill.TakedownAt.Valid {
			httpx.WriteError(w, http.StatusGone, "this skill has been withdrawn from the catalog")
			return gen.Skill{}, "", false
		}
		return skill, "catalog", true
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		httpx.WriteError(w, http.StatusInternalServerError, "skill lookup failed")
		return gen.Skill{}, "", false
	}

	user, ok := identity.SessionUser(ctx)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, errSkillNotFound.Error())
		return gen.Skill{}, "", false
	}
	ws, err := h.Identity.PersonalWorkspace(ctx, user)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "workspace lookup failed")
		return gen.Skill{}, "", false
	}
	skill, err = q.GetSkill(ctx, gen.GetSkillParams{ID: id, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, errSkillNotFound.Error())
		return gen.Skill{}, "", false
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "skill lookup failed")
		return gen.Skill{}, "", false
	}
	return skill, "private", true
}

func (h *Handler) storeGet(ctx context.Context, key string) ([]byte, error) {
	if h.Store == nil {
		return nil, errors.New("catalog: no object store configured")
	}
	return h.Store.Get(ctx, key)
}

// scanPackage re-validates the stored package. A package that cannot be read is
// not a failed request: the detail view still answers, with the risk block
// saying the scan is unavailable rather than implying a clean one.
func (h *Handler) scanPackage(ctx context.Context, key string) (skillpkg.Report, bool) {
	data, err := h.storeGet(ctx, key)
	if err != nil {
		return skillpkg.Report{}, false
	}
	fsys, err := ingest.PackageFS(data)
	if err != nil {
		return skillpkg.Report{}, false
	}
	return skillpkg.Validate(fsys), true
}

// --- assembly --------------------------------------------------------------

// tierLabel is TierIndexed for everything this endpoint can return. Curation is
// a recorded human review (PDM-002 nine-item checklist) and nothing records it
// yet — CONTENT-003's promotion workflow is what would. Membership of a catalog
// workspace is not that review, and labelling it 精選 would be the endorsement
// PDM-002 explicitly warns against.
func tierLabel() labelled {
	d := TierIndexed.Display()
	return labelled{Value: string(TierIndexed), Label: d.Badge, Note: d.TrustIndicator}
}

func statusLabel(s LicenseStatus) labelled {
	d := s.Display()
	return labelled{Value: string(s), Label: d.Label, Note: d.Note}
}

func trustLabel(t SourceTrust) labelled {
	d := t.Display()
	return labelled{Value: string(t), Label: d.Label, Note: d.Note}
}

func derivation(s gen.Skill) derivationInfo {
	isFork := s.ForkedFromSkillID.Valid
	b := Derivation(isFork)
	out := derivationInfo{IsFork: isFork, Label: b.Label, Note: b.Note}
	if isFork {
		out.ForkedFromSkillID = uuidString(s.ForkedFromSkillID)
		out.ForkedFromVersionID = uuidString(s.ForkedFromVersionID)
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
	out.Tags = strings.Fields(e.Tags)
	if e.EnrichmentModel != nil {
		out.Model = *e.EnrichmentModel
	}
	if e.EnrichmentPromptVersion != nil {
		out.PromptVersion = *e.EnrichmentPromptVersion
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

// licenseFrom reads the license off the version row, not off a re-scan: the row
// is what the import decided and what a fork carried forward, and the two axes
// (expression, provenance tier) travel together (ADR-021).
func licenseFrom(v gen.SkillVersion) licenseInfo {
	if v.LicenseExpression == nil || *v.LicenseExpression == "" {
		return licenseInfo{Status: statusLabel(LicenseStatusUnknown)}
	}
	// Declared, never Confirmed: confirmation is a reviewer's act and no column
	// records one. Even a repo-root MIT is only a declaration about the repo.
	out := licenseInfo{Expression: *v.LicenseExpression, Status: statusLabel(LicenseStatusDeclared)}
	if v.LicenseSource != nil {
		out.Source = *v.LicenseSource
		out.SourceNote = licenseSourceNotes[*v.LicenseSource]
	}
	return out
}

// sourceFrom maps the import record to the DISC-003 provenance block. Trust is
// Traceable only for a git import that actually recorded a URL; an upload has no
// verifiable origin, and manual confirmation has no store yet.
func sourceFrom(s gen.SkillSource) *sourceInfo {
	out := &sourceInfo{Type: s.SourceType, ContentHash: s.ContentHash, FetchedAt: timeString(s.FetchedAt)}
	if s.SourceUrl != nil {
		out.URL = *s.SourceUrl
	}
	if s.SourceRef != nil {
		out.SourceVersion = *s.SourceRef
	}
	trust := SourceTrustUnknown
	if s.SourceType == "git" && out.URL != "" {
		trust = SourceTrustTraceable
	}
	out.Trust = trustLabel(trust)
	return out
}

// summarizeRisk turns a validation report into the DISC-008 block: severity
// counts, every error and warning verbatim, and info-level disclosures folded to
// counts per code.
func summarizeRisk(r skillpkg.Report) riskSummary {
	out := riskSummary{
		ScanStatus: "scanned",
		Highlights: []skillpkg.Finding{},
		InfoCounts: map[string]int{},
		Note:       riskNote,
	}
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
		switch f.Code {
		case "script-file":
			out.HasScripts = true
		case "embedded-script":
			out.HasEmbeddedScript = true
		case "external-url":
			out.HasExternalURLs = true
		case "possible-secret":
			out.HasPossibleSecrets = true
		case "binary-file":
			out.HasBinaries = true
		}
	}
	return out
}

// specValidation reports the spec axis only. A passing report means the package
// parses and its references resolve — never that it is safe or effective
// (SKILL-002 規格通過不得自動標示為執行安全).
func specValidation(r skillpkg.Report) string {
	if r.Blocked {
		return "failed"
	}
	return "passed"
}

// fileTree lists the package's files with their sizes, marking scripts with the
// same rule the scan used (DISC-003 Script 必須有明確標示). Directories are left
// out: the paths carry the structure and an empty directory says nothing.
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
