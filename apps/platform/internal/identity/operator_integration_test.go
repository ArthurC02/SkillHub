// 02:SEC-011 database-backed tests: the platform operator role and the one
// action it can currently perform — setting and lifting the 0023 licensing hold.
// Shared harness (TestMain, migrate, requireDB, login, markCatalog,
// importPackage) lives in authz_integration_test.go and disc_integration_test.go.
package identity_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// operatorCall sends a body-carrying request on a method net/http has no helper
// for. DELETE with a body is unusual and deliberate: SEC-011 requires a reason on
// every operator action, and lifting a hold is an operator action.
func operatorCall(t *testing.T, c *client, method, path, body string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, c.base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// auditNote reads one audit event's before/after/note back out of the trail.
// Pointers because "no hold" has to stay distinguishable from "a hold with a
// blank reason" — the second is a state 0023's CHECK makes impossible, so a test
// that cannot tell them apart would pass on a broken write.
func auditNote(t *testing.T, c *client, action, skillID string) (before, after, note *string, count int) {
	t.Helper()
	pool := requireDB(t)
	rows, err := pool.Query(context.Background(), `
		SELECT metadata->>'before', metadata->>'after', metadata->>'note'
		FROM audit_events
		WHERE action = $1 AND resource_id = $2 AND actor_user_id = $3
		ORDER BY id DESC`, action, mustUUID(t, skillID), mustUUID(t, c.userID))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var b, a, n *string
		if err := rows.Scan(&b, &a, &n); err != nil {
			t.Fatal(err)
		}
		if count == 0 {
			before, after, note = b, a, n
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return before, after, note, count
}

func deref(s *string) string {
	if s == nil {
		return "<null>"
	}
	return *s
}

// SEC-011: 「`member` 呼叫 operator 端點時回 404」. Not 401, not 403 — a member must
// not be able to learn that the endpoint exists at all, which is the same
// non-disclosure rule WS-006 applies to other people's content.
func TestOperatorRoutesAreInvisibleWithoutTheRole(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	curator := a.login(t, "curator-operator-404")
	markCatalog(t, pool, curator.workspaceID)
	held := importPackage(t, pool, a.packages, curator, "vorpal-invisible-writer", false)

	member := a.login(t, "member-operator-404")
	anon := &client{Client: http.DefaultClient, base: a.URL}
	// Nobody is on the roster: the shipped default, and the state every
	// deployment that never sets OPERATOR_USER_IDS is in.

	for _, c := range []struct {
		name string
		cl   *client
	}{{"anonymous", anon}, {"member", member}} {
		for _, tc := range []struct{ method, body string }{
			{http.MethodPut, `{"reason":"license-review","note":"n"}`},
			{http.MethodDelete, `{"note":"n"}`},
		} {
			code, _ := operatorCall(t, c.cl, tc.method, "/admin/skills/"+held+"/restriction", tc.body)
			if code != http.StatusNotFound {
				t.Errorf("%s %s as %s: got %d, want 404", tc.method, "/admin/skills/{id}/restriction", c.name, code)
			}
		}
	}
	// And the refusal really refused: nothing was written.
	if n := countRow(t, pool,
		"SELECT count(*) FROM skills WHERE id = $1 AND access_restriction IS NOT NULL", mustUUID(t, held)); n != 0 {
		t.Fatal("a caller without the operator role changed the hold anyway")
	}

	// A logged-in member is still a member after somebody else is made operator.
	a.auth.Operators = map[string]bool{curator.userID: true}
	code, _ := operatorCall(t, member, http.MethodPut, "/admin/skills/"+held+"/restriction",
		`{"reason":"license-review","note":"n"}`)
	if code != http.StatusNotFound {
		t.Errorf("member call while another user is operator: got %d, want 404", code)
	}
}

// SEC-011 追加小節 end to end: an operator puts the licensing hold on, the read
// paths honour it immediately, the trail says who did it and why, and lifting it
// restores exactly what it closed. Both directions are idempotent, because an
// operator repeating an action is not an error.
func TestOperatorSetsAndLiftsTheLicensingHold(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	curator := a.login(t, "curator-operator-hold")
	markCatalog(t, pool, curator.workspaceID)
	held := importPackage(t, pool, a.packages, curator, "frumious-held-writer", false)

	// The operator is a different account from the workspace owner: the whole
	// point of the role is that it reaches content it does not own.
	operator := a.login(t, "platform-operator")
	a.auth.Operators = map[string]bool{operator.userID: true}
	path := "/admin/skills/" + held + "/restriction"
	anon := &client{Client: http.DefaultClient, base: a.URL}

	if code := anon.status(t, http.MethodGet, "/api/skills/"+held+"/files"); code != http.StatusOK {
		t.Fatalf("precondition: /files answered %d before the hold, want 200", code)
	}

	// A reason and a note are both required, and the reason has to be a code the
	// platform can render a sentence for — a hold nobody can explain is the state
	// 02:SEC-011 forbids.
	for _, body := range []string{
		`{"reason":"license-review"}`,           // no note
		`{"reason":"","note":"n"}`,              // no reason
		`{"reason":"licence-revue","note":"n"}`, // typo: unknown code
	} {
		if code, _ := operatorCall(t, operator, http.MethodPut, path, body); code != http.StatusBadRequest {
			t.Errorf("PUT %s: got %d, want 400", body, code)
		}
	}

	const note = "anthropics/skills source-available terms under review with legal"
	code, out := operatorCall(t, operator, http.MethodPut, path,
		`{"reason":"license-review","note":"`+note+`"}`)
	if code != http.StatusOK {
		t.Fatalf("operator PUT: got %d (%v)", code, out)
	}
	rest, _ := out["access_restriction"].(map[string]any)
	if rest == nil || rest["reason"] != "license-review" || rest["note"] == "" {
		t.Fatalf("response did not echo the hold the reader will see: %v", out["access_restriction"])
	}
	if out["previous_reason"] != nil {
		t.Errorf("previous_reason = %v, want null for a skill that was not held", out["previous_reason"])
	}

	// The read paths were already wired to the column (0023); this is the proof
	// that the endpoint reaches the same column and that it takes effect at once.
	if code := anon.status(t, http.MethodGet, "/api/skills/"+held+"/files"); code != http.StatusForbidden {
		t.Fatalf("/files after the hold: got %d, want 403", code)
	}
	var detail map[string]any
	if code := getJSON(t, http.DefaultClient, a.URL+"/api/skills/"+held, &detail); code != http.StatusOK {
		t.Fatalf("detail after the hold: got %d, want 200 — a hold is not a takedown", code)
	}
	if d, _ := detail["access_restriction"].(map[string]any); d == nil || d["reason"] != "license-review" {
		t.Fatalf("detail did not disclose the hold: %v", detail["access_restriction"])
	}

	before, after, gotNote, n := auditNote(t, operator, "skill.access_restrict", held)
	if n != 1 {
		t.Fatalf("audit events for the hold: %d, want 1", n)
	}
	if before != nil || deref(after) != "license-review" {
		t.Errorf("audit transition = %s -> %s, want <null> -> license-review", deref(before), deref(after))
	}
	if deref(gotNote) != note {
		t.Errorf("audit note = %q, want the operator's stated reason", deref(gotNote))
	}

	// Being an operator is not a wider workspace scope (SEC-011 最小權力原則):
	// the role reaches this one column and nothing else in the workspace.
	member := a.login(t, "member-operator-scope")
	private := seedSkill(t, pool, member.workspaceID, "member-private-thing")
	if code := getJSON(t, operator.Client, a.URL+"/api/skills/"+private, nil); code != http.StatusNotFound {
		t.Errorf("operator read of a private skill: got %d, want 404", code)
	}
	if ids := operator.skillIDs(t, "/skills"); contains(ids, private) || contains(ids, held) {
		t.Error("operator's own skill list carries somebody else's content")
	}

	// Idempotent: applying the same hold again is a second audit event and no
	// change to the row, not a 409.
	code, out = operatorCall(t, operator, http.MethodPut, path,
		`{"reason":"license-review","note":"still open, re-recording"}`)
	if code != http.StatusOK {
		t.Fatalf("repeat PUT: got %d, want 200", code)
	}
	if out["previous_reason"] != "license-review" {
		t.Errorf("previous_reason = %v, want the hold that was already in place", out["previous_reason"])
	}
	if _, _, _, n := auditNote(t, operator, "skill.access_restrict", held); n != 2 {
		t.Errorf("audit events after the repeat: %d, want 2 — a repeated action is still an action", n)
	}

	// Lift it. 0023 「終判允許後,把該欄位設回 NULL 即恢復」.
	if code, out := operatorCall(t, operator, http.MethodDelete, path,
		`{"note":"legal cleared it"}`); code != http.StatusNoContent {
		t.Fatalf("operator DELETE: got %d (%v), want 204", code, out)
	}
	if code := anon.status(t, http.MethodGet, "/api/skills/"+held+"/files"); code != http.StatusOK {
		t.Fatalf("/files after lifting the hold: got %d, want 200", code)
	}
	// A fresh map: decoding into the one above would merge into it, and the key
	// under test is exactly the one that has to disappear.
	var lifted map[string]any
	if code := getJSON(t, http.DefaultClient, a.URL+"/api/skills/"+held, &lifted); code != http.StatusOK ||
		lifted["access_restriction"] != nil {
		t.Fatalf("detail still reports a lifted hold: %v", lifted["access_restriction"])
	}
	before, after, _, n = auditNote(t, operator, "skill.access_unrestrict", held)
	if n != 1 || deref(before) != "license-review" || after != nil {
		t.Errorf("lift audit = %s -> %s (%d events), want license-review -> <null>, 1", deref(before), deref(after), n)
	}

	// Lifting a hold that is not there is a no-op, not an error: the caller's
	// intent (this skill must not be held) is already satisfied.
	if code, _ := operatorCall(t, operator, http.MethodDelete, path, `{"note":"double check"}`); code != http.StatusNoContent {
		t.Errorf("repeat DELETE: got %d, want 204", code)
	}
	if _, _, _, n := auditNote(t, operator, "skill.access_unrestrict", held); n != 2 {
		t.Errorf("audit events after the repeat lift: %d, want 2", n)
	}
	// A note is still required for a no-op.
	if code, _ := operatorCall(t, operator, http.MethodDelete, path, `{}`); code != http.StatusBadRequest {
		t.Errorf("DELETE with no note: got %d, want 400", code)
	}
	// An id nothing matches is 404 for an operator too.
	if code, _ := operatorCall(t, operator, http.MethodDelete,
		"/admin/skills/00000000-0000-0000-0000-000000000001/restriction", `{"note":"n"}`); code != http.StatusNotFound {
		t.Errorf("DELETE on an unknown skill: got %d, want 404", code)
	}
}

// SEC-011 「授予或撤銷 operator 角色本身也是 audit event」, at the strength a
// configuration roster can give: the list this process came up with is recorded,
// so a roster can never be in force with no record of it.
func TestOperatorRosterIsAudited(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	operator := a.login(t, "roster-operator")
	a.auth.Operators = map[string]bool{operator.userID: true}

	if err := a.auth.LogOperatorRoster(context.Background()); err != nil {
		t.Fatal(err)
	}
	var ids []string
	var count int
	if err := pool.QueryRow(context.Background(), `
		SELECT metadata->'user_ids', (metadata->>'count')::int
		FROM audit_events WHERE action = 'operator.roster'
		ORDER BY id DESC LIMIT 1`).Scan(&ids, &count); err != nil {
		t.Fatal(err)
	}
	if count != 1 || len(ids) != 1 || ids[0] != operator.userID {
		t.Fatalf("roster event = %v (count %d), want exactly the configured operator", ids, count)
	}
}
