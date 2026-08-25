package analytics

import (
	"encoding/json"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

// The disclosure must not promise a window the deployment does not apply. With no
// ANALYTICS_RETENTION set, the honest answer is "collecting nothing" and a zero
// window — the shipped default (NFR-002, ADR-029 決策 5).
func TestDataRetentionIsHonestWhenNothingIsCollected(t *testing.T) {
	for name, h := range map[string]*Handler{
		"nil service":               {},
		"zero retention":            {Svc: &Service{}},
		"retention set but no pool": {Svc: &Service{Retention: 180 * 24 * time.Hour}},
	} {
		w := httptest.NewRecorder()
		h.DataRetention(w, httptest.NewRequest("GET", "/policy/data-retention", nil))

		var body struct {
			Collecting    bool `json:"collecting"`
			RetentionDays int  `json:"retention_days"`
			Events        []struct {
				Name       string   `json:"name"`
				Attributes []string `json:"attributes"`
			} `json:"events"`
		}
		if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if body.Collecting {
			t.Errorf("%s: reports collecting with no pool or no retention", name)
		}
		// The four events are disclosed whether or not this deployment writes
		// them: "we collect nothing" is a disclosure, not an absence of one.
		if len(body.Events) != 4 {
			t.Errorf("%s: disclosed %d events, want the closed set of 4", name, len(body.Events))
		}
		// No per-event attribute check here: session_started adds nothing to the
		// columns every row carries, and those are disclosed once in `note`. What
		// each list may and may not contain is
		// TestTheDisclosureNamesWhatIsStoredAndNothingElse.
	}
}

// 02:O11Y-004 and ADR-029 決策 5 put a higher disclosure duty on this page than
// anywhere else, and it runs in both directions: naming a column nobody fills is
// as wrong as hiding one that is. Both were true before the M5 audit (2026-08-25)
// — `download_started` declared a `target` no caller has ever passed, and
// `session_id` was named for one event out of four although every row carries it.
func TestTheDisclosureNamesWhatIsStoredAndNothingElse(t *testing.T) {
	h := &Handler{Svc: &Service{Retention: 365 * 24 * time.Hour}}
	w := httptest.NewRecorder()
	h.DataRetention(w, httptest.NewRequest("GET", "/policy/data-retention", nil))

	var body struct {
		Events []struct {
			Name       string   `json:"name"`
			Attributes []string `json:"attributes"`
		} `json:"events"`
		Note string `json:"note"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	// ADR-029 決策 2's fixed columns. They belong to every event, so a list that
	// repeats one is describing the row rather than the event — which is how
	// three of the four came to look as though they carried no session id.
	fixed := []string{"event_id", "event_name", "occurred_at", "session_id", "workspace_id"}
	for _, e := range body.Events {
		for _, a := range e.Attributes {
			if slices.Contains(fixed, a) {
				t.Errorf("event %q lists %q among its own attributes; every row carries it", e.Name, a)
			}
		}
		// DownloadStarted writes `target` only when non-empty and the one caller
		// passes "" (feedback.go), so the column has never held a value. A caller
		// that starts passing one has to put it back here in the same change.
		if e.Name == EventDownloadStarted && slices.Contains(e.Attributes, "target") {
			t.Error("download_started discloses `target`, which nothing stores")
		}
		// 0040 dropped `arrival` and `arrival_rank` (04 丙-59), so the skill is the
		// whole of this event. Exact rather than "does not contain arrival": the
		// duty here runs in both directions, and skill_id going missing is the
		// half a blocklist would not catch.
		if e.Name == EventSkillDetailViewed && !slices.Equal(e.Attributes, []string{"skill_id"}) {
			t.Errorf("skill_detail_viewed discloses %v, want exactly [skill_id]", e.Attributes)
		}
	}

	for _, column := range fixed {
		if !strings.Contains(body.Note, column) {
			t.Errorf("the note does not name %q, and every row carries it", column)
		}
	}
	// The one that matters: a reader who sees only "query_length" cannot tell
	// that the row is stitched to the detail page they opened ten minutes later.
	if !strings.Contains(body.Note, sessionCookie) {
		t.Errorf("the note does not say session_id is the %s cookie", sessionCookie)
	}
}

// `collecting: true` with `retention_days: 0` is two sentences that contradict
// each other. Enabled() admits any window of a second or more, so integer
// truncation produced exactly that for every staging window under a day.
func TestARetentionWindowShorterThanADayIsNotReportedAsZero(t *testing.T) {
	h := &Handler{Svc: &Service{Retention: 12 * time.Hour}}
	w := httptest.NewRecorder()
	h.DataRetention(w, httptest.NewRequest("GET", "/policy/data-retention", nil))

	var body struct {
		RetentionDays int `json:"retention_days"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.RetentionDays != 1 {
		t.Errorf("retention_days = %d for a 12-hour window, want 1", body.RetentionDays)
	}
}

// The window is read from the service, never typed into the response.
func TestDataRetentionReportsTheConfiguredWindow(t *testing.T) {
	h := &Handler{Svc: &Service{Retention: 180 * 24 * time.Hour}}
	w := httptest.NewRecorder()
	h.DataRetention(w, httptest.NewRequest("GET", "/policy/data-retention", nil))

	var body struct {
		RetentionDays int `json:"retention_days"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.RetentionDays != 180 {
		t.Errorf("retention_days = %d, want 180", body.RetentionDays)
	}
}
