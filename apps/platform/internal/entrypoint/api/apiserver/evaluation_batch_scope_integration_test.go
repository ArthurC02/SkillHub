package apiserver_test

import (
	"net/http"
	"testing"
)

func TestABatchReportIsReadOnlyUnderItsOwnSkillAndHasNoWithheldArmUnlessAsked(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-batch-report-scope")
	f.confirmPermissions(t)
	code, body := f.doJSON(t, http.MethodPost, "/skills/"+f.skillID+"/evaluation-batches", batchBody(f.versionID, 1, f.testCaseID))
	if code != http.StatusCreated {
		t.Fatalf("create: got %d %v", code, body)
	}
	batchID := body["id"].(string)

	code, report := f.doJSON(t, http.MethodGet, "/skills/"+f.skillID+"/evaluation-batches/"+batchID, "")
	if code != http.StatusOK {
		t.Fatalf("report under its own skill: got %d %v", code, report)
	}
	for _, key := range []string{"skill_withheld", "skill_effect"} {
		if v, present := report[key]; present && v != nil {
			t.Errorf("a batch created without withholding the skill reports %s = %v", key, v)
		}
	}

	otherSkill := seedSkill(t, pool, f.workspaceID, "alice-batch-report-scope-other")
	if code, body := f.doJSON(t, http.MethodGet, "/skills/"+otherSkill+"/evaluation-batches/"+batchID, ""); code != http.StatusNotFound {
		t.Errorf("the batch read under another skill of the same workspace: got %d %v, want 404", code, body)
	}
}
