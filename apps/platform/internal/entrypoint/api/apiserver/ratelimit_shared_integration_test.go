package apiserver_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/apiserver"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

func sharedLimitStatus(h http.HandlerFunc) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/api/skills/search?q=x", nil)
	req.RemoteAddr = "192.0.2.77:4000"
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestTwoAPIInstancesSpendOneSharedBucket(t *testing.T) {
	pool := requireDB(t)
	if _, err := pool.Exec(t.Context(), `DELETE FROM rate_limit_buckets WHERE key LIKE 'shared-bucket-test|%'`); err != nil {
		t.Fatal(err)
	}
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	first := httpx.NewRateLimiter(60, 2).ShareThrough(pool).Limit("shared-bucket-test", ok)
	second := httpx.NewRateLimiter(60, 2).ShareThrough(pool).Limit("shared-bucket-test", ok)

	if sharedLimitStatus(first).Code != http.StatusOK || sharedLimitStatus(second).Code != http.StatusOK {
		t.Fatal("the burst of two was refused across two instances")
	}
	refused := sharedLimitStatus(first)
	if refused.Code != http.StatusTooManyRequests {
		t.Fatalf("the third request across two instances got %d; each instance kept its own burst", refused.Code)
	}
	if refused.Header().Get("Retry-After") == "" {
		t.Error("a shared refusal carries no Retry-After")
	}
	var tokens float64
	if err := pool.QueryRow(t.Context(), `SELECT tokens FROM rate_limit_buckets WHERE key = 'shared-bucket-test|addr:192.0.2.77'`).Scan(&tokens); err != nil {
		t.Fatal(err)
	}
	if tokens < 0 || tokens >= 1 {
		t.Fatalf("after a refusal the bucket holds %v tokens; a refusal must not spend one", tokens)
	}
}

func TestSignedInSearchersBehindOneAddressAreCountedApart(t *testing.T) {
	pool := requireDB(t)
	a := newAPITuned(t, pool, "", func(d *apiserver.Deps) {
		d.Limits = httpx.NewRateLimiter(60, 1)
	})
	alice := a.login(t, "ratelimit-office-alice")
	bob := a.login(t, "ratelimit-office-bob")
	search := func(c *client) int {
		resp, err := c.Get(c.base + "/api/skills/search?q=x")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if search(alice) == http.StatusTooManyRequests || search(bob) == http.StatusTooManyRequests {
		t.Fatal("two signed-in people at one address shared a search allowance")
	}
	if search(alice) != http.StatusTooManyRequests {
		t.Fatal("a signed-in searcher was not limited by account")
	}
}

func TestAskingTheModelToSuggestCriteriaIsLimitedPerAccount(t *testing.T) {
	pool := requireDB(t)
	a := newAPITuned(t, pool, "", func(d *apiserver.Deps) {
		d.Limits = httpx.NewRateLimiter(60, 1)
	})
	alice := a.login(t, "ratelimit-suggest-alice")
	suggest := func() int {
		resp, err := alice.Post(alice.base+"/test-cases/00000000-0000-0000-0000-000000000001/criteria/suggest", "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	if first := suggest(); first == http.StatusTooManyRequests {
		t.Fatal("the first suggestion request was already refused")
	}
	if second := suggest(); second != http.StatusTooManyRequests {
		t.Fatalf("second suggestion request = %d, want 429", second)
	}
}
