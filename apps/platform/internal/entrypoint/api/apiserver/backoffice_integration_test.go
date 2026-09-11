package apiserver_test

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func getAdmin(t *testing.T, c *client, path string) (int, map[string]any) {
	t.Helper()
	return operatorCall(t, c, http.MethodGet, path, "")
}

func strs(t *testing.T, v any) []string {
	t.Helper()
	items, ok := v.([]any)
	if !ok {
		t.Fatalf("want a JSON array, got %T (%v)", v, v)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, _ := item.(string)
		out = append(out, s)
	}
	return out
}

func objects(t *testing.T, v any) []map[string]any {
	t.Helper()
	items, ok := v.([]any)
	if !ok {
		t.Fatalf("want a JSON array, got %T (%v)", v, v)
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		m, _ := item.(map[string]any)
		out = append(out, m)
	}
	return out
}

func TestBackOfficeReadsAnswerAMemberLikeAMissingPage(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	member := a.login(t, "bo-member-refused")
	operator := a.login(t, "bo-operator-allowed")
	a.auth.Operators = map[string]bool{operator.userID: true}
	_, me := getAdmin(t, operator, "/me")
	operatorEmail, _ := me["email"].(string)

	reads := []string{
		"/admin/credits/" + member.workspaceID,
		"/admin/skills?q=anything",
		"/admin/rosters",
		"/admin/audit-log",
		"/admin/cost-statistics",
	}
	for _, path := range append([]string{"/admin/accounts?email=" + url.QueryEscape(operatorEmail)}, reads...) {
		if code, _ := getAdmin(t, member, path); code != http.StatusNotFound {
			t.Errorf("member GET %s: got %d, want 404", path, code)
		}
	}
	if n := countRow(t, pool, `SELECT count(*) FROM audit_events
		WHERE action IN ('account.lookup', 'credit.lookup') AND actor_user_id = $1`, mustUUID(t, member.userID)); n != 0 {
		t.Errorf("refused member calls left %d lookup audit events", n)
	}
	for _, path := range reads {
		if code, out := getAdmin(t, operator, path); code != http.StatusOK {
			t.Errorf("operator GET %s: got %d (%v), want 200", path, code, out)
		}
	}
}

func TestMeTellsTheCallerWhetherTheyAreAnOperator(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	member := a.login(t, "bo-me-member")
	operator := a.login(t, "bo-me-operator")
	a.auth.Operators = map[string]bool{operator.userID: true}

	for _, c := range []struct {
		name string
		cl   *client
		want bool
	}{{"member", member, false}, {"operator", operator, true}} {
		code, me := getAdmin(t, c.cl, "/me")
		got, isBool := me["operator"].(bool)
		if code != http.StatusOK || !isBool || got != c.want {
			t.Errorf("%s GET /me: code %d, operator %v, want %v", c.name, code, me["operator"], c.want)
		}
	}
}

func TestAccountLookupAuditsAHitAndNothingElse(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	member := a.login(t, "bo-lookup-member")
	departed := a.login(t, "bo-lookup-departed")
	operator := a.login(t, "bo-lookup-operator")
	a.auth.Operators = map[string]bool{operator.userID: true}

	emailOf := func(c *client) string {
		_, me := getAdmin(t, c, "/me")
		email, _ := me["email"].(string)
		if email == "" {
			t.Fatalf("GET /me gave no email: %v", me)
		}
		return email
	}
	email := emailOf(member)
	departedEmail := emailOf(departed)
	lookups := func() int {
		return countRow(t, pool, `SELECT count(*) FROM audit_events
			WHERE action = 'account.lookup' AND actor_user_id = $1`, mustUUID(t, operator.userID))
	}
	lookup := func(q string) (int, map[string]any) {
		return getAdmin(t, operator, "/admin/accounts?email="+url.QueryEscape(q))
	}

	code, out := lookup("  " + strings.ToUpper(email) + " ")
	if code != http.StatusOK {
		t.Fatalf("lookup of a live account: got %d (%v)", code, out)
	}
	if out["user_id"] != member.userID || out["workspace_id"] != member.workspaceID || out["email"] != email {
		t.Errorf("lookup named the wrong account: %v", out)
	}
	if out["deletion_requested_at"] != nil || out["in_beta_allowlist"] != true {
		t.Errorf("lookup state: deletion_requested_at %v, in_beta_allowlist %v; want null and true with no allowlist",
			out["deletion_requested_at"], out["in_beta_allowlist"])
	}
	if n := countRow(t, pool, `SELECT count(*) FROM audit_events
		WHERE action = 'account.lookup' AND actor_user_id = $1 AND resource_id = $2 AND workspace_id = $3`,
		mustUUID(t, operator.userID), mustUUID(t, member.userID), mustUUID(t, member.workspaceID)); n != 1 {
		t.Fatalf("audit events naming the looked-up account: %d, want 1", n)
	}

	if _, err := pool.Exec(context.Background(),
		"UPDATE users SET deleted_at = now() WHERE id = $1", mustUUID(t, departed.userID)); err != nil {
		t.Fatal(err)
	}
	for _, miss := range []string{"nobody-" + email, departedEmail} {
		if code, out := lookup(miss); code != http.StatusNotFound {
			t.Errorf("lookup of %q: got %d (%v), want 404", miss, code, out)
		}
	}
	for _, blank := range []string{"", "   "} {
		if code, _ := lookup(blank); code != http.StatusBadRequest {
			t.Errorf("lookup of %q: got %d, want 400", blank, code)
		}
	}
	if n := lookups(); n != 1 {
		t.Fatalf("after misses and blanks the operator has %d lookup events, want still 1", n)
	}

	var provider string
	if err := pool.QueryRow(context.Background(),
		"SELECT provider_user_id FROM user_identities WHERE user_id = $1", mustUUID(t, member.userID)).Scan(&provider); err != nil {
		t.Fatal(err)
	}
	a.auth.Invited = map[string]bool{"someone-else": true}
	if _, out := lookup(email); out["in_beta_allowlist"] != false {
		t.Errorf("an account missing from the allowlist: in_beta_allowlist %v, want false", out["in_beta_allowlist"])
	}
	a.auth.Invited = map[string]bool{"someone-else": true, provider: true}
	if _, out := lookup(email); out["in_beta_allowlist"] != true {
		t.Errorf("an account on the allowlist: in_beta_allowlist %v, want true", out["in_beta_allowlist"])
	}
}

func TestCreditLedgerShowsTheGrantAndAuditsEveryRead(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	member := a.login(t, "bo-ledger-member")
	operator := a.login(t, "bo-ledger-operator")
	a.auth.Operators = map[string]bool{operator.userID: true}
	path := "/admin/credits/" + member.workspaceID

	code, granted := postJSON(t, operator, path+"/grants", `{"amount_credits":30,"reason":"beta reward"}`)
	if code != http.StatusOK {
		t.Fatalf("grant: got %d (%v)", code, granted)
	}
	reads := func() int {
		return countRow(t, pool, `SELECT count(*) FROM audit_events
			WHERE action = 'credit.lookup' AND actor_user_id = $1 AND resource_id = $2 AND workspace_id = $3`,
			mustUUID(t, operator.userID), mustUUID(t, member.userID), mustUUID(t, member.workspaceID))
	}

	code, out := getAdmin(t, operator, path)
	if code != http.StatusOK {
		t.Fatalf("ledger: got %d (%v)", code, out)
	}
	if out["workspace_id"] != member.workspaceID || out["balance_credits"] != granted["balance_credits"] {
		t.Errorf("ledger balance %v for %v, want %v for %s",
			out["balance_credits"], out["workspace_id"], granted["balance_credits"], member.workspaceID)
	}
	entries := objects(t, out["entries"])
	if len(entries) == 0 {
		t.Fatal("the ledger lists no entries after a grant")
	}
	if e := entries[0]; e["kind"] != "grant" || e["delta_credits"] != float64(30) ||
		e["ref_type"] != nil || e["estimated"] != false || e["created_at"] == "" {
		t.Errorf("newest entry: %v, want the 30-credit operator grant", e)
	}
	if n := reads(); n != 1 {
		t.Fatalf("audit events after one ledger read: %d, want 1", n)
	}
	getAdmin(t, operator, path)
	if n := reads(); n != 2 {
		t.Fatalf("audit events after two ledger reads: %d, want 2", n)
	}

	for _, missing := range []string{"/admin/credits/00000000-0000-4000-8000-000000000000", "/admin/credits/not-a-uuid"} {
		if code, _ := getAdmin(t, operator, missing); code != http.StatusNotFound {
			t.Errorf("GET %s: got %d, want 404", missing, code)
		}
	}
	if n := countRow(t, pool, `SELECT count(*) FROM audit_events
		WHERE action = 'credit.lookup' AND actor_user_id = $1`, mustUUID(t, operator.userID)); n != 2 {
		t.Errorf("reads of missing workspaces were audited: %d lookup events, want 2", n)
	}
}

func TestGovernanceLookupReachesPrivateAndTakenDownSkillsButNotDeleted(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "bo-gov-owner")
	operator := a.login(t, "bo-gov-operator")
	a.auth.Operators = map[string]bool{operator.userID: true}

	private := importPackage(t, pool, a.packages, owner, "bandersnatch-private-gov", false)
	down := importPackage(t, pool, a.packages, owner, "bandersnatch-down-gov", false)
	gone := importPackage(t, pool, a.packages, owner, "bandersnatch-gone-gov", false)
	if code, out := operatorCall(t, operator, http.MethodPut, "/admin/skills/"+down+"/takedown",
		`{"reason":"DMCA governance"}`); code != http.StatusOK {
		t.Fatalf("operator takedown: got %d (%v)", code, out)
	}
	if code, out := operatorCall(t, owner, http.MethodDelete, "/skills/"+gone, ""); code >= 300 {
		t.Fatalf("owner delete: got %d (%v)", code, out)
	}

	find := func(q string) map[string]map[string]any {
		t.Helper()
		code, out := getAdmin(t, operator, "/admin/skills?q="+url.QueryEscape(q))
		if code != http.StatusOK {
			t.Fatalf("GET /admin/skills?q=%s: got %d (%v)", q, code, out)
		}
		byID := map[string]map[string]any{}
		for _, s := range objects(t, out["skills"]) {
			id, _ := s["skill_id"].(string)
			byID[id] = s
		}
		return byID
	}

	byName := find("BANDERSNATCH")
	if len(byName) != 2 || byName[private] == nil || byName[down] == nil {
		t.Fatalf("name search found %v, want exactly the private and the taken-down skill", byName)
	}
	if s := byName[private]; s["workspace_id"] != owner.workspaceID || s["name"] != "bandersnatch-private-gov" ||
		s["access_restriction"] != nil || s["takedown_at"] != nil || s["redistribution"] == "" {
		t.Errorf("private skill governance: %v", s)
	}
	if s := byName[down]; s["takedown_at"] == nil || s["takedown_reason"] != "DMCA governance" {
		t.Errorf("taken-down skill governance: %v", s)
	}
	if byID := find(private); len(byID) != 1 || byID[private] == nil {
		t.Errorf("id search found %v, want only %s", byID, private)
	}
	if byID := find(gone); len(byID) != 0 {
		t.Errorf("a deleted skill is still found by id: %v", byID)
	}
	if code, _ := getAdmin(t, operator, "/admin/skills?q=%20"); code != http.StatusBadRequest {
		t.Errorf("blank q: got %d, want 400", code)
	}
}

func TestOperatorAuditLogListsOnlyOperatorActions(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "bo-log-owner")
	operator := a.login(t, "bo-log-operator")
	a.auth.Operators = map[string]bool{operator.userID: true}

	selfDown := importPackage(t, pool, a.packages, owner, "jubjub-self-down", false)
	operatorDown := importPackage(t, pool, a.packages, owner, "jubjub-operator-down", false)
	held := importPackage(t, pool, a.packages, owner, "jubjub-held", false)
	if code, out := postJSON(t, owner, "/skills/"+selfDown+"/takedown", `{"reason":"my own mistake"}`); code != http.StatusOK {
		t.Fatalf("owner takedown: got %d (%v)", code, out)
	}
	if code, out := operatorCall(t, operator, http.MethodPut, "/admin/skills/"+operatorDown+"/takedown",
		`{"reason":"DMCA log"}`); code != http.StatusOK {
		t.Fatalf("operator takedown: got %d (%v)", code, out)
	}
	if code, out := operatorCall(t, operator, http.MethodPut, "/admin/skills/"+held+"/restriction",
		`{"reason":"license-review","note":"log"}`); code != http.StatusOK {
		t.Fatalf("operator restriction: got %d (%v)", code, out)
	}
	if code, _ := getAdmin(t, operator, "/admin/credits/"+owner.workspaceID); code != http.StatusOK {
		t.Fatalf("ledger read: got %d", code)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO audit_events
		(actor_user_id, action, resource_type, resource_id, metadata, created_at)
		VALUES ($1, 'credit.lookup', 'credit_account', $2, '{}', '2020-01-01T00:00:00Z')`,
		mustUUID(t, operator.userID), mustUUID(t, owner.userID)); err != nil {
		t.Fatal(err)
	}

	var events []map[string]any
	for offset := 0; ; offset += 100 {
		code, out := getAdmin(t, operator, "/admin/audit-log?limit=100&offset="+strconv.Itoa(offset))
		if code != http.StatusOK {
			t.Fatalf("audit log page at %d: got %d (%v)", offset, code, out)
		}
		page := objects(t, out["events"])
		events = append(events, page...)
		if len(page) < 100 {
			break
		}
	}
	has := func(action, resource string) bool {
		return slices.ContainsFunc(events, func(e map[string]any) bool {
			return e["action"] == action && e["resource_id"] == resource && e["actor_user_id"] == operator.userID
		})
	}
	if !has("skill.takedown", operatorDown) || !has("skill.access_restrict", held) || !has("credit.lookup", owner.userID) {
		t.Errorf("the log misses an operator action; got %d events", len(events))
	}
	for _, e := range events {
		if e["resource_id"] == selfDown || e["action"] == "skill.import" {
			t.Errorf("a non-operator action is in the operator log: %v", e)
		}
		if _, isObject := e["metadata"].(map[string]any); !isObject {
			t.Errorf("event metadata is %T, want an object: %v", e["metadata"], e)
		}
	}

	for i := 1; i < len(events); i++ {
		prev, _ := time.Parse(time.RFC3339, events[i-1]["occurred_at"].(string))
		cur, _ := time.Parse(time.RFC3339, events[i]["occurred_at"].(string))
		if cur.After(prev) {
			t.Fatalf("event %d at %v is newer than event %d at %v; the log must be newest first", i, cur, i-1, prev)
		}
	}
	_, first := getAdmin(t, operator, "/admin/audit-log?limit=1")
	_, second := getAdmin(t, operator, "/admin/audit-log?limit=1&offset=1")
	if a1, a2 := objects(t, first["events"]), objects(t, second["events"]); len(a1) != 1 || len(a2) != 1 {
		t.Fatalf("limit=1 pages hold %d and %d events, want 1 and 1", len(a1), len(a2))
	}

	for query, want := range map[string]int{
		"limit=0": http.StatusBadRequest, "limit=100": http.StatusOK, "limit=101": http.StatusBadRequest,
		"limit=abc": http.StatusBadRequest, "offset=0": http.StatusOK, "offset=-1": http.StatusBadRequest,
	} {
		if code, _ := getAdmin(t, operator, "/admin/audit-log?"+query); code != want {
			t.Errorf("audit log ?%s: got %d, want %d", query, code, want)
		}
	}
}

func TestCostStatisticsShowTheNewestWindowOfEachKind(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	operator := a.login(t, "bo-cost-operator")
	a.auth.Operators = map[string]bool{operator.userID: true}

	ctx := context.Background()
	far := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM cost_statistics WHERE window_end >= $1", far) })
	insert := func(kind string, end time.Time, samples int64, p *int64) {
		if _, err := pool.Exec(ctx, `INSERT INTO cost_statistics
			(kind, window_start, window_end, sample_count, p50_usd_micros, p90_usd_micros, p95_usd_micros, max_usd_micros)
			VALUES ($1, $2, $3, $4, $5, $5, $5, $5)`, kind, end.Add(-time.Hour), end, samples, p); err != nil {
			t.Fatal(err)
		}
	}
	micros := int64(1200)
	insert("match_reasons", far, 7, &micros)
	insert("match_reasons", far.Add(24*time.Hour), 9, &micros)
	insert("index_enrich", far, 0, nil)

	code, out := getAdmin(t, operator, "/admin/cost-statistics")
	if code != http.StatusOK {
		t.Fatalf("cost statistics: got %d (%v)", code, out)
	}
	byKind := map[string]map[string]any{}
	for _, s := range objects(t, out["statistics"]) {
		kind, _ := s["kind"].(string)
		if byKind[kind] != nil {
			t.Errorf("kind %s is listed twice", kind)
		}
		byKind[kind] = s
	}
	if s := byKind["match_reasons"]; s == nil || s["sample_count"] != float64(9) ||
		s["window_end"] != far.Add(24*time.Hour).Format(time.RFC3339) || s["p95_usd_micros"] != float64(1200) {
		t.Errorf("match_reasons: %v, want the newer nine-sample window", s)
	}
	if s := byKind["index_enrich"]; s == nil || s["p50_usd_micros"] != nil || s["sample_count"] != float64(0) {
		t.Errorf("index_enrich: %v, want a window with no percentiles", s)
	}
}

func TestRostersShowWhatIsInForce(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	operator := a.login(t, "bo-roster-operator")
	const other = "00000000-0000-4000-8000-0000000000aa"
	a.auth.Operators = map[string]bool{operator.userID: true, other: true, "11111111-0000-4000-8000-000000000000": false}

	_, out := getAdmin(t, operator, "/admin/rosters")
	want := []string{operator.userID, other}
	slices.Sort(want)
	if got := strs(t, out["operator_user_ids"]); !slices.Equal(got, want) {
		t.Errorf("operator_user_ids %v, want %v", got, want)
	}
	if got := strs(t, out["beta_allowlist"]); len(got) != 0 {
		t.Errorf("beta_allowlist with none configured: %v, want []", got)
	}

	a.auth.Invited = map[string]bool{"zeta": true, "alpha": true, "off": false}
	_, out = getAdmin(t, operator, "/admin/rosters")
	if got := strs(t, out["beta_allowlist"]); !slices.Equal(got, []string{"alpha", "zeta"}) {
		t.Errorf("beta_allowlist %v, want [alpha zeta]", got)
	}
}
