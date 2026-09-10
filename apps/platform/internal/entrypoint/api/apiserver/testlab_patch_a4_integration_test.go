package apiserver_test

import (
	"fmt"
	"net/http"
	"testing"
)

func TestARejectedRubricLeavesTheNameAlone(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "patch-atomicity")
	skillID := seedSkill(t, pool, alice.workspaceID, "patch-atomicity-skill")

	code, body := alice.doJSON(t, http.MethodPost, "/test-cases", fmt.Sprintf(
		`{"skill_id":%q,"name":"before","user_prompt":"Summarise the attached rows."}`, skillID))
	if code != http.StatusCreated {
		t.Fatalf("create test case: got %d, body %v", code, body)
	}
	id, _ := body["test_case_id"].(string)
	if id == "" {
		t.Fatalf("created test case has no id: %v", body)
	}

	code, body = alice.doJSON(t, http.MethodPatch, "/test-cases/"+id,
		`{"name":"after","rubric":{"version":"v1","items":[
		   {"id":"no-such-criterion","text":"quote the claim","evidence_required":true}]}}`)
	if code != http.StatusBadRequest {
		t.Fatalf("PATCH with an invalid rubric: got %d, body %v; want 400", code, body)
	}

	code, body = alice.doJSON(t, http.MethodGet, "/test-cases/"+id, "")
	if code != http.StatusOK {
		t.Fatalf("GET test case: got %d", code)
	}
	if body["name"] != "before" {
		t.Errorf("name = %v after a refused PATCH, want %q — a request answered 400 must not have changed anything",
			body["name"], "before")
	}
	if body["rubric"] != nil {
		t.Errorf("rubric = %v after a refused PATCH, want none", body["rubric"])
	}
}
