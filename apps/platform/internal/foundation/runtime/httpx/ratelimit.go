package httpx

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type RateLimiter struct {
	rate      float64
	burst     float64
	refill    time.Duration
	mu        sync.Mutex
	last      map[string]bucket
	nextSweep time.Time

	trustedProxies []netip.Prefix

	shared func(ctx context.Context, key string) (bool, time.Duration, error)
	sweep  func(ctx context.Context) error

	nextSharedSweep time.Time

	now func() time.Time
}

const (
	sharedTakeTimeout   = time.Second
	sharedSweepEvery    = 10 * time.Minute
	sharedSweepIdle     = time.Hour
	sharedSweepDeadline = 30 * time.Second
)

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
	rate := float64(perMinute) / time.Minute.Seconds()
	return &RateLimiter{
		rate:   rate,
		burst:  float64(burst),
		refill: time.Duration(float64(burst) / rate * float64(time.Second)),
		last:   map[string]bucket{},
		now:    time.Now,
	}
}

func (l *RateLimiter) forgetFullBuckets(now time.Time) {
	if now.Before(l.nextSweep) {
		return
	}
	for key, b := range l.last {
		if now.Sub(b.at) >= l.refill {
			delete(l.last, key)
		}
	}
	l.nextSweep = now.Add(l.refill)
}

func (l *RateLimiter) allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.forgetFullBuckets(now)
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

func (l *RateLimiter) ShareThrough(pool *pgxpool.Pool) *RateLimiter {
	q := gen.New(pool)
	l.shared = func(ctx context.Context, key string) (bool, time.Duration, error) {
		row, err := q.TakeRateLimitToken(ctx, gen.TakeRateLimitTokenParams{Key: key, Burst: l.burst, Rate: l.rate})
		if err != nil || row.Allowed {
			return row.Allowed, 0, err
		}
		return false, time.Duration((1 - row.Tokens) / l.rate * float64(time.Second)), nil
	}
	l.sweep = func(ctx context.Context) error {
		_, err := q.SweepRateLimitBuckets(ctx, pgconv.Interval(sharedSweepIdle))
		return err
	}
	return l
}

func (l *RateLimiter) subject(r *http.Request, account func(*http.Request) string) string {
	if account != nil {
		if id := account(r); id != "" {
			return "account:" + id
		}
	}
	return "addr:" + clientKey(l.clientAddress(r))
}

func (l *RateLimiter) take(ctx context.Context, key string) (bool, time.Duration) {
	if l.shared == nil {
		return l.allow(key)
	}
	l.sweepSharedWhenDue()
	ctx, cancel := context.WithTimeout(ctx, sharedTakeTimeout)
	defer cancel()
	ok, wait, err := l.shared(ctx, key)
	if err != nil {
		slog.Warn("rate limit: shared buckets unreachable; this process counts on its own", "error", err)
		return l.allow(key)
	}
	return ok, wait
}

func (l *RateLimiter) sweepSharedWhenDue() {
	l.mu.Lock()
	now := l.now()
	due := !now.Before(l.nextSharedSweep)
	if due {
		l.nextSharedSweep = now.Add(sharedSweepEvery)
	}
	l.mu.Unlock()
	if !due {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), sharedSweepDeadline)
		defer cancel()
		if err := l.sweep(ctx); err != nil {
			slog.Warn("rate limit: idle shared buckets not swept", "error", err)
		}
	}()
}

func (l *RateLimiter) Limit(route string, next http.HandlerFunc) http.HandlerFunc {
	return l.LimitBy(route, nil, next)
}

func (l *RateLimiter) LimitBy(route string, account func(*http.Request) string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ok, wait := l.take(r.Context(), route+"|"+l.subject(r, account))
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
	return netip.PrefixFrom(addr, ipv6AllocationPrefixBits).Masked().String()
}

const ipv6AllocationPrefixBits = 64
