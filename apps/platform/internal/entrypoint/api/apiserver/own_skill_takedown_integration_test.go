package apiserver_test

import (
	"context"
	"net/http"
	"testing"
	"time"
)

type ownSkillTakedownView struct {
	TakenDownAt     string  `json:"taken_down_at"`
	By              string  `json:"by"`
	RestorableUntil *string `json:"restorable_until"`
}

func ownSkillTakedowns(t *testing.T, c *client) map[string]*ownSkillTakedownView {
	t.Helper()
	var body struct {
		Skills []struct {
			SkillID  string                `json:"skill_id"`
			Takedown *ownSkillTakedownView `json:"takedown"`
		} `json:"skills"`
	}
	if code := getJSON(t, c.Client, c.base+"/skills?limit=50", &body); code != http.StatusOK {
		t.Fatalf("GET /skills: %d", code)
	}
	out := map[string]*ownSkillTakedownView{}
	for _, s := range body.Skills {
		out[s.SkillID] = s.Takedown
	}
	return out
}

func TestTheOwnListSaysWhoTookASkillDownAndUntilWhenTheOwnerMayRestoreIt(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "own-list-takedown-owner")
	operator := a.login(t, "own-list-takedown-operator")
	a.auth.Operators = map[string]bool{operator.userID: true}

	live := seedSkill(t, pool, owner.workspaceID, "own-list-live")
	ownDown := seedSkill(t, pool, owner.workspaceID, "own-list-own-down")
	operatorDown := seedSkill(t, pool, owner.workspaceID, "own-list-operator-down")
	if code, out := postJSON(t, owner, "/skills/"+ownDown+"/takedown", `{"reason":"my own mistake"}`); code != http.StatusOK {
		t.Fatalf("owner takedown: got %d (%v)", code, out)
	}
	if code, out := operatorCall(t, operator, http.MethodPut, "/admin/skills/"+operatorDown+"/takedown", `{"reason":"DMCA notice"}`); code != http.StatusOK {
		t.Fatalf("operator takedown: got %d (%v)", code, out)
	}
	var ownAt time.Time
	if err := pool.QueryRow(context.Background(), "SELECT takedown_at FROM skills WHERE id = $1", mustUUID(t, ownDown)).Scan(&ownAt); err != nil {
		t.Fatal(err)
	}

	listed := ownSkillTakedowns(t, owner)
	if got := listed[live]; got != nil {
		t.Errorf("a live skill carries a takedown: %+v", got)
	}
	own := listed[ownDown]
	if own == nil || own.By != "owner" || own.RestorableUntil == nil {
		t.Fatalf("the owner's own takedown = %+v, want by owner with a restore deadline", own)
	}
	if want := ownAt.UTC().Truncate(time.Second).Add(30 * 24 * time.Hour).Format(time.RFC3339); *own.RestorableUntil != want {
		t.Errorf("restorable_until = %s, want thirty days after the takedown, %s", *own.RestorableUntil, want)
	}
	if own.TakenDownAt != ownAt.UTC().Format(time.RFC3339) {
		t.Errorf("taken_down_at = %s, want %s", own.TakenDownAt, ownAt.UTC().Format(time.RFC3339))
	}
	if got := listed[operatorDown]; got == nil || got.By != "operator" || got.RestorableUntil != nil {
		t.Errorf("an operator takedown = %+v, want by operator and no restore deadline", got)
	}

	if code, out := postJSON(t, owner, "/skills/"+ownDown+"/restore", ""); code != http.StatusNoContent {
		t.Fatalf("restore: got %d (%v)", code, out)
	}
	if got := ownSkillTakedowns(t, owner)[ownDown]; got != nil {
		t.Errorf("a restored skill still carries a takedown: %+v", got)
	}
}
