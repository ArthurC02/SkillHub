package ingest

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

func TestFetchRejectsDisallowedHost(t *testing.T) {
	f := &URLFetcher{Allowed: map[string]bool{"github.com": true}}
	for _, u := range []string{
		"https://evil.example.com/pkg.zip",
		"https://localhost:8080/internal.zip",
		"http://github.com/o/r", // https required without AllowInsecure
		"://bad",
	} {
		if _, _, err := f.Fetch(context.Background(), u); !errors.Is(err, ErrFetch) {
			t.Fatalf("%s: want ErrFetch, got %v", u, err)
		}
	}
}

func TestFetchRejectsURLsThatCouldLeakCredentials(t *testing.T) {
	f := &URLFetcher{Allowed: map[string]bool{"github.com": true}}
	for _, raw := range []string{
		"https://user:secret@github.com/o/r",
		"https://github.com/o/r?token=secret",
		"https://github.com/o/r#secret",
	} {
		if _, _, err := f.Fetch(context.Background(), raw); !errors.Is(err, ErrFetch) {
			t.Fatalf("%s: want ErrFetch, got %v", raw, err)
		} else if strings.Contains(err.Error(), "secret") {
			t.Fatalf("error leaked credential: %v", err)
		}
	}
}

func TestNormalizeReturnsSafeCanonicalProvenance(t *testing.T) {
	f := &URLFetcher{Allowed: map[string]bool{"github.com": true}}
	got, err := f.Normalize("HTTPS://GitHub.com/o/r")
	if err != nil || got != "https://github.com/o/r" {
		t.Fatalf("Normalize = %q, %v", got, err)
	}
}

func TestFetchRedirectOffAllowListBlocked(t *testing.T) {
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("redirect target must never be reached")
	}))
	defer evil.Close()
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evil.URL+"/pkg.zip", http.StatusFound)
	}))
	defer src.Close()

	srcHost := strings.TrimPrefix(src.URL, "http://")
	f := &URLFetcher{Allowed: map[string]bool{srcHost: true}, AllowInsecure: true}
	if _, _, err := f.Fetch(context.Background(), src.URL+"/pkg.zip"); !errors.Is(err, ErrFetch) {
		t.Fatalf("want ErrFetch on off-list redirect, got %v", err)
	}
}

func TestFetchSizeCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, skillpkg.MaxZipBytes+1))
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	f := &URLFetcher{Allowed: map[string]bool{host: true}, AllowInsecure: true}
	before := refusalCount(t, metrics.CeilingURL)
	_, _, err := f.Fetch(context.Background(), srv.URL+"/big.zip")
	if !errors.Is(err, ErrFetch) {
		t.Fatalf("want ErrFetch for oversized package, got %v", err)
	}
	// 03:INGEST-016. The ceiling in the message, because this error reaches the
	// creator verbatim (Handler.respond writes err.Error() for ErrFetch); and no
	// actual size, because LimitReader abandoned the download one byte past the
	// cap and nobody here knows how big the source was.
	if !strings.Contains(err.Error(), skillpkg.HumanMB(skillpkg.MaxZipBytes)) {
		t.Errorf("the refusal does not name the import ceiling: %v", err)
	}
	if got := refusalCount(t, metrics.CeilingURL) - before; got != 1 {
		t.Errorf("url-fetch size refusals counted %v times, want 1", got)
	}
}

func TestFetchDownloadsFromAllowedHost(t *testing.T) {
	payload := []byte("zip-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	f := &URLFetcher{Allowed: map[string]bool{host: true}, AllowInsecure: true}
	data, ref, err := f.Fetch(context.Background(), srv.URL+"/pkg.zip")
	if err != nil || string(data) != string(payload) || ref != "" {
		t.Fatalf("got %q ref=%q err=%v", data, ref, err)
	}
}

func TestFetchDrainsOrdinaryHTTPErrorForConnectionReuse(t *testing.T) {
	var requests, connections atomic.Int32
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(strings.Repeat("x", 512<<10)))
			return
		}
		_, _ = w.Write([]byte("zip-bytes"))
	}))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	srv.Start()
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	f := &URLFetcher{Allowed: map[string]bool{host: true}, AllowInsecure: true}
	if _, _, err := f.Fetch(context.Background(), srv.URL+"/pkg.zip"); !errors.Is(err, ErrFetch) {
		t.Fatalf("first fetch returned %v, want ErrFetch", err)
	}
	if data, _, err := f.Fetch(context.Background(), srv.URL+"/pkg.zip"); err != nil || string(data) != "zip-bytes" {
		t.Fatalf("second fetch = %q, %v", data, err)
	}
	if got := connections.Load(); got != 1 {
		t.Fatalf("ordinary error body prevented connection reuse: opened %d connections", got)
	}
}

func TestGitHubURLNormalization(t *testing.T) {
	f := &URLFetcher{Allowed: DefaultAllowedHosts()}
	parse := func(s string) *url.URL {
		u, err := url.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}

	cands, _ := f.candidates(parse("https://github.com/anthropics/skills"))
	if len(cands) != 2 ||
		cands[0].url != "https://codeload.github.com/anthropics/skills/zip/refs/heads/main" ||
		cands[0].ref != "main" || cands[1].ref != "master" {
		t.Fatalf("repo URL candidates wrong: %+v", cands)
	}

	cands, _ = f.candidates(parse("https://github.com/anthropics/skills/tree/dev"))
	if len(cands) != 1 ||
		cands[0].url != "https://codeload.github.com/anthropics/skills/zip/refs/heads/dev" ||
		cands[0].ref != "dev" {
		t.Fatalf("tree URL candidate wrong: %+v", cands)
	}

	// INGEST-004 / import-report.md §6.1 bug 4: a 40-hex ref is a commit, and
	// codeload serves the archive at a bare SHA. This is the only URL shape that
	// pins an import to immutable content, so source_ref must record the SHA.
	sha := "0123456789abcdef0123456789abcdef01234567"
	for _, path := range []string{"tree", "commit"} {
		cands, _ = f.candidates(parse("https://github.com/anthropics/skills/" + path + "/" + sha))
		if len(cands) != 1 ||
			cands[0].url != "https://codeload.github.com/anthropics/skills/zip/"+sha ||
			cands[0].ref != sha {
			t.Fatalf("%s/<sha> must fetch and record the commit: %+v", path, cands)
		}
	}

	// A short SHA is not a stable identifier and must not be treated as one.
	cands, _ = f.candidates(parse("https://github.com/anthropics/skills/tree/0123456"))
	if len(cands) != 1 || cands[0].url != "https://codeload.github.com/anthropics/skills/zip/refs/heads/0123456" {
		t.Fatalf("abbreviated sha must stay on the branch path: %+v", cands)
	}

	cands, _ = f.candidates(parse("https://codeload.github.com/o/r/zip/refs/heads/main"))
	if len(cands) != 1 || cands[0].url != "https://codeload.github.com/o/r/zip/refs/heads/main" {
		t.Fatalf("non-repo URL must pass through: %+v", cands)
	}
}

// INGEST-014 / SEC-003: the allow list decides which host names may be asked
// for; this decides which addresses may be connected to. Both spellings of the
// metadata address answer to the same rule — netip's Is* methods say false for
// every v4-mapped address, so blockedAddr unmaps before it asks.
func TestBlockedAddrByFamily(t *testing.T) {
	for _, tc := range []struct {
		addr        string
		strict, dev bool
	}{
		// Blocked everywhere, development included.
		{"169.254.169.254", true, true},        // cloud metadata service
		{"::ffff:169.254.169.254", true, true}, // the same host, spelled v6
		{"fe80::1", true, true},                // v6 link-local
		{"224.0.0.1", true, true},              // v4 multicast
		{"ff02::1", true, true},                // v6 link-local multicast
		{"ff01::1", true, true},                // v6 interface-local multicast
		{"100.64.0.1", true, true},             // CGNAT
		{"::ffff:100.64.0.1", true, true},      // and its v6 spelling
		{"0.0.0.0", true, true},                // unspecified / "this network"
		{"::", true, true},                     // v6 unspecified
		{"0.1.2.3", true, true},                // 0.0.0.0/8
		{"::ffff:0.1.2.3", true, true},
		{"255.255.255.255", true, true}, // broadcast
		{"::ffff:255.255.255.255", true, true},
		// Local network: refused in a real deployment, reachable for httptest and
		// the compose stack when IMPORT_ALLOW_INSECURE is on.
		{"127.0.0.1", true, false},
		{"::1", true, false},
		{"::ffff:127.0.0.1", true, false},
		{"10.0.0.5", true, false},
		{"::ffff:10.0.0.5", true, false},
		{"172.16.0.1", true, false},
		{"192.168.1.1", true, false},
		{"fd00::1", true, false}, // unique-local v6
		// A normal external host is reachable in both modes.
		{"140.82.121.4", false, false},
		{"2606:2800:220:1:248:1893:25c8:1946", false, false},
	} {
		ip, err := netip.ParseAddr(tc.addr)
		if err != nil {
			t.Fatalf("%s: %v", tc.addr, err)
		}
		if got := blockedAddr(ip, false); got != tc.strict {
			t.Errorf("blockedAddr(%s, strict) = %v, want %v", tc.addr, got, tc.strict)
		}
		if got := blockedAddr(ip, true); got != tc.dev {
			t.Errorf("blockedAddr(%s, dev) = %v, want %v", tc.addr, got, tc.dev)
		}
	}
}

// wantBlocked asserts the fetch was refused for the destination and that the
// refusal says so without naming an address (SEC-003 錯誤訊息不洩漏內部資訊).
func wantBlocked(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrFetch) {
		t.Fatalf("want ErrFetch, got %v", err)
	}
	if got, want := err.Error(), "fetch failed: destination is not allowed"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

// A host name that resolves to loopback is refused after resolution and before
// connect. Port 9 is closed: without the dial guard this fails too, but with a
// connection error, which is why the assertion is on the category and not just
// on ErrFetch.
func TestFetchRefusesHostResolvingToLoopback(t *testing.T) {
	f := &URLFetcher{Allowed: map[string]bool{"localhost:9": true}}
	_, _, err := f.Fetch(context.Background(), "https://localhost:9/pkg.zip")
	wantBlocked(t, err)
	for _, leak := range []string{"127.0.0.1", "::1"} {
		if strings.Contains(err.Error(), leak) {
			t.Fatalf("error leaked the resolved address: %v", err)
		}
	}
}

// The metadata address is refused as an IP literal in both spellings. Neither
// reaches the network: the refusal happens in the dialler's Control hook.
func TestFetchRefusesMetadataAddressBothSpellings(t *testing.T) {
	for _, host := range []string{"169.254.169.254", "[::ffff:169.254.169.254]"} {
		f := &URLFetcher{Allowed: map[string]bool{host: true}}
		_, _, err := f.Fetch(context.Background(), "https://"+host+"/latest/meta-data")
		wantBlocked(t, err)
	}
}

// A redirect into a blocked address is refused even though the redirect target
// is on the host allow list — checking the original URL alone would have
// followed it. Development mode, so the loopback source server is reachable and
// the link-local target is still not.
func TestFetchRefusesRedirectIntoBlockedAddress(t *testing.T) {
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data", http.StatusFound)
	}))
	defer src.Close()
	srcHost := strings.TrimPrefix(src.URL, "http://")

	f := &URLFetcher{
		Allowed:       map[string]bool{srcHost: true, "169.254.169.254": true},
		AllowInsecure: true,
	}
	_, _, err := f.Fetch(context.Background(), src.URL+"/pkg.zip")
	wantBlocked(t, err)
}

// Four hops offered, three followed (SEC-003 redirect 上限 3 跳).
func TestFetchRedirectLimit(t *testing.T) {
	var hops int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops++
		http.Redirect(w, r, "/next", http.StatusFound)
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	f := &URLFetcher{Allowed: map[string]bool{host: true}, AllowInsecure: true}
	_, _, err := f.Fetch(context.Background(), srv.URL+"/pkg.zip")
	if err == nil || err.Error() != "fetch failed: too many redirects" {
		t.Fatalf("want the redirect budget to stop this, got %v", err)
	}
	if hops != 1+maxRedirects {
		t.Fatalf("server saw %d requests, want %d", hops, 1+maxRedirects)
	}
}

// SEC-003's model is that the allow list decides which names may be asked for.
// candidates() rewrites github.com/owner/repo into a codeload.github.com archive
// URL from a constant string, and download() used to send that without checking
// it: only the URL the user typed ever met the list. DefaultAllowedHosts happens
// to contain codeload, so a default deployment could not see the difference — a
// deployment that narrowed the list could, and would still have made the request.
func TestFetchRefusesAnArchiveHostTheAllowListDoesNotName(t *testing.T) {
	f := &URLFetcher{Allowed: map[string]bool{"github.com": true}}
	_, _, err := f.Fetch(context.Background(), "https://github.com/o/r")
	if !errors.Is(err, ErrFetch) {
		t.Fatalf("want ErrFetch for a rewrite onto an off-list host, got %v", err)
	}
	if !strings.Contains(err.Error(), "codeload.github.com") {
		t.Errorf("the refusal must name the host it refused, got %q", err)
	}
	// No positive half here on purpose: the default list does name codeload, so
	// asserting it gets through means letting the request leave the machine, and
	// a test that needs the internet is a test that goes red for the wrong reason.
	// checkURL's accept path is covered by every other fetch test in this file.
}
