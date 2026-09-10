package analytics

import (
	"encoding/json"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

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

		if len(body.Events) != 4 {
			t.Errorf("%s: disclosed %d events, want the closed set of 4", name, len(body.Events))
		}

	}
}

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

	fixed := []string{"event_id", "event_name", "occurred_at", "session_id", "workspace_id"}
	for _, e := range body.Events {
		for _, a := range e.Attributes {
			if slices.Contains(fixed, a) {
				t.Errorf("event %q lists %q among its own attributes; every row carries it", e.Name, a)
			}
		}

		if e.Name == EventDownloadStarted && slices.Contains(e.Attributes, "target") {
			t.Error("download_started discloses `target`, which nothing stores")
		}

		if e.Name == EventSkillDetailViewed && !slices.Equal(e.Attributes, []string{"skill_id"}) {
			t.Errorf("skill_detail_viewed discloses %v, want exactly [skill_id]", e.Attributes)
		}
	}

	for _, column := range fixed {
		if !strings.Contains(body.Note, column) {
			t.Errorf("the note does not name %q, and every row carries it", column)
		}
	}

	if !strings.Contains(body.Note, sessionCookie) {
		t.Errorf("the note does not say session_id is the %s cookie", sessionCookie)
	}
}

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

func TestDataRetentionDisclosesTheFeedbackClass(t *testing.T) {
	h := &Handler{Svc: &Service{Retention: 180 * 24 * time.Hour}, FeedbackRetention: 90 * 24 * time.Hour}
	w := httptest.NewRecorder()
	h.DataRetention(w, httptest.NewRequest("GET", "/policy/data-retention", nil))

	var body struct {
		Feedback struct {
			Collected         []string `json:"collected"`
			FreeText          string   `json:"free_text"`
			Kind              []string `json:"kind"`
			RetentionDays     *int     `json:"retention_days"`
			OnAccountDeletion string   `json:"on_account_deletion"`
		} `json:"feedback"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	for _, column := range []string{"kind", "message", "page_path", "run_id", "workspace_id", "user_id"} {
		if !slices.Contains(body.Feedback.Collected, column) {
			t.Errorf("the disclosure omits the %q column", column)
		}
	}
	if !strings.Contains(body.Feedback.FreeText, "2000") {
		t.Error("the disclosure does not say how much free text a report can carry")
	}
	if !slices.Contains(body.Feedback.Kind, "blocking_issue") || !slices.Contains(body.Feedback.Kind, "need_signal") {
		t.Errorf("the disclosed kinds %v are not the two the handler accepts", body.Feedback.Kind)
	}
	if body.Feedback.RetentionDays == nil || *body.Feedback.RetentionDays != 90 {
		t.Errorf("feedback retention_days = %v, want 90 (from FEEDBACK_RETENTION, not typed into the page)", body.Feedback.RetentionDays)
	}

	if !strings.Contains(body.Feedback.OnAccountDeletion, "去識別") {
		t.Errorf("the disclosure does not say what account deletion does to a report: %q", body.Feedback.OnAccountDeletion)
	}
}

func TestDataRetentionSaysSoWhenFeedbackHasNoWindow(t *testing.T) {
	h := &Handler{Svc: &Service{Retention: time.Hour}}
	w := httptest.NewRecorder()
	h.DataRetention(w, httptest.NewRequest("GET", "/policy/data-retention", nil))

	var body struct {
		Feedback map[string]any `json:"feedback"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if _, present := body.Feedback["retention_days"]; !present {
		t.Fatal("the feedback block has no retention_days key at all; silence reads as 'not collected'")
	}
	if body.Feedback["retention_days"] != nil {
		t.Errorf("retention_days = %v with FEEDBACK_RETENTION unset, want null", body.Feedback["retention_days"])
	}
	note, _ := body.Feedback["note"].(string)
	if !strings.Contains(note, "FEEDBACK_RETENTION") || !strings.Contains(note, "一直保留") {
		t.Errorf("the note does not say the reports are kept indefinitely and which variable would change that: %q", note)
	}
}
