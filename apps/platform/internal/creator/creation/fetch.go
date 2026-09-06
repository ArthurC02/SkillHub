package creation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// Fetch is one page Go read for a session after the person said yes (05
// R-47, 2026-09-06). The text itself lives only in the tool observation the
// model saw; the snapshot keeps where it came from and what happened.
type Fetch struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256,omitempty"`
	Bytes  int    `json:"bytes,omitempty"`
	// Status: ok, blocked (the site or the network environment refused — not
	// retried), not_found, unsupported (not text), network_error (retried
	// once), declined (the person said no).
	Status string `json:"status"`
}

const (
	// MaxFetchBytes caps what is read from a page; the rest is dropped and
	// the observation says so.
	MaxFetchBytes = 256 * 1024
	// MaxFetchTextRunes caps the text handed to the model.
	MaxFetchTextRunes = 8000
	fetchTimeout      = 15 * time.Second
	fetchRedirects    = 3
)

// validateFetchURL is the rule applied before the person is asked: an
// absolute http(s) URL with a host name, no credentials, and not a literal
// private, loopback, link-local or unspecified address. Names that resolve to
// such addresses are refused at dial time by the Fetcher.
func validateFetchURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", errors.New("not a URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("scheme must be http or https")
	}
	if u.Hostname() == "" || u.User != nil {
		return "", errors.New("host required, credentials refused")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !publicIP(ip) {
		return "", errors.New("private address refused")
	}
	u.Fragment = ""
	return u.String(), nil
}

func publicIP(ip net.IP) bool {
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() &&
		!ip.IsMulticast() && !ip.IsUnspecified()
}

// Fetcher reads one page as text. It is the Worker's, never the API's (the
// step job calls it before the model call), and it never follows a redirect
// or dials an address the guard refuses.
type Fetcher struct {
	client *http.Client
}

// NewFetcher builds the guarded client. allowLoopback exists for tests
// against httptest servers; production wiring passes false.
func NewFetcher(allowLoopback bool) *Fetcher {
	guard := func(ip net.IP) bool {
		if allowLoopback && ip.IsLoopback() {
			return true
		}
		return publicIP(ip)
	}
	dialer := &net.Dialer{Timeout: fetchTimeout}
	transport := &http.Transport{
		Proxy: nil, // the environment's proxy would bypass the dial guard
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if !guard(ip.IP) {
					return nil, errFetchBlocked
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
		TLSHandshakeTimeout:   fetchTimeout,
		ResponseHeaderTimeout: fetchTimeout,
		DisableKeepAlives:     true,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   fetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= fetchRedirects {
				return errFetchBlocked
			}
			if _, err := validateFetchURL(req.URL.String()); err != nil {
				return errFetchBlocked
			}
			return nil
		},
	}
	return &Fetcher{client: client}
}

var errFetchBlocked = errors.New("fetch blocked")

// Fetch reads the page and returns the record plus the text for the model.
// The owner's rule (05 R-47): a refusal by the site or by the network
// environment is reported once, never retried; only a network-level error
// (DNS, timeout, reset before any response) gets one more try.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (Fetch, string) {
	rec := Fetch{URL: rawURL}
	for attempt := 0; attempt < 2; attempt++ {
		status, text, sha, n, retry := f.once(ctx, rawURL)
		rec.Status, rec.SHA256, rec.Bytes = status, sha, n
		if !retry || ctx.Err() != nil {
			return rec, text
		}
	}
	return rec, ""
}

func (f *Fetcher) once(ctx context.Context, rawURL string) (status, text, sha string, n int, retry bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "blocked", "", "", 0, false
	}
	req.Header.Set("Accept", "text/html, text/plain;q=0.9")
	req.Header.Set("User-Agent", "SkillHub-creation-fetch/1 (+consented)")
	resp, err := f.client.Do(req)
	if err != nil {
		if errors.Is(err, errFetchBlocked) {
			return "blocked", "", "", 0, false
		}
		var netErr net.Error
		if errors.As(err, &netErr) || errors.Is(err, context.DeadlineExceeded) {
			return "network_error", "", "", 0, true
		}
		return "blocked", "", "", 0, false
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return "not_found", "", "", 0, false
	case resp.StatusCode >= 500:
		return "network_error", "", "", 0, true
	case resp.StatusCode >= 400:
		return "blocked", "", "", 0, false
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if !strings.HasPrefix(ct, "text/html") && !strings.HasPrefix(ct, "text/plain") {
		return "unsupported", "", "", 0, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxFetchBytes))
	if err != nil {
		return "network_error", "", "", 0, true
	}
	if !utf8.Valid(body) {
		return "unsupported", "", "", 0, false
	}
	sum := sha256.Sum256(body)
	text = string(body)
	if strings.HasPrefix(ct, "text/html") {
		text = htmlToText(text)
	}
	text = truncateRunes(strings.TrimSpace(text), MaxFetchTextRunes)
	return "ok", text, hex.EncodeToString(sum[:]), len(body), false
}

// htmlToText drops script and style blocks, then tags, and folds whitespace.
// Not a parser: the model reads prose, and a page that is nothing but markup
// comes back empty rather than as markup.
func htmlToText(s string) string {
	lower := strings.ToLower(s)
	for _, tag := range []string{"script", "style", "noscript"} {
		for {
			start := strings.Index(lower, "<"+tag)
			if start < 0 {
				break
			}
			end := strings.Index(lower[start:], "</"+tag)
			if end < 0 {
				s, lower = s[:start], lower[:start]
				break
			}
			close := strings.Index(lower[start+end:], ">")
			if close < 0 {
				s, lower = s[:start], lower[:start]
				break
			}
			cut := start + end + close + 1
			s, lower = s[:start]+" "+s[cut:], lower[:start]+" "+lower[cut:]
		}
	}
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
			b.WriteRune(' ')
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n]) + "…"
}

// fetchObservation is the tool message the model reads. Sentences are Go's;
// the page text is untrusted and travels inside the JSON like a run
// observation does.
func fetchObservation(rec Fetch, text string) string {
	var why string
	switch rec.Status {
	case "ok":
		why = "已讀取。"
	case "blocked":
		why = "網站或這個網路環境拒絕了連線（不會重試）；請改用手上的資料或問使用者。"
	case "not_found":
		why = "頁面不存在。"
	case "unsupported":
		why = "不是文字頁面，無法讀取。"
	case "network_error":
		why = "網路層錯誤，重試一次後仍失敗。"
	case "declined":
		why = "使用者不同意連網；請用手上的資料繼續。"
	default:
		why = "未讀取。"
	}
	data, _ := json.Marshal(map[string]any{"fetch": map[string]any{
		"url": rec.URL, "status": rec.Status, "note": why, "sha256": rec.SHA256, "bytes": rec.Bytes, "text": text,
	}})
	return string(data)
}
