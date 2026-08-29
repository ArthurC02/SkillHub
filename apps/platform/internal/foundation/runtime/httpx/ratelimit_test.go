package httpx

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	h := l.Limit("public_search", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

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

// clientKey is the bucket a request lands in, and until now it had no test at
// all: three of the four tests above hand allow() a key they made up, and the
// fourth proves only that SOME key was computed. Two mutations survived that,
// and both switch the limiter off in production while the suite stays green --
// `return remoteAddr` gives every TCP connection its own bucket, and dropping
// the /64 mask hands anybody with an IPv6 allocation 2^64 buckets, which is the
// package comment's own stated reason for the mask.
//
// No HTTP here on purpose. The keying rule is a pure function and the failure it
// has to catch is a wrong string, so a table is the whole test.
func TestClientKeyBucketsByAddressNotByConnection(t *testing.T) {
	for _, tc := range []struct {
		name string
		a, b string
		same bool
	}{
		{
			name: "two connections from one IPv4 host",
			a:    "1.2.3.4:5678", b: "1.2.3.4:9999", same: true,
		},
		{
			name: "IPv4-mapped IPv6 is the same host as its IPv4 form",
			a:    "1.2.3.4:5678", b: "[::ffff:1.2.3.4]:80", same: true,
		},
		{
			name: "two addresses inside one IPv6 /64",
			a:    "[2001:db8::1]:1", b: "[2001:db8::ffff]:1", same: true,
		},
		{
			name: "two different IPv6 /64s",
			a:    "[2001:db8::1]:1", b: "[2001:db8:0:1::1]:1", same: false,
		},
		{
			name: "two different IPv4 hosts",
			a:    "1.2.3.4:80", b: "1.2.3.5:80", same: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ka, kb := clientKey(tc.a), clientKey(tc.b)
			if (ka == kb) != tc.same {
				t.Errorf("clientKey(%q) = %q and clientKey(%q) = %q; want same=%v", tc.a, ka, tc.b, kb, tc.same)
			}
		})
	}
}

// A remote address is not user input, but it is not this package's to trust
// either: net.SplitHostPort fails on plenty of real strings (a bare address, a
// unix socket path), and a panic here is a panic on every request.
func TestClientKeySurvivesAMalformedRemoteAddress(t *testing.T) {
	for _, raw := range []string{"", "1.2.3.4", "not an address", "[::1]", "/tmp/api.sock", ":::::"} {
		if got := clientKey(raw); got == "" && raw != "" {
			t.Errorf("clientKey(%q) = %q; a bucket key of nothing merges every malformed caller into one", raw, got)
		}
	}
}

// The refusal has to be countable, because it is the only evidence that this
// protection is running at all: with RUN_QUOTA and GENERATE_QUOTA both off
// (ADR-055/056), a limiter that has been switched off and one that is refusing
func TestARefusalIsCountedOnTheMetricsSurface(t *testing.T) {
	const route = "test_route"

	l, _ := testLimiter(60, 1)
	h := l.Limit(route, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest("GET", "/api/skills/search?q=x", nil)
	req.RemoteAddr = "9.9.9.9:1234"

	h(httptest.NewRecorder(), req) // allowed
	if got := scrape(t, route); got != "" {
		t.Fatalf("an allowed request already published %q; only a 429 may count", got)
	}
	h(httptest.NewRecorder(), req) // refused
	if got := scrape(t, route); got != `skillhub_rate_limited_total{route="test_route"} 1` {
		t.Errorf("after one 429 /metrics has %q, want the route's counter at 1", got)
	}
}

// scrape returns the one skillhub_rate_limited_total line carrying route, or "".
func scrape(t *testing.T, route string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if strings.HasPrefix(line, "skillhub_rate_limited_total{") && strings.Contains(line, `"`+route+`"`) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}
