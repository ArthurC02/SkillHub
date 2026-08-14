package ingest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ErrFetch marks URL-import failures the client can fix: disallowed host,
// unreachable URL, oversized package.
var ErrFetch = errors.New("fetch failed")

// URLFetcher downloads skill packages from allow-listed hosts only (INGEST-001,
// SEC-003). The allow list plus a size cap is the whole SSRF story here: URLs
// resolve to well-known public hosts, redirects are re-checked against the
// same list, and nothing internal is ever on it.
type URLFetcher struct {
	// Allowed hosts (exact match, lowercase, host[:port]).
	Allowed map[string]bool
	// AllowInsecure permits plain http; tests and local stubs only.
	AllowInsecure bool
	Client        *http.Client
}

// DefaultAllowedHosts covers GitHub repo and archive downloads (PDM-002: the
// first-batch skill source is GitHub).
func DefaultAllowedHosts() map[string]bool {
	return map[string]bool{
		"github.com":          true,
		"codeload.github.com": true,
		"objects.githubusercontent.com": true,
	}
}

func (f *URLFetcher) client() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return http.DefaultClient
}

func (f *URLFetcher) checkURL(u *url.URL) error {
	if u.Scheme != "https" && !(f.AllowInsecure && u.Scheme == "http") {
		return fmt.Errorf("%w: only https URLs are supported", ErrFetch)
	}
	if !f.Allowed[strings.ToLower(u.Host)] {
		return fmt.Errorf("%w: host %q is not on the allowed source list", ErrFetch, u.Host)
	}
	return nil
}

// Fetch downloads rawURL and returns the package bytes plus the git ref when
// it can be derived from the URL. GitHub repo pages are normalized to their
// codeload zip archives; anything else must already point at a zip.
func (f *URLFetcher) Fetch(ctx context.Context, rawURL string) (data []byte, ref string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", fmt.Errorf("%w: invalid URL", ErrFetch)
	}
	if err := f.checkURL(u); err != nil {
		return nil, "", err
	}

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

// candidates expands a GitHub repo URL into archive URLs to try in order;
// non-repo URLs pass through unchanged.
// ponytail: main-then-master probing instead of a GitHub API call for the
// default branch; switch to the API if unauthenticated rate limits ever bite.
func (f *URLFetcher) candidates(u *url.URL) ([]candidate, string) {
	if strings.ToLower(u.Host) != "github.com" {
		return []candidate{{url: u.String()}}, ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	archive := func(owner, repo, kind, ref string) string {
		return fmt.Sprintf("https://codeload.github.com/%s/%s/zip/refs/%s/%s", owner, repo, kind, ref)
	}
	switch {
	case len(parts) == 2: // github.com/owner/repo
		return []candidate{
			{archive(parts[0], parts[1], "heads", "main"), "main"},
			{archive(parts[0], parts[1], "heads", "master"), "master"},
		}, ""
	case len(parts) >= 4 && parts[2] == "tree": // github.com/owner/repo/tree/<ref>
		ref := strings.Join(parts[3:], "/")
		return []candidate{{archive(parts[0], parts[1], "heads", ref), ref}}, ""
	default: // e.g. github.com/owner/repo/archive/... — direct file URL
		return []candidate{{url: u.String()}}, ""
	}
}

func (f *URLFetcher) download(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetch, err)
	}
	client := *f.client()
	// Redirect targets must satisfy the same allow list (SSRF).
	client.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		return f.checkURL(req.URL)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetch, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s returned status %d", ErrFetch, rawURL, resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxZipBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetch, err)
	}
	if len(data) > MaxZipBytes {
		return nil, fmt.Errorf("%w: package exceeds %d bytes", ErrFetch, MaxZipBytes)
	}
	return data, nil
}
