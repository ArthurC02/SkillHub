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

var ErrFetch = errors.New("fetch failed")

type URLFetcher struct {
	Allowed map[string]bool

	AllowInsecure bool
}

const (
	connectTimeout = 10 * time.Second
	fetchTimeout   = 60 * time.Second
	maxRedirects   = 3
)

var (
	errBlockedDestination = errors.New("destination address is not allowed")
	errTooManyRedirects   = errors.New("too many redirects")
)

var alwaysBlocked = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("255.255.255.255/32"),
}

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
	return ip.IsLoopback() || ip.IsPrivate()
}

func newClient(dev bool) *http.Client {
	d := &net.Dialer{
		Timeout:   connectTimeout,
		KeepAlive: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil || blockedAddr(ap.Addr(), dev) {
				return errBlockedDestination
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

func (f *URLFetcher) do(req *http.Request) (*http.Response, error) {
	client := *f.client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {

		if len(via) > maxRedirects {
			return errTooManyRedirects
		}
		return f.checkURL(req.URL)
	}
	resp, err := client.Do(req)

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

func classify(err error) error {
	var ne net.Error
	switch {
	case errors.Is(err, errTooManyRedirects):
		return fmt.Errorf("%w: 來源網址的轉址次數超過上限，平台不再往下追。請直接給出套件 zip 的最終網址。", ErrFetch)
	case errors.Is(err, errBlockedDestination), errors.Is(err, ErrFetch):

		return fmt.Errorf("%w: 轉址之後落在不允許的位址，下載已停止。", ErrFetch)
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return fmt.Errorf("%w: 連線來源逾時。稍後再試，或先確認這個網址在外部下載得到。", ErrFetch)
	}
	return fmt.Errorf("%w: 連不上這個來源。", ErrFetch)
}

func (f *URLFetcher) checkURL(u *url.URL) error {
	if u.Scheme != "https" && (!f.AllowInsecure || u.Scheme != "http") {
		return fmt.Errorf("%w: 來源網址必須是 https。", ErrFetch)
	}
	if !f.Allowed[strings.ToLower(u.Host)] {

		return fmt.Errorf("%w: 來源網域 %q 不在允許清單內。", ErrFetch, u.Host)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%w: 來源網址不得帶帳號密碼、查詢字串或錨點。", ErrFetch)
	}
	return nil
}

func (f *URLFetcher) Normalize(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("%w: 這不是一個合法的網址。", ErrFetch)
	}
	if err := f.checkURL(u); err != nil {
		return "", err
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	return u.String(), nil
}

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

var commitSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

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
	case len(parts) == 2:
		return []candidate{
			{archive(parts[0], parts[1], "heads", "main"), "main"},
			{archive(parts[0], parts[1], "heads", "master"), "master"},
		}, ""

	case len(parts) >= 4 && (parts[2] == "tree" || parts[2] == "commit"):
		ref := strings.Join(parts[3:], "/")
		if commitSHA.MatchString(ref) {
			return []candidate{{zipAt(parts[0], parts[1], ref), ref}}, ""
		}
		return []candidate{{archive(parts[0], parts[1], "heads", ref), ref}}, ""
	default:
		return []candidate{{url: u.String()}}, ""
	}
}

func (f *URLFetcher) download(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: 這不是一個合法的網址。", ErrFetch)
	}

	if err := f.checkURL(req.URL); err != nil {
		return nil, err
	}
	resp, err := f.do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("%w: 來源回應 HTTP %d，沒有取得檔案。", ErrFetch, resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, skillpkg.MaxZipBytes+1))
	if err != nil {
		return nil, classify(err)
	}
	if len(data) > skillpkg.MaxZipBytes {

		metrics.PackageSizeRefused.WithLabelValues(metrics.CeilingURL).Inc()
		return nil, fmt.Errorf("%w: 這個來源的套件超過平台的 %s 匯入上限。",
			ErrFetch, skillpkg.HumanMB(skillpkg.MaxZipBytes))
	}
	return data, nil
}
