package httpx

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// The limiter is NFR-001 clause 5's protection, so what the tests pin is the
// protective shape: a burst is allowed, the burst end is refused with a usable
// Retry-After, time restores service, and one abuser does not starve a
// neighbour. The clock is injected — a limiter test that sleeps is a flaky
// test about scheduling, not a test about limiting.

func testLimiter(perMinute, burst int) (*RateLimiter, *time.Time) {
	l := NewRateLimiter(perMinute, burst)
	now := time.Unix(1_700_000_000, 0)
	l.now = func() time.Time { return now }
	return l, &now
}

func TestTheBurstIsAllowedAndTheBurstEndIsRefused(t *testing.T) {
	l, _ := testLimiter(60, 3)
	for i := 0; i < 3; i++ {
		if ok, _ := l.allow("1.2.3.4"); !ok {
			t.Fatalf("request %d of the burst was refused", i+1)
		}
	}
	ok, wait := l.allow("1.2.3.4")
	if ok {
		t.Fatal("the request past the burst was allowed")
	}
	if wait <= 0 {
		t.Fatalf("no wait hint: %v", wait)
	}
}

func TestTimeRestoresService(t *testing.T) {
	l, now := testLimiter(60, 1) // one token, one per second
	if ok, _ := l.allow("k"); !ok {
		t.Fatal("first request refused")
	}
	if ok, _ := l.allow("k"); ok {
		t.Fatal("second immediate request allowed")
	}
	*now = now.Add(1100 * time.Millisecond)
	if ok, _ := l.allow("k"); !ok {
		t.Fatal("a second later the caller is still refused; the bucket does not refill")
	}
}

func TestOneAbuserDoesNotStarveANeighbour(t *testing.T) {
	l, _ := testLimiter(60, 2)
	l.allow("abuser")
	l.allow("abuser")
	if ok, _ := l.allow("abuser"); ok {
		t.Fatal("the abuser was not refused")
	}
	if ok, _ := l.allow("neighbour"); !ok {
		t.Fatal("the neighbour paid for the abuser's traffic")
	}
}

func TestTheRefusalCarriesRetryAfterAndASentence(t *testing.T) {
	l, _ := testLimiter(60, 1)
	h := l.Limit(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	req := httptest.NewRequest("GET", "/api/skills/search?q=x", nil)
	req.RemoteAddr = "9.9.9.9:1234"
	first := httptest.NewRecorder()
	h(first, req)
	if first.Code != http.StatusOK {
		t.Fatalf("first request: %d", first.Code)
	}
	second := httptest.NewRecorder()
	h(second, req)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: %d, want 429", second.Code)
	}
	if s, err := strconv.Atoi(second.Header().Get("Retry-After")); err != nil || s < 1 {
		t.Errorf("Retry-After = %q; a refusal without a time is the version nobody can act on", second.Header().Get("Retry-After"))
	}
	if second.Body.Len() == 0 {
		t.Error("a bare 429 with no sentence")
	}
}
