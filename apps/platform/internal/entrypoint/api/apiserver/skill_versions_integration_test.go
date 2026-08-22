// WS-001's read half through the real route table: the version history of one
// skill, which is what the pre-run permission screen and the packaging screen
// pick a version from.
//
// The same gap as 04 丙-22 in a different place (04 丙-14): ListSkillVersions has
// been a generated query since M1 and no service ever called it, so the two
// screens that need a version took the id from the URL and told the reader to
// paste one by hand. A query nothing calls is not a capability a user has.
package apiserver_test

import (
	"net/http"
	"testing"
)

type versionView struct {
	VersionID     string `json:"version_id"`
	VersionNumber int32  `json:"version_number"`
	ContentHash   string `json:"content_hash"`
	CreatedAt     string `json:"created_at"`
}

func (c *client) listVersions(t *testing.T, skillID string) []versionView {
	t.Helper()
	var out struct {
		Versions []versionView `json:"versions"`
	}
	path := c.base + "/skills/" + skillID + "/versions"
	if code := getJSON(t, c.Client, path, &out); code != http.StatusOK {
		t.Fatalf("GET /skills/%s/versions: got %d", skillID, code)
	}
	return out.Versions
}

func TestTheVersionHistoryListsASkillsOwnVersionsNewestFirst(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	mine := a.login(t, "version-history-owner")

	skillID := seedSkill(t, pool, mine.workspaceID, "version-history-skill")
	v1 := seedVersion(t, pool, mine.workspaceID, skillID, "hash-v1")
	v2 := seedVersion(t, pool, mine.workspaceID, skillID, "hash-v2")
	// A second skill in the same workspace: the list is per skill, and a picker
	// offering another skill's versions would offer a run that cannot happen.
	otherSkill := seedSkill(t, pool, mine.workspaceID, "version-history-other-skill")
	seedVersion(t, pool, mine.workspaceID, otherSkill, "hash-other")

	got := mine.listVersions(t, skillID)
	if len(got) != 2 {
		t.Fatalf("got %d versions, want 2: %+v", len(got), got)
	}
	// Newest first, because the picker's first row is the one a reader reads as
	// "the current one".
	if got[0].VersionID != uuidText(v2.ID) || got[1].VersionID != uuidText(v1.ID) {
		t.Errorf("versions are not newest first: %+v", got)
	}
	if got[0].VersionNumber != 2 || got[1].VersionNumber != 1 {
		t.Errorf("version numbers are %d, %d; want 2, 1", got[0].VersionNumber, got[1].VersionNumber)
	}
	// The content hash and the creation time travel with the row: a version
	// identified only by a uuid is one nobody can tell apart from another.
	if got[0].ContentHash != "hash-v2" || got[0].CreatedAt == "" {
		t.Errorf("the newest row is missing its identity: %+v", got[0])
	}
}

// Iron rule 3: the workspace comes from the session, never from the request. A
// stranger who knows the skill id gets an empty history rather than somebody
// else's versions — which is what stops a picker from offering a version its
// reader could not run or package anyway.
func TestTheVersionHistoryIsScopedToTheCallersWorkspace(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "version-scope-owner")
	skillID := seedSkill(t, pool, owner.workspaceID, "version-scope-skill")
	seedVersion(t, pool, owner.workspaceID, skillID, "hash-scoped")

	stranger := a.login(t, "version-scope-stranger")
	if got := stranger.listVersions(t, skillID); len(got) != 0 {
		t.Errorf("a stranger reads %d of the owner's versions: %+v", len(got), got)
	}

	// And no session reads nothing at all. Asserted here because the route table
	// test's list is hand-kept and this route is not on it.
	anon := &client{Client: &http.Client{}, base: a.URL}
	if code := anon.status(t, http.MethodGet, "/skills/"+skillID+"/versions"); code != http.StatusUnauthorized {
		t.Errorf("anonymous GET of the version history: got %d, want 401", code)
	}
}
