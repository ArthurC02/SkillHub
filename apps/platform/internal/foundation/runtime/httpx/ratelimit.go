package httpx

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
)

type RateLimiter struct {
	rate  float64
	burst float64
	mu    sync.Mutex
	last  map[string]bucket

	now func() time.Time
}

type bucket struct {
	tokens float64
	at     time.Time
}

func NewRateLimiter(perMinute int, burst int) *RateLimiter {

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

func (l *RateLimiter) Limit(route string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ok, wait := l.allow(clientKey(r.RemoteAddr))
		if !ok {
			metrics.RateLimited.WithLabelValues(route).Inc()
			secs := int(wait.Seconds()) + 1
			w.Header().Set("Retry-After", fmt.Sprintf("%d", secs))
			WriteError(w, http.StatusTooManyRequests,
				fmt.Sprintf("太多請求了，請在 %d 秒後再試。", secs))
			return
		}
		next(w, r)
	}
}

// clientKey masks an IPv6 address to its /64 rather than keying on the full
// address: a single allocation hands out 2^64 addresses, so keying on the
// full address would let one allocation dodge the limit entirely.
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
