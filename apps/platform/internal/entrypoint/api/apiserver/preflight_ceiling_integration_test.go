package apiserver_test

import (
	"fmt"
	"net/http"
	"testing"
)

func preflightCeilingAndBlock(t *testing.T, f fixture) (ceiling int64, blocked string) {
	t.Helper()
	code, body := f.doJSON(t, http.MethodGet, "/skills/"+f.skillID+"/runs/preflight?version_id="+f.versionID+"&test_case_id="+f.testCaseID, "")
	if code != http.StatusOK {
		t.Fatalf("preflight: %d %v", code, body)
	}
	raw, ok := body["ceiling_credits"].(float64)
	if !ok {
		t.Fatalf("preflight carries no ceiling_credits: %v", body["ceiling_credits"])
	}
	blocked, _ = body["blocked"].(string)
	return int64(raw), blocked
}

func TestThePreflightCeilingIsTheBalanceTheCreditCheckStartsAt(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, freshName("alice-preflight-ceiling"))
	code, credits := f.doJSON(t, http.MethodGet, "/me/credits", "")
	if code != http.StatusOK {
		t.Fatalf("GET /me/credits: %d %v", code, credits)
	}
	floor := int64(credits["debt_floor_credits"].(float64))

	ceiling, _ := preflightCeilingAndBlock(t, f)
	if ceiling <= 0 {
		t.Fatalf("ceiling_credits = %d, want a positive ceiling", ceiling)
	}
	for _, tc := range []struct {
		balance int64
		want    string
	}{
		{ceiling + floor, ""},
		{ceiling + floor - 1, "credit_balance"},
	} {
		t.Run(fmt.Sprintf("balance %d", tc.balance), func(t *testing.T) {
			setOwnerBalance(t, pool, f.userID, tc.balance)
			got, blocked := preflightCeilingAndBlock(t, f)
			if got != ceiling || blocked != tc.want {
				t.Errorf("ceiling %d blocked %q, want ceiling %d blocked %q", got, blocked, ceiling, tc.want)
			}
		})
	}
}
