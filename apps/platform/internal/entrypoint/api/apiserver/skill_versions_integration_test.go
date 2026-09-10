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

	otherSkill := seedSkill(t, pool, mine.workspaceID, "version-history-other-skill")
	seedVersion(t, pool, mine.workspaceID, otherSkill, "hash-other")

	got := mine.listVersions(t, skillID)
	if len(got) != 2 {
		t.Fatalf("got %d versions, want 2: %+v", len(got), got)
	}

	if got[0].VersionID != uuidText(v2.ID) || got[1].VersionID != uuidText(v1.ID) {
		t.Errorf("versions are not newest first: %+v", got)
	}
	if got[0].VersionNumber != 2 || got[1].VersionNumber != 1 {
		t.Errorf("version numbers are %d, %d; want 2, 1", got[0].VersionNumber, got[1].VersionNumber)
	}

	if got[0].ContentHash != "hash-v2" || got[0].CreatedAt == "" {
		t.Errorf("the newest row is missing its identity: %+v", got[0])
	}
}

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

	anon := &client{Client: &http.Client{}, base: a.URL}
	if code := anon.status(t, http.MethodGet, "/skills/"+skillID+"/versions"); code != http.StatusUnauthorized {
		t.Errorf("anonymous GET of the version history: got %d, want 401", code)
	}
}
