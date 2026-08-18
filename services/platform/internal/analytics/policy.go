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

	"github.com/ArthurC02/skillhub/services/platform/internal/platform/httpx"
)

// DataRetention handles GET /policy/data-retention.
func (h *Handler) DataRetention(w http.ResponseWriter, _ *http.Request) {
	days := 0
	if h.Svc != nil {
		days = int(h.Svc.Retention / (24 * time.Hour))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"collecting":     h.Svc.Enabled(),
		"retention_days": days,
		// The closed set of four. Adding a fifth means answering first why no
		// domain table can (ADR-029 決策 1), and the CHECK in 0029 enforces it.
		// Each `attributes` list is the constructor signature in analytics.go,
		// which is the whitelist itself: an attribute nobody declared cannot be
		// passed, so it cannot be stored.
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
				"attributes":   []string{"workspace_id", "skill_id", "arrival", "arrival_rank"},
				"not_recorded": "arrival is one of two words and arrival_rank a small integer; both are clamped on the way in, never trusted from the link",
			},
			{
				"name":         EventSessionStarted,
				"when":         "a visit begins",
				"attributes":   []string{"session_id", "occurred_at"},
				"not_recorded": "session_id is an unrelated random cookie value — not the login session token, not a hash of it, and it cannot be resolved back to one",
			},
			{
				"name":         EventDownloadStarted,
				"when":         "a download is requested",
				"attributes":   []string{"workspace_id", "artifact_id", "target"},
				"not_recorded": "whether the bytes actually went out; that is download_records, a domain fact, and the split is why this event exists",
			},
		},
		"note": "these four events are the whole of this data class. There is no " +
			"free-text column anywhere in the table — not masked free text, none — " +
			"so an attribute the schema does not declare is dropped by the writer " +
			"rather than stored. Nothing is collected at all, cookie included, " +
			"until a deployment sets a retention period.",
	})
}
