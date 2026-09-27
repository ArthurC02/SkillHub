package creation

import (
	"context"
	"fmt"
	"testing"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
)

func TestAStepOffersOnlyTheToolsThisServiceHasWired(t *testing.T) {
	knowledge := func(context.Context, identity.Workspace, []string) ([]Reference, float64, error) { return nil, 0, nil }
	fetch := func(context.Context, string) (Fetch, string) { return Fetch{}, "" }
	for _, tc := range []struct {
		name         string
		svc          *Service
		searchRounds int
		want         []string
	}{
		{"nothing extra wired", &Service{}, 0, []string{"validate_draft", "search_catalog"}},
		{"knowledge search wired", &Service{SearchKnowledge: knowledge}, 0, []string{"validate_draft", "search_catalog", "search_knowledge"}},
		{"fetch wired", &Service{Fetch: fetch}, 0, []string{"validate_draft", "search_catalog", "fetch_url"}},
		{"search rounds spent", &Service{SearchKnowledge: knowledge, Fetch: fetch}, MaxSearchRounds, []string{"validate_draft", "fetch_url"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := envelope{Limits: Limits{MaxToolCalls: 8}, Snapshot: Snapshot{SearchRounds: tc.searchRounds}}
			got := tc.svc.stepRequest(JobArgs{}, 1, e, nil).AllowedTools
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("allowed tools = %v, want %v", got, tc.want)
			}
		})
	}
}
