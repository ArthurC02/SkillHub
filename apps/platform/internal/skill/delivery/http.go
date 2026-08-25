package packaging

// The packaging surface of contracts/openapi/public.yaml:
//
//	GET  /packaging/targets                                     what can be built
//	GET  /skills/{id}/versions/{versionId}/packaging/preview     what it would produce
//	POST /skills/{id}/versions/{versionId}/packaging             produce it
//
// All three take their workspace from the session and never from the request
// (iron rule 3), and answer 404 rather than 403 for content that is not the
// caller's — existence is private (WS-006).
//
// The preview and the create call answer from one function (Plan), so a preview
// that says yes and a create that refuses cannot both happen. Two sets of
// criteria drift, and the drift always favours the step the user most wants to
// succeed — the same reasoning that put EVAL-002's preview and apply behind one
// check().
//
// The download routes are deliberately absent from this file: serving bytes,
// download records and the download audit event are their own batch.

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

type Handler struct {
	Svc      *Service
	Identity *identity.Service
}

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

// configured refuses every packaging route on a deployment that has no target
// configuration. 503 and not an empty list: "this deployment cannot package" and
// "there is nothing to package for" are different answers, and only the first
// one is true here.
func (h *Handler) configured(w http.ResponseWriter) bool {
	if h.Svc == nil || len(h.Svc.Profiles) == 0 {
		httpx.WriteError(w, http.StatusServiceUnavailable,
			"no packaging targets are configured on this deployment")
		return false
	}
	return true
}

// targetView is public.yaml's PackagingTarget.
//
// The two verification fields are served rather than left to INSTALL.md alone:
// 02:PACK-002 第 3 條 asks for at least one post-install check, and a check that
// only exists inside a package the user has not built yet is not on the page
// where they are choosing a target. The profile schema guarantees at least one
// of the two per profile; on the wire both stay optional, because the standard
// package names no agent and therefore has no prompt to run against one.
//
// EnvVars travels for the same reason and it is the other half of 02:PACK-002
// 第 1 條. It is a property of the target, not of the Skill: `ANTHROPIC_API_KEY`
// is what the Agent Skills SDK needs, whichever Skill is inside. The dependency
// half of that clause belongs to the package rather than the target, so it is on
// the preview below.
type targetView struct {
	ID                 string       `json:"id"`
	Kind               string       `json:"kind"`
	Version            string       `json:"version"`
	DisplayName        string       `json:"display_name"`
	InstallLocation    string       `json:"install_location,omitempty"`
	SupportStatus      string       `json:"support_status"`
	VerificationPrompt string       `json:"verification_prompt,omitempty"`
	VerificationSteps  []string     `json:"verification_steps,omitempty"`
	EnvVars            []envVarView `json:"env_vars"`
	Notes              []string     `json:"notes"`
}

// envVarView is one row of the profile's env_vars. `Example` is a placeholder the
// profile schema refuses to let carry a credential pattern, and it is the same
// string INSTALL.md renders — one reviewed text, two surfaces (iron rule 11).
type envVarView struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
	Example     string `json:"example,omitempty"`
}

func envVarViews(in []EnvVar) []envVarView {
	out := make([]envVarView, 0, len(in))
	for _, v := range in {
		out = append(out, envVarView(v))
	}
	return out
}

// Targets handles GET /packaging/targets.
//
// Served from the deployment's profile configuration, not from a constant here:
// support status changes when a target is measured and a compiled-in copy would
// be a second truth nobody re-measures (contract-deltas §1.1). A deployment with
// no configuration therefore answers 503 and lists nothing — an honest "this
// deployment cannot package" rather than an invented target list.
func (h *Handler) Targets(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.workspace(w, r); !ok {
		return
	}
	if !h.configured(w) {
		return
	}
	profiles := h.Svc.Profiles.Ordered()
	out := make([]targetView, 0, len(profiles))
	for _, p := range profiles {
		notes := append([]string{}, p.KnownLimitations...)
		notes = append(notes, p.Notes...)
		out = append(out, targetView{
			ID: p.ID, Kind: p.Kind, Version: p.Version, DisplayName: p.DisplayName,
			InstallLocation:    installLocationLine(p),
			SupportStatus:      p.SupportStatus,
			VerificationPrompt: p.VerificationPrompt,
			VerificationSteps:  p.VerificationSteps,
			EnvVars:            envVarViews(p.EnvVars),
			Notes:              notes,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, struct {
		Targets []targetView `json:"targets"`
	}{out})
}

// installLocationLine states where the package goes, in the words the install
// instructions use. Empty for the standard package, which makes no claim about
// any Agent's layout. The scope is part of the sentence because the two profiles
// resolve to the same path and differ only in which layer it is — without that
// word they read as one option listed twice (PDM-008 v4).
func installLocationLine(p Profile) string {
	line := ""
	for i, loc := range p.Install.Locations {
		if i > 0 {
			line += "; "
		}
		line += loc.Path + " (" + loc.Scope + ")"
	}
	return line
}

// previewView is public.yaml's PackagingPreview.
//
// Dependencies is the package's half of 02:PACK-002 第 1 條, and it is here
// rather than on the target because it is a property of these bytes: the same
// lines INSTALL.md will carry, taken from the same findings, so the download page
// and the packaged document cannot say different things about what the Skill
// needs.
type previewView struct {
	Target            string             `json:"target"`
	Allowed           bool               `json:"allowed"`
	BlockedReason     string             `json:"blocked_reason,omitempty"`
	BlockedMessage    string             `json:"blocked_message,omitempty"`
	Validation        validationView     `json:"validation"`
	Dependencies      []string           `json:"dependencies"`
	IncludedTestCases []testCaseView     `json:"included_test_cases"`
	ExcludedTestCases []excludedCaseView `json:"excluded_test_cases"`
	// Served on the preview, not only in the manifest, because the manifest is
	// inside the thing the user has not decided to download yet. An answer that
	// only exists after the decision is not an answer to that decision.
	ExcludedFiles []ExcludedFile `json:"excluded_files"`
	// RetentionDays is how long the package would be kept, and it is on the
	// PREVIEW rather than on the create response for exactly the reason above:
	// 03:PACK-011 asks for it 「打包之前」, and the create response arrives after the
	// decision it is supposed to inform. 02:NFR-001 是同一條理由——會影響你的上限要在
	// 撞到之前看得見, which is what put `redistribution` on the owner's list too.
	//
	// It is not on GET /packaging/targets, the other pre-decision surface,
	// because retention is one deployment number and not a property of a target:
	// three targets would carry three copies of it on the wire.
	//
	// Always present and always positive — Plan refuses the whole preview when the
	// deployment has no ratified period, so this never has to encode 「不知道」.
	// That refusal is the disclosure's fail-closed half: no ratified number, no
	// number on the screen, and no build either.
	RetentionDays int `json:"retention_days"`
}

// retentionDays is the served period in whole days, truncated.
//
// Truncated and not rounded: the number is a promise about how long the bytes
// survive, so the error has to fall on the side of promising less. Anything under
// a day answers 0 and the page says 「不到 1 天」 rather than a rounded-up 1 —
// a deployment configured that short is already violating 02:NFR-002a 第 1 條
// (下載產物的保存期限 ≥ 當期觀察窗), and that should read as wrong rather than be
// smoothed into a plausible-looking day.
func retentionDays(d time.Duration) int { return int(d / (24 * time.Hour)) }

// validationView and findingView are public.yaml's PackageValidation and
// Finding: the manifest's own validation block with each finding's severity
// stamped back on.
//
// The severity is not added to ManifestFinding, because the manifest and the
// API answer to two different contracts and both are right. Inside a package
// the list a finding sits in IS its severity, and download-manifest.schema.json
// is closed around that (`additionalProperties: false`), so a severity field
// there would be a contract break. On the wire the shared `Finding` is what
// every other endpoint returns and its `severity` is required, so a client that
// renders findings from one place must get it. One conversion at the boundary
// is the whole of the difference.
type validationView struct {
	Blocked  bool          `json:"blocked"`
	Errors   []findingView `json:"errors"`
	Warnings []findingView `json:"warnings"`
	Infos    []findingView `json:"infos"`
}

type findingView struct {
	Severity string   `json:"severity"`
	Code     string   `json:"code"`
	Path     string   `json:"path,omitempty"`
	Message  string   `json:"message"`
	Details  []string `json:"details,omitempty"`
}

func validationOf(v ManifestValidation) validationView {
	return validationView{
		Blocked:  v.Blocked,
		Errors:   findingViews(v.Errors, skillpkg.SeverityError),
		Warnings: findingViews(v.Warnings, skillpkg.SeverityWarning),
		Infos:    findingViews(v.Infos, skillpkg.SeverityInfo),
	}
}

func findingViews(in []ManifestFinding, severity skillpkg.Severity) []findingView {
	out := make([]findingView, 0, len(in))
	for _, f := range in {
		out = append(out, findingView{
			Severity: string(severity), Code: f.Code, Path: f.Path,
			Message: f.Message, Details: f.Details,
		})
	}
	return out
}

// testCaseView and excludedCaseView are the manifest's two test-case shapes plus
// the id. The id is here and NOT in the manifest on purpose: inside a package it
// would be a platform identifier travelling to somebody who cannot use it, while
// on this endpoint it is how the UI links the row back to the Test Case.
type testCaseView struct {
	TestCaseID string `json:"test_case_id"`
	Slug       string `json:"slug"`
	Name       string `json:"name"`
}

type excludedCaseView struct {
	TestCaseID string `json:"test_case_id"`
	Name       string `json:"name"`
	Reason     string `json:"reason"`
}

// Preview handles GET .../packaging/preview. Nothing is written and no object is
// created; a version that cannot be packaged still answers 200 with
// `allowed: false` and the reason, because being told why is the point.
func (h *Handler) Preview(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok || !h.configured(w) {
		return
	}
	skillID, versionID, ok := pathIDs(w, r)
	if !ok {
		return
	}
	target := r.URL.Query().Get("target")
	if target == "" {
		httpx.WriteError(w, http.StatusBadRequest, "`target` is required")
		return
	}
	include := r.URL.Query().Get("include_test_cases") == "true"

	p, err := h.Svc.Plan(r.Context(), ws, skillID, versionID, target, include)
	if !h.writeServiceError(w, err) {
		return
	}
	httpx.WriteJSON(w, http.StatusOK, previewView{
		Target: target, Allowed: p.Allowed,
		BlockedReason: p.BlockedReason, BlockedMessage: p.BlockedMessage,
		Validation:        validationOf(p.Validation),
		Dependencies:      p.Dependencies,
		IncludedTestCases: includedViews(p.Included),
		ExcludedTestCases: excludedViews(p.Excluded),
		ExcludedFiles:     p.ExcludedFiles,
		RetentionDays:     retentionDays(p.Retention),
	})
}

// Create handles POST .../packaging.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	skillID, versionID, ok := pathIDs(w, r)
	if !ok {
		return
	}
	if !h.configured(w) {
		return
	}
	var body struct {
		Target           string `json:"target"`
		IncludeTestCases bool   `json:"include_test_cases"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with a `target`")
		return
	}
	if body.Target == "" {
		httpx.WriteError(w, http.StatusBadRequest, "`target` is required")
		return
	}

	res, err := h.Svc.Create(r.Context(), ws, skillID, versionID, body.Target, body.IncludeTestCases)
	if !h.writeServiceError(w, err) {
		return
	}
	if res.Plan != nil && !res.Plan.Allowed {
		// 422 and not 403: the skill is readable and the caller is allowed to
		// ask — it is the content that may not be handed out. `validation`
		// travels only when the reason is about it, carrying the same findings an
		// import refusal does.
		out := map[string]any{
			"error":          res.Plan.BlockedMessage,
			"blocked_reason": res.Plan.BlockedReason,
		}
		if res.Plan.BlockedReason == BlockedValidation {
			out["validation"] = validationOf(res.Plan.Validation)
		}
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, out)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, struct {
		Artifact
		Duplicate bool `json:"duplicate"`
	}{res.Artifact, res.Duplicate})
}

// writeServiceError maps the service's sentinel errors to the contract's status
// codes and reports whether the caller may carry on. The three licensing/target
// distinctions matter: a target the contract does not name is the client's
// mistake, a target this deployment has not configured is the deployment's, and
// a missing object store is neither and was never a verdict about the content.
func (h *Handler) writeServiceError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "skill version not found")
	case errors.Is(err, ErrUnknownTarget):
		httpx.WriteError(w, http.StatusBadRequest,
			"`target` must be one of standard, claude-code, claude-agent-sdk")
	case errors.Is(err, ErrNoProfile), errors.Is(err, ErrNoStore), errors.Is(err, ErrRetentionNotConfigured):
		httpx.WriteError(w, http.StatusServiceUnavailable, err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "packaging failed")
	}
	return false
}

func includedViews(in []IncludedTestCase) []testCaseView {
	out := make([]testCaseView, 0, len(in))
	for _, tc := range in {
		out = append(out, testCaseView(tc))
	}
	return out
}

func excludedViews(in []ExcludedTestCase) []excludedCaseView {
	out := make([]excludedCaseView, 0, len(in))
	for _, tc := range in {
		out = append(out, excludedCaseView(tc))
	}
	return out
}

func pathIDs(w http.ResponseWriter, r *http.Request) (skillID, versionID pgtype.UUID, ok bool) {
	if err := skillID.Scan(r.PathValue("id")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "skill version not found")
		return skillID, versionID, false
	}
	if err := versionID.Scan(r.PathValue("versionId")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "skill version not found")
		return skillID, versionID, false
	}
	return skillID, versionID, true
}
