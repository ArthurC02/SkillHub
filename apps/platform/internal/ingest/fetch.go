package ingest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/skillpkg"
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
		"github.com":                    true,
		"codeload.github.com":           true,
		"objects.githubusercontent.com": true,
	}
}

func (f *URLFetcher) client() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (f *URLFetcher) checkURL(u *url.URL) error {
	if u.Scheme != "https" && (!f.AllowInsecure || u.Scheme != "http") {
		return fmt.Errorf("%w: only https URLs are supported", ErrFetch)
	}
	if !f.Allowed[strings.ToLower(u.Host)] {
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
		return nil, fmt.Errorf("%w: source returned status %d", ErrFetch, resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, skillpkg.MaxZipBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFetch, err)
	}
	if len(data) > skillpkg.MaxZipBytes {
		return nil, fmt.Errorf("%w: package exceeds %d bytes", ErrFetch, skillpkg.MaxZipBytes)
	}
	return data, nil
}
