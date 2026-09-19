package ingest

import (
	"fmt"
	"net/url"
	"strings"
)

type sourcePolicy struct {
	allowedHosts  map[string]bool
	allowInsecure bool
}

func (p sourcePolicy) check(u *url.URL) error {
	if u.Scheme != "https" && (!p.allowInsecure || u.Scheme != "http") {
		return fmt.Errorf("%w: 來源網址必須是 https。", ErrFetch)
	}
	if !p.allowedHosts[strings.ToLower(u.Host)] {
		return fmt.Errorf("%w: 來源網域 %q 不在允許清單內。", ErrFetch, u.Host)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%w: 來源網址不得帶帳號密碼、查詢字串或錨點。", ErrFetch)
	}
	return nil
}

func (p sourcePolicy) normalize(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("%w: 這不是一個合法的網址。", ErrFetch)
	}
	if err := p.check(u); err != nil {
		return "", err
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	return u.String(), nil
}
