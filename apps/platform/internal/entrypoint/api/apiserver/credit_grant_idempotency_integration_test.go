package apiserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func grantCounts(t *testing.T, member, operator *creditsClient) (entries, audits int) {
	t.Helper()
	pool := creditsTestPool(t)
	ctx := context.Background()
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM credit_entries WHERE user_id = $1`,
		mustParseUUID(t, member.userID)).Scan(&entries); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_events
		WHERE action = 'credit.grant' AND actor_user_id = $1 AND workspace_id = $2`,
		mustParseUUID(t, operator.userID), mustParseUUID(t, member.workspaceID)).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	return entries, audits
}

func TestAReplayedGrantKeyGrantsAndAuditsOnce(t *testing.T) {
	app, srv := realCreditsServer(t, creditsTestPool(t))
	member := creditsLogin(t, srv, "credits-grant-replay-member")
	operator := creditsLogin(t, srv, "credits-grant-replay-operator")
	app.Auth.Operators = map[string]bool{operator.userID: true}
	grants := "/admin/credits/" + member.workspaceID + "/grants"
	body := `{"amount_credits":40,"reason":"beta reward","idempotency_key":"submission-1"}`

	code, first := operator.postJSON(t, grants, body)
	if code != http.StatusOK {
		t.Fatalf("first grant: got %d (%v)", code, first)
	}
	code, replay := operator.postJSON(t, grants, body)
	if code != http.StatusOK {
		t.Fatalf("replayed grant: got %d (%v)", code, replay)
	}
	if first["balance_credits"] != float64(40) || replay["balance_credits"] != float64(40) {
		t.Errorf("balances = %v then %v, want 40 then 40", first["balance_credits"], replay["balance_credits"])
	}
	if entries, audits := grantCounts(t, member, operator); entries != 1 || audits != 1 {
		t.Errorf("ledger entries %d and audit events %d after a replay, want 1 and 1", entries, audits)
	}
}

func TestAGrantKeyReusedForADifferentAmountIsRefusedAndChangesNothing(t *testing.T) {
	app, srv := realCreditsServer(t, creditsTestPool(t))
	member := creditsLogin(t, srv, "credits-grant-reuse-member")
	operator := creditsLogin(t, srv, "credits-grant-reuse-operator")
	app.Auth.Operators = map[string]bool{operator.userID: true}
	grants := "/admin/credits/" + member.workspaceID + "/grants"

	if code, out := operator.postJSON(t, grants, `{"amount_credits":40,"reason":"beta reward","idempotency_key":"submission-1"}`); code != http.StatusOK {
		t.Fatalf("first grant: got %d (%v)", code, out)
	}
	code, reused := operator.postJSON(t, grants, `{"amount_credits":500,"reason":"beta reward","idempotency_key":"submission-1"}`)
	if code != http.StatusConflict {
		t.Fatalf("reused key with another amount: got %d (%v), want 409", code, reused)
	}
	code, replay := operator.postJSON(t, grants, `{"amount_credits":40,"reason":"beta reward","idempotency_key":"submission-1"}`)
	if code != http.StatusOK || replay["balance_credits"] != float64(40) {
		t.Errorf("replay after the refusal: got %d (%v), want 200 with balance 40", code, replay)
	}
	if entries, audits := grantCounts(t, member, operator); entries != 1 || audits != 1 {
		t.Errorf("ledger entries %d and audit events %d, want 1 and 1", entries, audits)
	}
}

func TestGrantsWithoutAKeyOrWithDistinctKeysEachApply(t *testing.T) {
	app, srv := realCreditsServer(t, creditsTestPool(t))
	member := creditsLogin(t, srv, "credits-grant-nokey-member")
	operator := creditsLogin(t, srv, "credits-grant-nokey-operator")
	app.Auth.Operators = map[string]bool{operator.userID: true}
	grants := "/admin/credits/" + member.workspaceID + "/grants"

	for _, body := range []string{
		`{"amount_credits":10,"reason":"r"}`,
		`{"amount_credits":10,"reason":"r"}`,
		`{"amount_credits":10,"reason":"r","idempotency_key":"a"}`,
		`{"amount_credits":10,"reason":"r","idempotency_key":"b"}`,
	} {
		if code, out := operator.postJSON(t, grants, body); code != http.StatusOK {
			t.Fatalf("grant %s: got %d (%v)", body, code, out)
		}
	}
	if entries, audits := grantCounts(t, member, operator); entries != 4 || audits != 4 {
		t.Errorf("ledger entries %d and audit events %d, want 4 and 4", entries, audits)
	}
}

func TestOneGrantKeyIsScopedToTheAccountItWasUsedOn(t *testing.T) {
	app, srv := realCreditsServer(t, creditsTestPool(t))
	first := creditsLogin(t, srv, "credits-grant-scope-first")
	second := creditsLogin(t, srv, "credits-grant-scope-second")
	operator := creditsLogin(t, srv, "credits-grant-scope-operator")
	app.Auth.Operators = map[string]bool{operator.userID: true}
	body := `{"amount_credits":25,"reason":"r","idempotency_key":"shared-key"}`

	for _, member := range []*creditsClient{first, second} {
		code, out := operator.postJSON(t, "/admin/credits/"+member.workspaceID+"/grants", body)
		if code != http.StatusOK || out["balance_credits"] != float64(25) {
			t.Fatalf("grant to %s: got %d (%v), want 200 with balance 25", member.workspaceID, code, out)
		}
	}
}

func TestAGrantKeyLengthIsBoundedAtOneTo128Characters(t *testing.T) {
	app, srv := realCreditsServer(t, creditsTestPool(t))
	member := creditsLogin(t, srv, "credits-grant-keylen-member")
	operator := creditsLogin(t, srv, "credits-grant-keylen-operator")
	app.Auth.Operators = map[string]bool{operator.userID: true}
	grants := "/admin/credits/" + member.workspaceID + "/grants"

	for _, tc := range []struct {
		name string
		key  string
		want int
	}{
		{"empty", "", http.StatusBadRequest},
		{"one character", "k", http.StatusOK},
		{"128 characters", strings.Repeat("k", 127) + "x", http.StatusOK},
		{"129 characters", strings.Repeat("k", 129), http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"amount_credits":1,"reason":"r","idempotency_key":%q}`, tc.key)
			if code, out := operator.postJSON(t, grants, body); code != tc.want {
				t.Fatalf("key of %d characters: got %d (%v), want %d", len(tc.key), code, out, tc.want)
			}
		})
	}
}
