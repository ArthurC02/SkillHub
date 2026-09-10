package packaging

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

func (h *Handler) configured(w http.ResponseWriter) bool {
	if h.Svc == nil || len(h.Svc.Profiles) == 0 {
		httpx.WriteError(w, http.StatusServiceUnavailable,
			"這個部署沒有設定任何打包目標")
		return false
	}
	return true
}

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

type previewView struct {
	Target            string             `json:"target"`
	Allowed           bool               `json:"allowed"`
	BlockedReason     string             `json:"blocked_reason,omitempty"`
	BlockedMessage    string             `json:"blocked_message,omitempty"`
	Validation        validationView     `json:"validation"`
	Dependencies      []string           `json:"dependencies"`
	IncludedTestCases []testCaseView     `json:"included_test_cases"`
	ExcludedTestCases []excludedCaseView `json:"excluded_test_cases"`

	ExcludedFiles []ExcludedFile `json:"excluded_files"`

	RetentionDays int `json:"retention_days"`
}

func retentionDays(d time.Duration) int { return int(d / (24 * time.Hour)) }

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

type testCaseView struct {
	TestCaseID string `json:"test_case_id"`
	Slug       string `json:"slug"`
	Name       string `json:"name"`
}

type excludedCaseView struct {
	TestCaseID string `json:"test_case_id"`
	Name       string `json:"name"`
	Reason     string `json:"reason"`
	Label      string `json:"label"`
	Note       string `json:"note"`
}

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

func (h *Handler) writeServiceError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "找不到這個 Skill 版本")
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
		httpx.WriteError(w, http.StatusNotFound, "找不到這個 Skill 版本")
		return skillID, versionID, false
	}
	if err := versionID.Scan(r.PathValue("versionId")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "找不到這個 Skill 版本")
		return skillID, versionID, false
	}
	return skillID, versionID, true
}
