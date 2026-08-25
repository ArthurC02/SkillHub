package analytics

// GET /policy/data-retention — the disclosure surface for the one data class a
// user produces without submitting anything (02:O11Y-004, ADR-029, 04 丙-25②).
//
// Same shape and the same reason as GET /test-cases/limits: a policy that only
// exists in a document is a policy the product cannot be held to, so the values
// are served from the constants the writer itself reads. `retention_days` comes
// from Service.Retention (ANALYTICS_RETENTION), not from a number typed here, so
// the page cannot promise a window the deployment does not apply.
//
// `collecting: false` is the honest answer for a deployment that has not set one,
// and it is the shipped default: NFR-002 forbids collection before a retention
// value exists, and ADR-029 決策 5's 180 days is still a proposal. The endpoint
// exists either way — "we collect nothing" is a disclosure, not an absence of one.
//
// Public, no session: a data policy a visitor has to log in to read is not a
// policy they can decide by. It reads nothing user-specific.

import (
	"net/http"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

// DataRetention handles GET /policy/data-retention.
func (h *Handler) DataRetention(w http.ResponseWriter, _ *http.Request) {
	days := 0
	if h.Svc != nil {
		// Rounded up, not truncated. A deployment with ANALYTICS_RETENTION=12h is
		// collecting — Enabled() only asks for a second — and reporting
		// `{"collecting": true, "retention_days": 0}` is two sentences that
		// contradict each other. Production's 8760h never reaches this; a staging
		// window shorter than a day does (M5 audit, 2026-08-25).
		days = int(h.Svc.Retention / (24 * time.Hour))
		if h.Svc.Retention%(24*time.Hour) != 0 {
			days++
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"collecting":     h.Svc.Enabled(),
		"retention_days": days,
		// The closed set of four. Adding a fifth means answering first why no
		// domain table can (ADR-029 決策 1), and the CHECK in 0029 enforces it.
		//
		// Each `attributes` list is what that event adds *on top of* the five
		// fixed columns every row carries (ADR-029 決策 2). It is not the row and
		// it is not the constructor signature — the constructor's first field is
		// the session id, which belongs to all four and is disclosed once in
		// `note` rather than four times here (M5 audit, 2026-08-25).
		"events": []map[string]any{
			{
				"name":         EventSearchPerformed,
				"when":         "a search is submitted",
				"attributes":   []string{"query_length", "query_language", "result_count", "has_results", "filters_applied"},
				"not_recorded": "not one word of the query itself; query_language is a writing-system bucket (han/latin/mixed/other), not a locale",
			},
			{
				"name":         EventSkillDetailViewed,
				"when":         "a skill detail page is opened",
				"attributes":   []string{"skill_id"},
				"not_recorded": "how the page was reached — whether from a search result or a direct link, and the position it held in that result list. Both were columns until 0040 dropped them (04 丙-59); nothing records them now",
			},
			{
				"name":         EventSessionStarted,
				"when":         "a visit begins",
				"attributes":   []string{},
				"not_recorded": "nothing beyond the columns every row carries: the visit is the whole of the event. session_id is an unrelated random cookie value — not the login session token, not a hash of it, and it cannot be resolved back to one",
			},
			// No target attribute: the only caller passes an empty one
			// (feedback.go's route wrapper deliberately does not re-read the
			// artifact row for it) and DownloadStarted writes the column only when
			// it is non-empty, so it has never held a value. Naming it here told
			// the reader we collect more than we do, and this page is the one
			// ADR-029 決策 5 holds to a higher standard than any other.
			{
				"name":         EventDownloadStarted,
				"when":         "a download is requested",
				"attributes":   []string{"artifact_id"},
				"not_recorded": "whether the bytes actually went out; that is download_records, a domain fact, and the split is why this event exists",
			},
		},
		"note": "these four events are the whole of this data class. Every row also " +
			"carries the five columns ADR-029 決策 2 fixes for all of them — event_id, " +
			"event_name, occurred_at, session_id and workspace_id — on top of the " +
			"attributes listed above. session_id is the sh_analytics cookie value: it " +
			"is what links one visitor's searches and detail views into a single " +
			"journey, and it is kept for the retention period above. workspace_id is " +
			"null until that visitor signs in, and the two are never joined backwards. " +
			"There is no free-text column anywhere in the table — not masked free " +
			"text, none — " +
			"so an attribute the schema does not declare is dropped by the writer " +
			"rather than stored. Nothing is collected at all, cookie included, " +
			"until a deployment sets a retention period.",
	})
}
