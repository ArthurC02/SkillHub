package apiserver_test

import (
	"net/http"
	"testing"
)

func TestATakenDownSkillCannotBePublished(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, freshName("takedown-alice"))
	registerPublisher(t, alice, freshName("takedown"))
	skillID := uploadedSkill(t, alice, freshName("takedown-skill"), "Explain it.")
	setSkill(t, pool, skillID, "takedown_at = now(), takedown_reason = 'fixture'")

	if code, body := publish(t, alice, skillID, `{"rights_attested":true}`); code != http.StatusNotFound {
		t.Fatalf("publishing a taken-down Skill: %d %v, want 404", code, body)
	}
	if n := countRow(t, pool, `SELECT count(*) FROM publications WHERE skill_id = $1`, mustUUID(t, skillID)); n != 0 {
		t.Errorf("a taken-down Skill got %d publications", n)
	}
}

func TestAPublicationRefusesAVersionOfAnotherSkill(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, freshName("foreign-version-alice"))
	registerPublisher(t, alice, freshName("foreign-version"))
	target := uploadedSkill(t, alice, freshName("foreign-target"), "Explain it.")
	other := uploadedSkill(t, alice, freshName("foreign-other"), "Summarise it.")
	var otherVersion string
	if err := pool.QueryRow(t.Context(), `SELECT id::text FROM skill_versions WHERE skill_id = $1`, mustUUID(t, other)).Scan(&otherVersion); err != nil {
		t.Fatal(err)
	}

	if code, body := publish(t, alice, target, `{"rights_attested":true,"version_id":"`+otherVersion+`"}`); code != http.StatusNotFound {
		t.Fatalf("publishing another Skill's version: %d %v, want 404", code, body)
	}
	if n := countRow(t, pool, `SELECT count(*) FROM publications WHERE skill_id = $1`, mustUUID(t, target)); n != 0 {
		t.Errorf("the Skill got %d publications from another Skill's version", n)
	}
}

func TestDelistingTwiceRecordsOneDelistInTheAuditLog(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, freshName("delist-alice"))
	registerPublisher(t, alice, freshName("delist"))
	skillID := uploadedSkill(t, alice, freshName("delist-skill"), "Explain it.")
	if code, body := publish(t, alice, skillID, `{"rights_attested":true}`); code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("publish: %d %v", code, body)
	}

	for range 2 {
		if code, body := deleteJSON(t, alice, "/skills/"+skillID+"/publication"); code != http.StatusOK || body["status"] != "delisted" {
			t.Fatalf("delist: %d %v, want 200 delisted", code, body)
		}
	}
	if n := countRow(t, pool, `SELECT count(*) FROM audit_events
		WHERE action = 'publication.delist'
		  AND resource_id::text IN (SELECT id::text FROM publications WHERE skill_id = $1)`, mustUUID(t, skillID)); n != 1 {
		t.Errorf("delisting twice wrote %d delist audit events, want 1", n)
	}
}
