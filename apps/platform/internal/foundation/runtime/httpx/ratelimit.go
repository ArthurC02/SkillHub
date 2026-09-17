package httpx

import (
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
)

type RateLimiter struct {
	rate  float64
	burst float64
	mu    sync.Mutex
	last  map[string]bucket

	trustedProxies []netip.Prefix

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
		ok, wait := l.allow(clientKey(l.clientAddress(r)))
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

func ParseTrustedProxies(raw string) ([]netip.Prefix, error) {
	var prefixes []netip.Prefix
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if addr, err := netip.ParseAddr(entry); err == nil {
			addr = addr.Unmap()
			prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
			continue
		}
		prefix, err := netip.ParsePrefix(entry)
		if err != nil {
			return nil, fmt.Errorf("TRUSTED_PROXIES entry %q is neither an address nor a CIDR prefix", entry)
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}

func (l *RateLimiter) TrustProxies(prefixes []netip.Prefix) *RateLimiter {
	l.trustedProxies = prefixes
	return l
}

func (l *RateLimiter) clientAddress(r *http.Request) string {
	client := hostOf(r.RemoteAddr)
	if !l.isTrustedProxy(client) {
		return client
	}
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		hop := strings.TrimSpace(hops[i])
		if _, err := netip.ParseAddr(hop); err != nil {
			break
		}
		client = hop
		if !l.isTrustedProxy(hop) {
			break
		}
	}
	return client
}

func (l *RateLimiter) isTrustedProxy(host string) bool {
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, prefix := range l.trustedProxies {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func hostOf(remoteAddr string) string {
	if addrPort, err := netip.ParseAddrPort(remoteAddr); err == nil {
		return addrPort.Addr().String()
	}
	return remoteAddr
}

// clientKey masks an IPv6 address to its /64 rather than keying on the full
// address: a single allocation hands out 2^64 addresses, so keying on the
// full address would let one allocation dodge the limit entirely.
func clientKey(address string) string {
	addr, err := netip.ParseAddr(hostOf(address))
	if err != nil {
		return address
	}
	addr = addr.Unmap()
	if addr.Is4() {
		return addr.String()
	}
	return netip.PrefixFrom(addr, 64).Masked().String()
}
