package analytics

// POST /feedback — the closed beta's qualitative channel (BETA-003/004/005).
//
// Three entry points, one endpoint, split by `kind` and not by URL: the visitor
// who is not on the invite list and the one whose run allowance ran out are asking
// the same question ("what did you want that you could not have"), and PDM-010
// §8.1 designed those two to share a form; being stuck somewhere in the journey is
// a different question and says so in the field.
//
// Not the same channel as PUT /runs/{id}/evaluation/feedback, which answers "was
// this one judgement useful". Merging them would produce a single bucket that
// answers neither (beta-design §5 lists the three channels and says not to merge).
//
// It lives in this package rather than beside the evaluation feedback because the
// two things it is actually coupled to are here: BETA-004's "blocking" is decided
// by lining a report up against the funnel, and that only works if both carry the
// same session identifier.

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/identity"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/httpx"
)

// maxFeedbackMessage matches the maxLength public.yaml declares and the CHECK in
// 0029, so an oversized body is refused the same way wherever it arrives.
const maxFeedbackMessage = 2000

// Handler serves POST /feedback. Session scoped like everything else that writes
// workspace data (iron rule 3).
type Handler struct {
	Svc      *Service
	Identity *identity.Service
}

// DownloadStartedOn wraps the download-content route and records funnel segment
// 6's first half before it runs (02:O11Y-004).
//
// A wrapper in the route table rather than a line inside the packaging handler,
// for the reason that file exists: the middleware on each route is meant to be one
// reviewable list. It also keeps the two halves of "download" visibly separate —
// this records the attempt, and whether bytes actually went out is answered by
// download_records, which the handler writes.
//
// Mount inside RequireSession: the workspace comes from the session, never from
// the request (iron rule 3).
func (h *Handler) DownloadStartedOn(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.Svc.Enabled() {
			var artifactID pgtype.UUID
			if artifactID.Scan(r.PathValue("artifactId")) == nil {
				var workspace pgtype.UUID
				if user, ok := identity.SessionUser(r.Context()); ok {
					if ws, err := h.Identity.PersonalWorkspace(r.Context(), user); err == nil {
						workspace = ws.ID
					}
				}
				// No target: the artifact carries it, and reading the row here to
				// copy one field would be a second query on the download path for a
				// dimension the funnel does not ask about.
				h.Svc.DownloadStarted(r.Context(), workspace, artifactID, "")
			}
		}
		next(w, r)
	}
}

// Feedback handles POST /feedback (BETA-003/004/005).
//
// 204 with no body and no id: there is no read surface for feedback, and handing
// back an identifier would imply one exists.
func (h *Handler) Feedback(w http.ResponseWriter, r *http.Request) {
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
		Kind     string `json:"kind"`
		Message  string `json:"message"`
		PagePath string `json:"page_path"`
		RunID    string `json:"run_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with kind and message")
		return
	}
	if body.Kind != "blocking_issue" && body.Kind != "need_signal" {
		httpx.WriteError(w, http.StatusBadRequest, "kind must be blocking_issue or need_signal")
		return
	}
	message := strings.TrimSpace(body.Message)
	if message == "" || len([]rune(message)) > maxFeedbackMessage {
		httpx.WriteError(w, http.StatusBadRequest, "message must not be blank and must be at most 2000 characters")
		return
	}

	p := gen.InsertFeedbackReportParams{
		WorkspaceID: ws.ID,
		UserID:      user.ID,
		Kind:        body.Kind,
		Message:     message,
	}
	// A route path, never a full URL: a query string can carry personal data, and
	// this channel is not where it belongs (beta-design §4.2 界線 2). Anything that
	// is not a path is dropped rather than refused — losing the report over a
	// context field would be the worse outcome.
	if path := strings.TrimSpace(body.PagePath); strings.HasPrefix(path, "/") &&
		!strings.ContainsAny(path, "?#") && len(path) <= 512 {
		p.PagePath = &path
	}
	// Same rule for the run: it has to be the caller's own, and one that is not is
	// dropped, not rejected. Existence stays private either way (WS-006) — a
	// caller cannot learn anything about somebody else's run from a 204.
	var runID pgtype.UUID
	if body.RunID != "" && runID.Scan(body.RunID) == nil {
		mine, err := gen.New(h.Svc.Pool).RunInWorkspace(r.Context(), gen.RunInWorkspaceParams{
			ID: runID, WorkspaceID: ws.ID,
		})
		if err == nil && mine {
			p.RunID = runID
		}
	}

	if err := gen.New(h.Svc.Pool).InsertFeedbackReport(r.Context(), p); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "feedback not recorded")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
