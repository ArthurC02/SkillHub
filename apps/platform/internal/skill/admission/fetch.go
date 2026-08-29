package ingest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

// ErrFetch marks URL-import failures the client can fix: disallowed host,
// unreachable URL, oversized package.
var ErrFetch = errors.New("fetch failed")

// URLFetcher downloads skill packages from allow-listed hosts only (INGEST-001,
// SEC-003). Two independent checks, because the host name and the address it
// resolves to are two different facts: the allow list decides which names may be
// asked for, and [blockedAddr] decides which addresses may be connected to.
type URLFetcher struct {
	// Allowed hosts (exact match, lowercase, host[:port]).
	Allowed map[string]bool
	// AllowInsecure is the development switch (IMPORT_ALLOW_INSECURE): it permits
	// plain http, and with it the loopback and RFC1918 addresses that httptest and
	// the compose stack live on. Never production. The link-local, CGNAT,
	// broadcast, multicast and unspecified ranges stay blocked even here — a
	// development deployment has no reason to reach a metadata service either.
	AllowInsecure bool
}

// Fetch's three configured values (SEC-003 大小與逾時). They are numbers, not
// rules: changing one does not change what is checked.
const (
	connectTimeout = 10 * time.Second
	fetchTimeout   = 60 * time.Second
	maxRedirects   = 3
)

// Neither of these reaches a caller as written. Their text names the address or
// the host that was refused, and SEC-003 says the caller gets the category and
// not the address; [classify] does that swap.
var (
	errBlockedDestination = errors.New("destination address is not allowed")
	errTooManyRedirects   = errors.New("too many redirects")
)

// alwaysBlocked are the ranges no deployment may reach, development included.
// The link-local case (169.254.169.254 and friends) is handled by
// netip's IsLinkLocalUnicast; these are the ones it has no method for.
var alwaysBlocked = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),      // CGNAT
	netip.MustParsePrefix("0.0.0.0/8"),          // "this network"
	netip.MustParsePrefix("255.255.255.255/32"), // broadcast
}

// blockedAddr answers whether one resolved address may be connected to.
//
// The v4-mapped form is unmapped first: ::ffff:169.254.169.254 and
// 169.254.169.254 are one host and must get one answer. netip's Is* predicates
// see through the mapping on their own, but netip.Prefix.Contains does not — it
// is family-strict, so a mapped 100.64.0.1 is in no v4 prefix at all, and the v6
// spelling of a v4 rule would slide straight past the list below.
func blockedAddr(ip netip.Addr, dev bool) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return true
	}
	for _, p := range alwaysBlocked {
		if p.Contains(ip) {
			return true
		}
	}
	if dev {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() // 127/8, ::1, RFC1918, fc00::/7
}

// newClient builds the client the fetch runs on. The whole SSRF story lives in
// the dialler's Control hook: it runs after resolution and before connect, and
// is handed the concrete address about to be dialled. Refusing there closes both
// halves of the problem at once — a name that resolves to an internal address is
// refused, and a second lookup cannot substitute a different answer, because the
// address that was checked is the address that gets connected (DNS rebinding).
// Every redirect hop dials again and is therefore checked again.
//
// Proxy is nil on purpose. Through a proxy the dialled address is the proxy's,
// and the check would be inspecting a host nobody asked about.
func newClient(dev bool) *http.Client {
	d := &net.Dialer{
		Timeout:   connectTimeout,
		KeepAlive: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil || blockedAddr(ap.Addr(), dev) {
				return errBlockedDestination // fail-closed: unparseable is not cleared
			}
			return nil
		},
	}
	return &http.Client{
		Timeout: fetchTimeout,
		Transport: &http.Transport{
			DialContext:           d.DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConnsPerHost:   2,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   connectTimeout,
			ExpectContinueTimeout: time.Second,
		},
	}
}

var (
	strictClient = newClient(false)
	devClient    = newClient(true)
)

// DefaultAllowedHosts covers GitHub repo and archive downloads (PDM-002: the
// first-batch skill source is GitHub).
func DefaultAllowedHosts() map[string]bool {
	return map[string]bool{
		"github.com":                    true,
		"codeload.github.com":           true,
		"objects.githubusercontent.com": true,
	}
}

func (f *URLFetcher) client() *http.Client {
	if f.AllowInsecure {
		return devClient
	}
	return strictClient
}

// do runs one request under the redirect budget and the per-hop scheme and host
// re-check. The per-hop address re-check is the dialler's (see newClient).
func (f *URLFetcher) do(req *http.Request) (*http.Response, error) {
	client := *f.client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		// via holds the hops already made, so this follows maxRedirects of them
		// and refuses the one after.
		if len(via) > maxRedirects {
			return errTooManyRedirects
		}
		return f.checkURL(req.URL)
	}
	resp, err := client.Do(req)
	// SEC-003 可稽核: the host finally dialled — the last hop, not the one the user
	// typed — and whether it was accepted. Host only: no path, no query, and by
	// Normalize no credentials to leak.
	host := req.URL.Host
	if resp != nil && resp.Request != nil {
		host = resp.Request.URL.Host
	}
	slog.Info("import fetch", "host", host, "ok", err == nil)
	if err != nil {
		return nil, classify(err)
	}
	return resp, nil
}

// classify maps a transport failure onto one of the reason categories SEC-003
// permits us to report. The raw error is dropped rather than wrapped: it names
// the address the dialler resolved, and that address is exactly what must not
// travel back to the caller.
func classify(err error) error {
	var ne net.Error
	switch {
	case errors.Is(err, errTooManyRedirects):
		return fmt.Errorf("%w: too many redirects", ErrFetch)
	case errors.Is(err, errBlockedDestination), errors.Is(err, ErrFetch):
		// An ErrFetch out here came from CheckRedirect, so its text describes a
		// redirect target the caller did not choose and may not be told about.
		return fmt.Errorf("%w: destination is not allowed", ErrFetch)
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return fmt.Errorf("%w: timed out", ErrFetch)
	}
	return fmt.Errorf("%w: source could not be fetched", ErrFetch)
}

func (f *URLFetcher) checkURL(u *url.URL) error {
	if u.Scheme != "https" && (!f.AllowInsecure || u.Scheme != "http") {
		return fmt.Errorf("%w: only https URLs are supported", ErrFetch)
	}
	if !f.Allowed[strings.ToLower(u.Host)] {
		// This one names the host, and reaches the caller verbatim through
		// respond(). That is deliberate and it is not the thing SEC-003 forbids:
		// the rule is that an error must not disclose an address the PLATFORM
		// resolved, and this host is the caller's own input, echoed back before
		// any DNS happened. Telling them which host was refused is the whole
		// difference between a fixable refusal and a mystery. The resolved
		// addresses are handled the other way -- see classify(), which swaps them
		// for a category precisely so the internal network never leaks.
		return fmt.Errorf("%w: host %q is not on the allowed source list", ErrFetch, u.Host)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%w: source URLs cannot contain credentials, query parameters, or fragments", ErrFetch)
	}
	return nil
}

// Normalize validates a user-supplied source URL and returns the credential-free
// canonical provenance that may be persisted, logged, or put in a manifest.
func (f *URLFetcher) Normalize(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("%w: invalid URL", ErrFetch)
	}
	if err := f.checkURL(u); err != nil {
		return "", err
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	return u.String(), nil
}

// Fetch downloads rawURL and returns the package bytes plus the git ref when
// it can be derived from the URL. GitHub repo pages are normalized to their
// codeload zip archives; anything else must already point at a zip.
func (f *URLFetcher) Fetch(ctx context.Context, rawURL string) (data []byte, ref string, err error) {
	normalized, err := f.Normalize(rawURL)
	if err != nil {
		return nil, "", err
	}
	u, _ := url.Parse(normalized)

	candidates, ref := f.candidates(u)
	var lastErr error
	for _, c := range candidates {
		data, err := f.download(ctx, c.url)
		if err == nil {
			if c.ref != "" {
				ref = c.ref
			}
			return data, ref, nil
		}
		lastErr = err
	}
	return nil, "", lastErr
}

type candidate struct{ url, ref string }

// commitSHA is a full 40-hex git object name. Only the full form is accepted:
// an abbreviated SHA is not a stable identifier, and INGEST-004 records this
// value as the source ref that a later audit is expected to resolve.
var commitSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// candidates expands a GitHub repo URL into archive URLs to try in order;
// non-repo URLs pass through unchanged.
// ponytail: main-then-master probing instead of a GitHub API call for the
// default branch; switch to the API if unauthenticated rate limits ever bite.
func (f *URLFetcher) candidates(u *url.URL) ([]candidate, string) {
	if strings.ToLower(u.Host) != "github.com" {
		return []candidate{{url: u.String()}}, ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	zipAt := func(owner, repo, ref string) string {
		return fmt.Sprintf("https://codeload.github.com/%s/%s/zip/%s", owner, repo, ref)
	}
	archive := func(owner, repo, kind, ref string) string {
		return zipAt(owner, repo, "refs/"+kind+"/"+ref)
	}
	switch {
	case len(parts) == 2: // github.com/owner/repo
		return []candidate{
			{archive(parts[0], parts[1], "heads", "main"), "main"},
			{archive(parts[0], parts[1], "heads", "master"), "master"},
		}, ""
	// github.com/owner/repo/tree|commit/<ref>. A 40-hex ref is a commit, and
	// codeload serves an archive at a bare SHA — that is the only input shape
	// that can pin an import to an immutable point in history, so source_ref
	// records the SHA rather than a branch name that moves under it
	// (INGEST-004, import-report.md §6.1 bug 4).
	case len(parts) >= 4 && (parts[2] == "tree" || parts[2] == "commit"):
		ref := strings.Join(parts[3:], "/")
		if commitSHA.MatchString(ref) {
			return []candidate{{zipAt(parts[0], parts[1], ref), ref}}, ""
		}
		return []candidate{{archive(parts[0], parts[1], "heads", ref), ref}}, ""
	default: // e.g. github.com/owner/repo/archive/... — direct file URL
		return []candidate{{url: u.String()}}, ""
	}
}

func (f *URLFetcher) download(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid URL", ErrFetch)
	}
	// The allow list decides which names may be ASKED FOR (02:SEC-003), and until
	// 2026-08-29 it only ever saw the URL the user typed. do() re-checks redirect
	// hops but not the URL it is handed, and candidates() rewrites a github.com
	// repo URL into a codeload.github.com archive URL from a constant string — so a
	// deployment that narrowed the list to github.com alone still sent requests to
	// codeload. Not a hole today (the dialler's address check is unchanged and the
	// target is a constant), but it meant the list was no longer the only authority
	// on outbound hosts, which is the entire point of having one.
	//
	// Here rather than in candidates(): every URL this fetcher requests goes through
	// this function, so one check covers the rewrite and whatever the next one is.
	if err := f.checkURL(req.URL); err != nil {
		return nil, err
	}
	resp, err := f.do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: source returned status %d", ErrFetch, resp.StatusCode)
	}

	// Counted while reading, not after: LimitReader stops one byte past the cap,
	// so an oversized source is abandoned mid-download (SEC-003 邊讀邊累計).
	data, err := io.ReadAll(io.LimitReader(resp.Body, skillpkg.MaxZipBytes+1))
	if err != nil {
		return nil, classify(err)
	}
	if len(data) > skillpkg.MaxZipBytes {
		// The ceiling and no actual size, and that is the honest pair here
		// (03:INGEST-016): LimitReader abandoned the download one byte past the
		// cap, so nobody in this process knows how big the source really was.
		// Content-Length is the remote's claim about bytes we chose not to read.
		metrics.PackageSizeRefused.WithLabelValues(metrics.CeilingURL).Inc()
		return nil, fmt.Errorf("%w: package from this source is over the %s import limit",
			ErrFetch, skillpkg.HumanMB(skillpkg.MaxZipBytes))
	}
	return data, nil
}
