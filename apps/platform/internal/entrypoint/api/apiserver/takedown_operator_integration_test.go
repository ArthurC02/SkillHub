package apiserver_test

import (
	"context"
	"net/http"
	"testing"
)

func auditReason(t *testing.T, c *client, skillID string) (string, int) {
	t.Helper()
	rows, err := requireDB(t).Query(context.Background(), `
		SELECT metadata->>'reason'
		FROM audit_events
		WHERE action = 'skill.takedown' AND resource_id = $1 AND actor_user_id = $2
		ORDER BY id DESC`, mustUUID(t, skillID), mustUUID(t, c.userID))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var first string
	n := 0
	for rows.Next() {
		var r *string
		if err := rows.Scan(&r); err != nil {
			t.Fatal(err)
		}
		if n == 0 && r != nil {
			first = *r
		}
		n++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return first, n
}

func takenDown(t *testing.T, skillID string) bool {
	t.Helper()
	var down bool
	if err := requireDB(t).QueryRow(context.Background(),
		"SELECT takedown_at IS NOT NULL FROM skills WHERE id = $1", mustUUID(t, skillID),
	).Scan(&down); err != nil {
		t.Fatal(err)
	}
	return down
}

func TestAnOperatorCanTakeDownContentTheyDoNotOwn(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	curator := a.login(t, "curator-operator-takedown")
	markCatalog(t, pool, curator.workspaceID)
	published := importPackage(t, pool, a.packages, curator, "manxome-abuse-report", false)
	other := importPackage(t, pool, a.packages, curator, "manxome-bystander", false)

	operator := a.login(t, "platform-operator-takedown")
	a.auth.Operators = map[string]bool{operator.userID: true}

	if code, _ := operatorCall(t, operator, http.MethodPost,
		"/skills/"+published+"/takedown", `{"reason":"DMCA"}`); code != http.StatusNotFound {
		t.Fatalf("owner-scoped takedown by a non-owner: got %d, want 404 — "+
			"if this passes, the new route was not needed", code)
	}

	path := "/admin/skills/" + published + "/takedown"
	code, _ := operatorCall(t, operator, http.MethodPut, path, `{"reason":"DMCA notice 2026-08-28"}`)
	if code != http.StatusOK {
		t.Fatalf("operator takedown: got %d", code)
	}
	if !takenDown(t, published) {
		t.Fatal("the skill is not marked taken down")
	}

	reason, n := auditReason(t, operator, published)
	if n != 1 {
		t.Fatalf("audit events for this takedown: %d, want 1", n)
	}
	if reason != "DMCA notice 2026-08-28" {
		t.Fatalf("audit reason = %q", reason)
	}

	resp, err := http.Get(a.URL + "/api/skills/" + published)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusGone {
		t.Fatalf("detail view of an operator-taken-down skill: got %d, want 410", resp.StatusCode)
	}

	if code, _ := operatorCall(t, operator, http.MethodPut, path, `{"reason":"again"}`); code != http.StatusConflict {
		t.Fatalf("second takedown: got %d, want 409", code)
	}

	if takenDown(t, other) {
		t.Fatal("taking one skill down also took down the one beside it")
	}
}

func TestTheOperatorTakedownRouteIsInvisibleWithoutTheRole(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	curator := a.login(t, "curator-takedown-404")
	markCatalog(t, pool, curator.workspaceID)
	published := importPackage(t, pool, a.packages, curator, "manxome-invisible-takedown", false)

	member := a.login(t, "member-takedown-404")
	anon := &client{Client: http.DefaultClient, base: a.URL}

	for name, c := range map[string]*client{"anonymous": anon, "member": member} {
		code, _ := operatorCall(t, c, http.MethodPut,
			"/admin/skills/"+published+"/takedown", `{"reason":"n"}`)
		if code != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", name, code)
		}
	}
	if takenDown(t, published) {
		t.Fatal("a caller without the operator role took content down anyway")
	}

	operator := a.login(t, "platform-operator-blank")
	a.auth.Operators = map[string]bool{operator.userID: true}
	for _, body := range []string{`{"reason":""}`, `{"reason":"   "}`, `{}`} {
		code, _ := operatorCall(t, operator, http.MethodPut,
			"/admin/skills/"+published+"/takedown", body)
		if code != http.StatusBadRequest {
			t.Errorf("takedown with body %s: got %d, want 400", body, code)
		}
	}
	if takenDown(t, published) {
		t.Fatal("a takedown with no reason went through")
	}
}
