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

type Fetch struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256,omitempty"`
	Bytes  int    `json:"bytes,omitempty"`

	Status string `json:"status"`
}

const (
	MaxFetchBytes = 256 * 1024

	MaxFetchTextRunes = 8000
	fetchTimeout      = 15 * time.Second
	fetchRedirects    = 3
)

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

type Fetcher struct {
	client *http.Client
}

func NewFetcher() *Fetcher {
	return fetcherGuardedBy(publicIP)
}

func newLoopbackFetcher() *Fetcher {
	return fetcherGuardedBy(func(ip net.IP) bool {
		if ip.IsLoopback() {
			return true
		}
		return publicIP(ip)
	})
}

func fetcherGuardedBy(guard func(ip net.IP) bool) *Fetcher {
	dialer := &net.Dialer{Timeout: fetchTimeout}
	transport := &http.Transport{
		// No system proxy: one would route the connection around the
		// resolve-then-check dial guard below.
		Proxy: nil,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			// Resolve and check every address before dialing, and dial the
			// checked IP directly, to close a DNS-rebinding window between
			// the check and the connection.
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

func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (Fetch, string) {
	rec := Fetch{URL: rawURL}
	for attempt := 0; attempt < 2; attempt++ {
		got := f.once(ctx, rawURL)
		rec.Status, rec.SHA256, rec.Bytes = got.status, got.sha256, got.bytes
		if !got.retry || ctx.Err() != nil {
			return rec, got.text
		}
	}
	return rec, ""
}

type fetchAttempt struct {
	status string
	text   string
	sha256 string
	bytes  int
	retry  bool
}

func fetchEndedWith(status string) fetchAttempt { return fetchAttempt{status: status} }

func fetchWorthRetrying() fetchAttempt { return fetchAttempt{status: "network_error", retry: true} }

func (f *Fetcher) once(ctx context.Context, rawURL string) fetchAttempt {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fetchEndedWith("blocked")
	}
	req.Header.Set("Accept", "text/html, text/plain;q=0.9")
	req.Header.Set("User-Agent", "SkillHub-creation-fetch/1 (+consented)")
	resp, err := f.client.Do(req)
	if err != nil {
		if errors.Is(err, errFetchBlocked) {
			return fetchEndedWith("blocked")
		}
		var netErr net.Error
		if errors.As(err, &netErr) || errors.Is(err, context.DeadlineExceeded) {
			return fetchWorthRetrying()
		}
		return fetchEndedWith("blocked")
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return fetchEndedWith("not_found")
	case resp.StatusCode >= http.StatusInternalServerError:
		return fetchWorthRetrying()
	case resp.StatusCode >= http.StatusBadRequest:
		return fetchEndedWith("blocked")
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if !strings.HasPrefix(ct, "text/html") && !strings.HasPrefix(ct, "text/plain") {
		return fetchEndedWith("unsupported")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxFetchBytes))
	if err != nil {
		return fetchWorthRetrying()
	}
	if !utf8.Valid(body) {
		return fetchEndedWith("unsupported")
	}
	sum := sha256.Sum256(body)
	text := string(body)
	if strings.HasPrefix(ct, "text/html") {
		text = htmlToText(text)
	}
	text = truncateRunes(strings.TrimSpace(text), MaxFetchTextRunes)
	return fetchAttempt{status: "ok", text: text, sha256: hex.EncodeToString(sum[:]), bytes: len(body)}
}

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
