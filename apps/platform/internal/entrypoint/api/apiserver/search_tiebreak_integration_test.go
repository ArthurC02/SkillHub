package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const equallyRankedSkills = 6

func seedEquallyRanked(t *testing.T, pool *pgxpool.Pool, workspaceID, name string) []string {
	t.Helper()
	ids := make([]string, 0, equallyRankedSkills)
	for i := range equallyRankedSkills {
		ids = append(ids, seedSkill(t, pool, workspaceID, name+" "+string(rune('a'+i))))
	}
	slices.Sort(ids)
	return ids
}

func TestAWorkspaceKeywordSearchOrdersEquallyRankedSkillsBySkillID(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "owner-tiebreak")
	want := seedEquallyRanked(t, pool, owner.workspaceID, "quuxtiebreak sorter")

	if got := owner.skillIDs(t, "/skills/search?q=quuxtiebreak"); !slices.Equal(got, want) {
		t.Fatalf("equally ranked skills came back as %v, want skill id order %v", got, want)
	}
}

func TestAPublicKeywordOnlySearchOrdersEquallyRankedSkillsBySkillID(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	curator := a.login(t, "curator-tiebreak")
	var curatorWS pgtype.UUID
	if err := curatorWS.Scan(curator.workspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), "UPDATE workspaces SET is_catalog = true WHERE id = $1", curatorWS); err != nil {
		t.Fatal(err)
	}
	leaveCatalogAtEnd(t, pool, curatorWS)
	want := seedEquallyRanked(t, pool, curator.workspaceID, "corgetiebreak sorter")

	resp, err := http.Get(a.URL + "/api/skills/search?q=corgetiebreak")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Degraded *json.RawMessage `json:"degraded"`
		Results  []skillRef       `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d, decode %v", resp.StatusCode, err)
	}
	if out.Degraded == nil || string(*out.Degraded) == "null" {
		t.Fatal("the search did not take the keyword-only path this test is about")
	}
	got := make([]string, 0, len(out.Results))
	for _, r := range out.Results {
		got = append(got, r.SkillID)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("equally ranked skills came back as %v, want skill id order %v", got, want)
	}
}
