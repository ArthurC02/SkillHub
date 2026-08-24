package httpx

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

// Rate limiting for the endpoints 02:NFR-001 clause 5 names: anonymous search
// and the import endpoints (live external search is not in the MVP). Until this
// existed the platform had no rate limiter anywhere — anonymous search had been
// open and unlimited since M1, and 04 丙-54 records that the generation quota
// counter cannot double as one (a failed generation writes no row, so the
// abuse shape is invisible to it).
//
// A token bucket per client IP, in process. That bounds one API replica and not
// a fleet — the same ceiling the per-workspace generation slot accepts, for the
// same reason: there is one cmd/api today, and the durable version belongs at
// the edge. Keyed by IP rather than by session even where a session exists,
// because one mechanism that runs before authentication also shields the
// authentication path itself, and the abuse NFR-001 names is volumetric, not
// per-account.
//
// The numbers are operational tuning, not product promises: nothing displays
// them, so 04 乙-2's rule (a number on a screen is a claim) does not bite, and
// they need no ratification — only a source, which is this comment. Defaults
// are set to be invisible to a human clicking around and to matter only to a
// loop.
//
// **The key is not the client where a proxy sits in front.** RemoteAddr is
// whatever opened the TCP connection, so behind a TLS terminator every user
// shares one bucket and the whole cohort is refused together. X-Forwarded-For
// is deliberately NOT read — a header the client sets is a bucket the client
// chooses — so the correct fix is at the proxy, not here, and the release
// checklist carries it as a deployment item rather than this file pretending
// to solve it.
//
// IPv6 is keyed by /64, not by address: a single allocation hands out 2^64
// addresses, which is both a way around the limit and a way to grow the map
// without bound. /64 is the smallest block routinely assigned to one
// subscriber.
//
// ponytail: unbounded map growth across distinct /64s and IPv4 addresses. The
// endpoint this mostly protects — anonymous search — is deliberately NOT
// behind the invite list (BETA-001 gates forks, runs and downloads, not
// reading), so "a closed beta" is not the bound. Add eviction, or move the
// whole thing to the edge, before an open launch.

// RateLimiter allows `burst` immediate requests per key and refills at `rate`
// tokens per second. The zero value is not usable; use NewRateLimiter.
type RateLimiter struct {
	rate  float64
	burst float64
	mu    sync.Mutex
	last  map[string]bucket
	// now is replaceable so tests do not sleep their way through refills.
	now func() time.Time
}

type bucket struct {
	tokens float64
	at     time.Time
}

func NewRateLimiter(perMinute int, burst int) *RateLimiter {
	// A non-positive rate makes the refill wait +Inf, and time.Duration(+Inf)
	// is MinInt64 — a negative Retry-After, which RFC 9110 forbids and no
	// client can act on. Unreachable today; a floor is cheaper than the
	// investigation if it ever is not.
	if perMinute <= 0 {
		perMinute = 60
	}
	if burst <= 0 {
		burst = 1
	}
	return &RateLimiter{
		rate:  float64(perMinute) / 60.0,
		burst: float64(burst),
		last:  map[string]bucket{},
		now:   time.Now,
	}
}

// allow spends one token for key, reporting whether one was available and, if
// not, how long until one is.
func (l *RateLimiter) allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b, ok := l.last[key]
	if !ok {
		b = bucket{tokens: l.burst, at: now}
	}
	b.tokens += now.Sub(b.at).Seconds() * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.at = now
	if b.tokens < 1 {
		wait := time.Duration((1 - b.tokens) / l.rate * float64(time.Second))
		l.last[key] = b
		return false, wait
	}
	b.tokens--
	l.last[key] = b
	return true, 0
}

// Limit wraps next, refusing with 429 when the caller's IP is out of tokens.
//
// The refusal carries Retry-After and a sentence rather than a bare status:
// the honest audience for it is a script, but the person debugging that script
// reads the body. No request content is inspected and nothing is logged here —
// a limiter that logs every refusal is itself a write amplifier under the load
// it exists to shed.
func (l *RateLimiter) Limit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ok, wait := l.allow(clientKey(r.RemoteAddr))
		if !ok {
			secs := int(wait.Seconds()) + 1
			w.Header().Set("Retry-After", fmt.Sprintf("%d", secs))
			WriteError(w, http.StatusTooManyRequests,
				fmt.Sprintf("太多請求了，請在 %d 秒後再試。", secs))
			return
		}
		next(w, r)
	}
}

// clientKey is the bucket a remote address belongs to: the IPv4 address, or the
// IPv6 /64. See the package comment for why a /64 and not an address.
func clientKey(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return host
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

