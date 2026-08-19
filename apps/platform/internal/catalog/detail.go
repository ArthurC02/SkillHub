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
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/analytics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/identity"
	"github.com/ArthurC02/skillhub/apps/platform/internal/ingest"
	"github.com/ArthurC02/skillhub/apps/platform/internal/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skillpkg"
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
	Type          string `json:"type"` // git | upload
	URL           string `json:"url,omitempty"`
	SourceVersion string `json:"source_version,omitempty"` // commit sha, tag, or branch
	FetchedAt     string `json:"fetched_at,omitempty"`
	ContentHash   string `json:"content_hash,omitempty"`
	// LastCheckedAt and UnavailableSince are the upstream-availability probe's
	// two separate facts (0013_governance): when the source was last looked at,
	// and since when it has been failing. They are reported apart because
	// "checked a minute ago, still there" and "checked a minute ago, gone for two
	// weeks" are different provenance stories (DISC-003/008). Absent means never
	// probed / currently available — never rendered as a reassurance.
	LastCheckedAt    string   `json:"last_checked_at,omitempty"`
	UnavailableSince string   `json:"unavailable_since,omitempty"`
	Trust            labelled `json:"trust"`
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

// compatibility keeps the three DISC-008 axes apart. An axis with no answer says
// "unverified" rather than being omitted: a missing field reads as "fine", an
// explicit 未驗證 does not.
//
// Capability and Runtime come from a measurement (0022), and RuntimeImage says
// which runtime image it was measured on. That pairing is not decoration: the
// same package on the same version gets a different Runtime answer on an image
// that provides python3 than on one that does not, so a verdict without its
// image is a claim nobody made. Empty RuntimeImage means unverified — nothing
// was measured, so there is no image to name.
type compatibility struct {
	SpecValidation string `json:"spec_validation"` // passed | failed | unverified
	Capability     string `json:"capability"`      // activated | not_activated | unverified
	Runtime        string `json:"runtime"`         // native | transpiled | failed | unverified
	RuntimeImage   string `json:"runtime_image,omitempty"`
	MeasuredAt     string `json:"measured_at,omitempty"`
	Note           string `json:"note"`
}

// unverifiedCompat is the pre-measurement state of the two sandbox axes, and the
// zero value every caller starts from.
func unverifiedCompat() compatibility {
	return compatibility{
		SpecValidation: "unverified",
		Capability:     "unverified",
		Runtime:        "unverified",
		Note:           compatUnverifiedNote,
	}
}

// enrichmentInfo labels the model-written fields as model-written (ADR-013).
//
// Tags keeps the buckets the enrichment produced. DISC-003 一般模式 asks for
// 輸入、輸出、依賴 as separate answers and a flat list cannot say which token
// was which — nothing recovers that once it has been joined.
type enrichmentInfo struct {
	Status        string               `json:"status"` // pending | enriched
	Summary       string               `json:"summary,omitempty"`
	TaskExamples  []string             `json:"task_examples,omitempty"`
	Tags          *llmclient.SkillTags `json:"tags,omitempty"`
	Model         string               `json:"model,omitempty"`
	PromptVersion string               `json:"prompt_version,omitempty"`
	Note          string               `json:"note"`
}

// limitation is one entry of the DISC-003 一般模式「限制」 block, with where it
// came from. The two sources answer different questions — what the author's own
// document says the skill cannot do, and what the package's contents imply it
// needs — so they are labelled rather than merged into one anonymous sentence
// (ADR-013 also requires the model-written half to be marked as such).
type limitation struct {
	Text   string `json:"text"`
	Source string `json:"source"` // model | scan
}

const (
	limitSourceModel = "model"
	limitSourceScan  = "scan"
)

// scanLimitations are the limitations derivable from the static scan: what the
// package's own contents say it will need in order to work. Keyed by finding
// code, so a code the scan stops emitting silently stops producing a line
// rather than producing a stale one.
//
// Only findings that describe a *requirement* belong here. A warning about an
// unknown license is a licensing fact, not a limitation on using the skill, and
// it already has its own field.
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

// accessRestriction is the 0023 licensing hold, rendered so a reader is told
// what is missing and why rather than being shown a detail page that quietly has
// no "view files" on it. Absent for everything not on hold, which is nearly
// everything — hence the pointer.
type accessRestriction struct {
	Reason string `json:"reason"` // reason code, e.g. license-review
	Note   string `json:"note"`
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
	Scope      string         `json:"scope"`
	Tier       labelled       `json:"tier"`
	Enrichment enrichmentInfo `json:"enrichment"`
	// Limitations is DISC-003 一般模式「限制」, from both sources, each labelled.
	// Always present, empty when neither source stated one — which is not a
	// claim that the skill is unconstrained.
	Limitations []limitation `json:"limitations"`
	Version     *versionInfo `json:"version,omitempty"`
	Source      *sourceInfo  `json:"source,omitempty"`
	License     licenseInfo  `json:"license"`
	// Redistribution is the ADR-027 決策 4 verdict on whether this content may be
	// handed on at all, which is what decides whether a download package can be
	// built from it. Always present, because every skill has an answer — a skill
	// nobody classified is `unknown`, and an absent field would be a fourth state
	// the packaging gate does not have. A separate axis from License and from
	// Restriction below, and derived from neither.
	Redistribution labelled       `json:"redistribution"`
	Derivation     derivationInfo `json:"derivation"`
	AllowedTools   []string       `json:"allowed_tools,omitempty"`
	Risk           riskSummary    `json:"risk"`
	Compat         compatibility  `json:"compatibility"`
	// Restriction is present only while a licensing question about the package
	// is open (0023). Everything above it still answers: the hold is on the
	// materials, not on the platform's own description of them.
	Restriction *accessRestriction `json:"access_restriction,omitempty"`
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
		SkillID:     uuidString(skill.ID),
		Name:        skill.Name,
		Scope:       scope,
		Tier:        tierLabel(),
		Limitations: []limitation{},
		Derivation:  derivation(skill),
		License:     licenseInfo{Status: statusLabel(LicenseStatusUnknown)},
		// Read off the skill row, not off the license: 02:CONTENT-002 forbids
		// deriving one from the other, and the packaging gate reads this same
		// column.
		Redistribution: redistributionLabel(skill.Redistribution),
		Risk:           riskSummary{ScanStatus: "unavailable", InfoCounts: map[string]int{}, Highlights: []skillpkg.Finding{}, Note: riskNote},
		Compat:         unverifiedCompat(),
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

	// DISC-002「Agent 相容」. Absent is the normal state for anything nobody has
	// run yet, and it leaves the two axes on unverified rather than failing the
	// request — a skill with no measurement is still a skill worth showing.
	if c, err := q.GetSkillRuntimeCompatibility(ctx, ver.ID); err == nil {
		out.Compat.Capability = c.Capability
		out.Compat.Runtime = c.Runtime
		out.Compat.RuntimeImage = c.RuntimeImage
		out.Compat.MeasuredAt = timeString(c.MeasuredAt)
		out.Compat.Note = compatMeasuredNote
	} else if !errors.Is(err, pgx.ErrNoRows) {
		httpx.WriteError(w, http.StatusInternalServerError, "skill detail failed")
		return
	}

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
		out.Limitations = append(out.Limitations, scanDerivedLimitations(report)...)
		if report.Manifest != nil {
			out.AllowedTools = report.Manifest.AllowedTools
		}
	}
	h.recordDetailView(r, skill.ID)
	httpx.WriteJSON(w, http.StatusOK, out)
}

// recordDetailView is funnel segment 1's second half (02:O11Y-004): somebody who
// submitted an intent actually opened a detail page.
//
// Called only where the page is really served, not after resolveSkill, because a
// view that ended in a 500 is not a segment anybody completed.
//
// The workspace is the *viewer's*, resolved from the session, and not the skill
// owner's — the catalogue's entries all belong to the operator's workspace, so
// recording that would label every event with the same id and answer nothing. It
// is absent for anonymous visitors, which is most of this segment (DISC-010), and
// session_id is what stitches those to whatever they do after signing in.
func (h *Handler) recordDetailView(r *http.Request, skillID pgtype.UUID) {
	if !h.Analytics.Enabled() {
		return // no lookup, and above all no extra query, when nothing is collected
	}
	var workspace pgtype.UUID
	if user, ok := identity.SessionUser(r.Context()); ok {
		if ws, err := h.Identity.PersonalWorkspace(r.Context(), user); err == nil {
			workspace = ws.ID
		}
	}
	arrival, rank := analytics.ArrivalFromRequest(r)
	h.Analytics.SkillDetailViewed(r.Context(), workspace, skillID, arrival, rank)
}

// SkillFiles handles GET /api/skills/{id}/files (DISC-007): the SKILL.md text
// and the package file tree, with scripts marked. Same scope rules as
// SkillDetail.
func (h *Handler) SkillFiles(w http.ResponseWriter, r *http.Request) {
	skill, _, ok := h.resolveSkill(w, r)
	if !ok {
		return
	}
	// 0023: this endpoint is the one that reproduces the package's own bytes —
	// SKILL.md verbatim and the file tree — so it is the one a licensing hold
	// closes. 403 and not 404: search still lists the skill and the detail page
	// still describes it, so "no such thing" would be a lie the rest of the API
	// immediately contradicts. The reason travels with the refusal.
	if rest := restrictionOf(skill); rest != nil {
		httpx.WriteError(w, http.StatusForbidden, rest.Note)
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

// restrictionOf turns the stored reason code into the block the API returns.
// One place, because the detail view, the files view and the run gate must not
// be able to disagree about whether a skill is on hold.
func restrictionOf(s gen.Skill) *accessRestriction {
	if s.AccessRestriction == nil || strings.TrimSpace(*s.AccessRestriction) == "" {
		return nil
	}
	note, ok := restrictionNotes[*s.AccessRestriction]
	if !ok {
		// An unknown code still restricts. Failing open on a reason nobody
		// recognises would make a typo in a review the way to unlock content.
		note = restrictionNoteDefault
	}
	return &accessRestriction{Reason: *s.AccessRestriction, Note: note}
}

// restrictionNotes is the user-facing text per reason code. Kept in Go rather
// than in the row: the row records the decision, and the decision does not
// change when the wording does.
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
	compatUnverifiedNote = "規格驗證只檢查套件格式。能力相容與實測相容需要 Sandbox 實測才有結果," +
		"未實測一律標示為未驗證,不代表相容。"
	// The measured note names the image because the verdict is only about that
	// image. 「模型轉譯」 is spelled out rather than left as a value name: it is
	// the case a reader will not guess, and it is the one that decides whether
	// the Skill's own script is what actually ran.
	compatMeasuredNote = "能力相容與實測相容為 Sandbox 實測結果,只對下方 runtime_image 成立," +
		"換一個執行映像需要重新實測。實測相容為「模型轉譯」時,執行的是模型對套件腳本的改寫,不是腳本本身。"
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

// redistributionLabel echoes the stored value and pairs it with the copy for it.
// The value is passed through rather than normalised: a client's own gate keys
// off it, and rewriting an unrecognised value to `unknown` would hide from the
// reader that the row holds something nobody planned for.
func redistributionLabel(v string) labelled {
	d := Redistribution(v).Display()
	return labelled{Value: v, Label: d.Label, Note: d.Note}
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
	// Unparseable or absent tags are reported as absent, not as empty buckets:
	// "{}" would say the enrichment looked and found no inputs.
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

// modelLimitations is the enrichment's half of the 限制 block: what the
// package's own documentation states about its limits, as the model read it.
func modelLimitations(stored string) []limitation {
	lines := nonEmptyLines(stored)
	out := make([]limitation, 0, len(lines))
	for _, l := range lines {
		out = append(out, limitation{Text: l, Source: limitSourceModel})
	}
	return out
}

// scanDerivedLimitations is the scan's half: requirements the package's own
// contents imply, regardless of what the document claims. Order follows
// scanLimitations' iteration over findings, so it is stable per package.
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
	out := &sourceInfo{
		Type:             s.SourceType,
		ContentHash:      s.ContentHash,
		FetchedAt:        timeString(s.FetchedAt),
		LastCheckedAt:    timeString(s.LastCheckedAt),
		UnavailableSince: timeString(s.UnavailableSince),
	}
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
