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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

// maxFeedbackMessage matches the maxLength public.yaml declares and the CHECK in
// 0029, so an oversized body is refused the same way wherever it arrives.
const maxFeedbackMessage = 2000

// Handler serves POST /feedback. Session scoped like everything else that writes
// workspace data (iron rule 3).
type Handler struct {
	Svc      *Service
	Identity *identity.Service
	// FeedbackRetention is how long a report lives, for GET
	// /policy/data-retention to disclose. Deployment configuration
	// (FEEDBACK_RETENTION), injected rather than read here, and zero is the
	// honest "nobody has set one" the disclosure prints as such.
	//
	// It is not enforced on this side: `maintenance purge-feedback` does the
	// deleting, and this field only says what that sweep is configured to do.
	// The alternative — the page computing a number of its own — is how a
	// promise and a sweep drift apart, which is the failure PDM-006 6 has
	// already had four times.
	FeedbackRetention time.Duration
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
		BuildID  string `json:"build_id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "內容必須是 JSON，且包含 kind 與 message")
		return
	}
	if body.Kind != "blocking_issue" && body.Kind != "need_signal" {
		httpx.WriteError(w, http.StatusBadRequest, "kind 必須是 blocking_issue 或 need_signal")
		return
	}
	message := strings.TrimSpace(body.Message)
	if message == "" || len([]rune(message)) > maxFeedbackMessage {
		httpx.WriteError(w, http.StatusBadRequest, "message 不能空白，且最多 2000 字")
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
	// The build the page came from (0054, 資訊架構 IA-11): the footer's own
	// identifier, sent by the form. It names the software, not the person, and it
	// is what lets 「這一頁怪怪的」 be reproduced against a rolling deployment.
	// Same drop-not-refuse rule as the path, and the length is the contract's.
	if build := strings.TrimSpace(body.BuildID); build != "" && len(build) <= 64 {
		p.BuildID = &build
	}
	// Same rule for the run: it has to be the caller's own, and one that is not is
	// dropped, not rejected. Existence stays private either way (WS-006) — a
	// caller cannot learn anything about somebody else's run from a 204.
	var runID pgtype.UUID
	if body.RunID != "" && runID.Scan(body.RunID) == nil {
		if h.Svc.RunBelongsToWorkspace == nil {
			httpx.WriteError(w, http.StatusInternalServerError, "feedback run lookup unavailable")
			return
		}
		mine, err := h.Svc.RunBelongsToWorkspace(r.Context(), ws.ID, runID)
		if err == nil && mine {
			p.RunID = runID
		}
	}

	if err := gen.New(h.Svc.Pool).InsertFeedbackReport(r.Context(), p); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "回報沒有記錄成功，可以再送一次")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PurgeExpiredFeedback removes the reports older than `retention` and returns how
// many went. `maintenance purge-feedback` is the only caller; the window is the
// deployment's and is passed in rather than read here, like every other retention
// number in this repository.
//
// It sits next to the handler that writes these rows, deliberately: the write
// side is what makes the deadline necessary, and every previous instance of this
// bug in this repository — audit events, datasets, deleted skills — was a table
// whose writer and whose sweeper lived far enough apart that nobody noticed one
// of them was missing. Here they are eighty lines apart.
//
// Analytics' own retention is not here and no longer exists as a bulk statement:
// analytics_events is dropped a partition at a time (0029 said so in as many
// words), which is `maintenance rotate-partitions`. feedback_reports is not
// partitioned, so it needs this.
func (s *Service) PurgeExpiredFeedback(ctx context.Context, retention time.Duration) (int64, error) {
	if s == nil || s.Pool == nil {
		return 0, errors.New("feedback purge requires a database pool")
	}
	if retention <= 0 {
		return 0, errors.New("feedback purge requires a positive retention period")
	}
	return gen.New(s.Pool).DeleteExpiredFeedbackReports(ctx,
		pgtype.Timestamptz{Time: s.now().Add(-retention), Valid: true})
}
