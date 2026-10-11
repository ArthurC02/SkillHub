package operations

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/agentloop"
)

func TestASightingJoinsTheFindingItSharesACiteWith(t *testing.T) {
	day := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	findings := []matchable{
		{status: FindingResolved, cites: []string{"/a", "/b"}, lastSeen: day},
		{status: FindingOpen, cites: []string{"/b"}, lastSeen: day.AddDate(0, 0, -5)},
		{status: FindingRecovered, cites: []string{"/c"}, lastSeen: day.AddDate(0, 0, -2)},
		{status: FindingDismissed, cites: []string{"/c"}, lastSeen: day.AddDate(0, 0, -1)},
		{status: FindingAcknowledged, cites: []string{"/d"}, lastSeen: day},
	}
	cases := []struct {
		name      string
		sightings [][]string
		want      []int
	}{
		{"nothing in common is new", [][]string{{"/z"}}, []int{-1}},
		{"a live finding wins over a fresher closed one", [][]string{{"/b"}}, []int{1}},
		{"among closed ones the most recently seen wins", [][]string{{"/c"}}, []int{3}},
		{"one shared cite is enough", [][]string{{"/x", "/d"}}, []int{4}},
		{"a finding is taken once, the next sighting falls through", [][]string{{"/b"}, {"/b"}}, []int{1, 0}},
		{"once every candidate is taken the sighting is new", [][]string{{"/d"}, {"/d"}}, []int{4, -1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sightings := make([]Sighting, len(tc.sightings))
			for i, cites := range tc.sightings {
				sightings[i] = Sighting{Cites: cites}
			}
			if got := matchSightings(sightings, findings); !slices.Equal(got, tc.want) {
				t.Errorf("matched %v, want %v", got, tc.want)
			}
		})
	}
}

func TestOnlyAttentionItemsBecomeSightingsWithTheValuesTheyCite(t *testing.T) {
	steps := []agentloop.StepRecord{{Result: `{"jobs":{"purge":{"overdue":2.5}},"size":10}`}}
	result := json.RawMessage(`{"items":[
		{"status":"fine","text":"size ok","cites":["/size"]},
		{"status":"attention","text":"purge late","cites":["/jobs/purge/overdue","/missing"]}
	]}`)
	got := DailyReportSightings(result, steps)
	if len(got) != 1 {
		t.Fatalf("sightings %+v, want the one attention item", got)
	}
	if got[0].Title != "purge late" || !slices.Equal(got[0].Cites, []string{"/jobs/purge/overdue", "/missing"}) {
		t.Errorf("sighting %+v, want the item's text and cites", got[0])
	}
	if len(got[0].Evidence) != 1 || got[0].Evidence["/jobs/purge/overdue"] != 2.5 {
		t.Errorf("evidence %v, want only the cite that resolves, with its value", got[0].Evidence)
	}
	if DailyReportSightings(json.RawMessage(`not json`), steps) != nil {
		t.Error("a result that is not a report yielded sightings")
	}
}
