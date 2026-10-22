package apiserver_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAnOwnerDeletionUnderAnOperatorHoldKeepsTheContentUntilTheOperatorReleasesIt(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	a := newAPI(t, pool)
	owner := a.login(t, freshName("hold-owner"))
	operator := a.login(t, freshName("hold-operator"))
	a.auth.Operators = map[string]bool{operator.userID: true}

	operatorDown := importPackage(t, pool, a.packages, owner, freshName("hold-operator-down"), false)
	restricted := importPackage(t, pool, a.packages, owner, freshName("hold-restricted"), false)
	selfDown := importPackage(t, pool, a.packages, owner, freshName("hold-self-down"), false)
	free := importPackage(t, pool, a.packages, owner, freshName("hold-free"), false)

	if code, out := operatorCall(t, operator, http.MethodPut, "/admin/skills/"+operatorDown+"/takedown",
		`{"reason":"DMCA notice"}`); code != http.StatusOK {
		t.Fatalf("operator takedown: got %d (%v)", code, out)
	}
	if code, out := operatorCall(t, operator, http.MethodPut, "/admin/skills/"+restricted+"/restriction",
		`{"reason":"license-review","note":"checking the licence"}`); code != http.StatusOK {
		t.Fatalf("operator restriction: got %d (%v)", code, out)
	}
	if code, out := postJSON(t, owner, "/skills/"+selfDown+"/takedown", `{"reason":"my own mistake"}`); code != http.StatusOK {
		t.Fatalf("owner takedown: got %d (%v)", code, out)
	}

	assertDeletionNote(t, owner, operatorDown, "平台營運處理中")
	assertDeletionNote(t, owner, restricted, "平台營運處理中")
	assertDeletionNote(t, owner, selfDown, "版本快照維持凍結")
	assertDeletionNote(t, owner, free, "版本快照維持凍結")
	ageSkillDeletionsPastGrace(t, pool, operatorDown, restricted, selfDown, free)

	purger := registryPurger(pool)
	if _, err := purger.PurgeDeletedSkills(ctx, 30*24*time.Hour, 100); err != nil {
		t.Fatal(err)
	}
	assertSkillsKeptWithTheirVersions(t, pool, []keptSkill{
		{operatorDown, "an operator takedown is still in force"},
		{restricted, "an operator access restriction is still in force"},
	})
	assertSkillAndItsVersionsPurged(t, pool, selfDown)
	assertSkillAndItsVersionsPurged(t, pool, free)

	if code, out := operatorCall(t, operator, http.MethodDelete, "/admin/skills/"+operatorDown+"/takedown", ""); code != http.StatusNoContent {
		t.Fatalf("operator lift on a skill its owner deleted: got %d (%v)", code, out)
	}
	if c := countRow(t, pool, "SELECT count(*) FROM search_documents WHERE skill_id = $1", mustUUID(t, operatorDown)); c != 0 {
		t.Error("lifting the takedown put a skill its owner deleted back into search")
	}
	if code, out := operatorCall(t, operator, http.MethodDelete, "/admin/skills/"+restricted+"/restriction",
		`{"note":"licence confirmed"}`); code != http.StatusNoContent {
		t.Fatalf("operator clearing the restriction on a skill its owner deleted: got %d (%v)", code, out)
	}

	if _, err := purger.PurgeDeletedSkills(ctx, 30*24*time.Hour, 100); err != nil {
		t.Fatal(err)
	}
	assertSkillAndItsVersionsPurged(t, pool, operatorDown)
	assertSkillAndItsVersionsPurged(t, pool, restricted)
}

func assertDeletionNote(t *testing.T, owner *client, skillID, want string) {
	t.Helper()
	code, body := deleteJSON(t, owner, "/skills/"+skillID)
	if code != http.StatusOK {
		t.Fatalf("DELETE /skills/%s: got %d (%v)", skillID, code, body)
	}
	if note, _ := body["note"].(string); !strings.Contains(note, want) {
		t.Errorf("deletion note for %s = %q, want it to say %q", skillID, note, want)
	}
}

func TestAnOperatorCanHoldASkillItsOwnerDeletedFirst(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	a := newAPI(t, pool)
	owner := a.login(t, freshName("deleted-first-owner"))
	operator := a.login(t, freshName("deleted-first-operator"))
	a.auth.Operators = map[string]bool{operator.userID: true}

	takenDown := importPackage(t, pool, a.packages, owner, freshName("deleted-first-down"), false)
	restricted := importPackage(t, pool, a.packages, owner, freshName("deleted-first-restricted"), false)
	untouched := importPackage(t, pool, a.packages, owner, freshName("deleted-first-untouched"), false)
	for _, id := range []string{takenDown, restricted, untouched} {
		if code, out := deleteJSON(t, owner, "/skills/"+id); code != http.StatusOK {
			t.Fatalf("owner delete %s: got %d (%v)", id, code, out)
		}
	}

	code, out := operatorCall(t, operator, http.MethodGet, "/admin/skills?q="+takenDown, "")
	skills, _ := out["skills"].([]any)
	if code != http.StatusOK || len(skills) != 1 {
		t.Fatalf("operator search for a deleted skill: got %d with %d skills (%v), want it listed", code, len(skills), out)
	}
	if found, _ := skills[0].(map[string]any); found["deleted_at"] == nil {
		t.Errorf("the deleted skill is listed without deleted_at: %v", found)
	}

	if code, out := operatorCall(t, operator, http.MethodPut, "/admin/skills/"+takenDown+"/takedown",
		`{"reason":"DMCA notice"}`); code != http.StatusOK {
		t.Fatalf("operator takedown of a deleted skill: got %d (%v)", code, out)
	}
	if code, out := operatorCall(t, operator, http.MethodPut, "/admin/skills/"+restricted+"/restriction",
		`{"reason":"license-review","note":"checking the licence"}`); code != http.StatusOK {
		t.Fatalf("operator restriction of a deleted skill: got %d (%v)", code, out)
	}
	if code, out := operatorCall(t, operator, http.MethodPut, "/admin/skills/"+untouched+"/redistribution",
		`{"value":"blocked","note":"no licence recorded"}`); code != http.StatusNotFound {
		t.Errorf("redistribution verdict on a deleted skill: got %d (%v), want 404", code, out)
	}

	ageSkillDeletionsPastGrace(t, pool, takenDown, restricted, untouched)
	if _, err := registryPurger(pool).PurgeDeletedSkills(ctx, 30*24*time.Hour, 100); err != nil {
		t.Fatal(err)
	}
	assertSkillsKeptWithTheirVersions(t, pool, []keptSkill{
		{takenDown, "an operator took it down after its owner deleted it"},
		{restricted, "an operator restricted it after its owner deleted it"},
	})
	assertSkillAndItsVersionsPurged(t, pool, untouched)
}
