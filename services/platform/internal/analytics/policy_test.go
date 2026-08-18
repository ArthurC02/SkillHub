package analytics

import (
	"encoding/json"
	"net/http/httptest"
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
		for _, e := range body.Events {
			if len(e.Attributes) == 0 {
				t.Errorf("%s: event %q discloses no attribute whitelist", name, e.Name)
			}
		}
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
