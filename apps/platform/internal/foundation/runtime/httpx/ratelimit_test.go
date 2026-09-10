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
	l, now := testLimiter(60, 1)
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

func TestClientKeySurvivesAMalformedRemoteAddress(t *testing.T) {
	for _, raw := range []string{"", "1.2.3.4", "not an address", "[::1]", "/tmp/api.sock", ":::::"} {
		if got := clientKey(raw); got == "" && raw != "" {
			t.Errorf("clientKey(%q) = %q; a bucket key of nothing merges every malformed caller into one", raw, got)
		}
	}
}

func TestARefusalIsCountedOnTheMetricsSurface(t *testing.T) {
	const route = "test_route"

	l, _ := testLimiter(60, 1)
	h := l.Limit(route, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest("GET", "/api/skills/search?q=x", nil)
	req.RemoteAddr = "9.9.9.9:1234"

	h(httptest.NewRecorder(), req)
	if got := scrape(t, route); got != "" {
		t.Fatalf("an allowed request already published %q; only a 429 may count", got)
	}
	h(httptest.NewRecorder(), req)
	if got := scrape(t, route); got != `skillhub_rate_limited_total{route="test_route"} 1` {
		t.Errorf("after one 429 /metrics has %q, want the route's counter at 1", got)
	}
}

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
