package envx

import (
	"net"
	"net/url"
	"strings"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

type Posture struct {
	AppURL              string
	SecureCookies       bool
	DevLogin            bool
	DevCORSOrigin       string
	ImportAllowInsecure bool
	ImportExtraHosts    string
}

func (p Posture) Public() bool {
	return strings.HasPrefix(httpx.Origin(p.AppURL), "https://")
}

func (p Posture) APIRefusals() []string {
	refusals := p.devLoginRefusals()
	if p.SecureCookies && httpx.Origin(p.AppURL) == "" {
		refusals = append(refusals, "secure session cookies without a valid APP_URL: the same-origin write check "+
			"has no origin to compare and would let every cross-site write through. Set APP_URL to the public URL, "+
			"or set COOKIE_INSECURE=1 if this really is plain-http local dev.")
	}
	return append(refusals, p.publicDevelopmentRefusals()...)
}

func (p Posture) RateLimitRefusals(limits *httpx.RateLimiter) []string {
	if limits != nil || !p.Public() {
		return nil
	}
	return []string{"RATE_LIMIT=off on a deployment whose APP_URL is https: anonymous search and the " +
		"import endpoints would take unlimited traffic. Unset RATE_LIMIT."}
}

func (p Posture) WorkerRefusals() []string {
	return append(p.devLoginRefusals(), p.publicDevelopmentRefusals()...)
}

func (p Posture) devLoginRefusals() []string {
	switch {
	case !p.DevLogin:
		return nil
	case p.SecureCookies:
		return []string{"DEV_LOGIN=1 with secure session cookies: it marks a development deployment, where the API lets " +
			"anybody sign in as any name without a credential and the worker dispatches runs to sandboxes that share " +
			"the host kernel. Unset DEV_LOGIN, or set COOKIE_INSECURE=1 if this really is plain-http local dev."}
	case !p.servedOnlyToThisMachine():
		return []string{"DEV_LOGIN=1 with an APP_URL other machines reach: the API would let anybody who reaches " +
			p.AppURL + " sign in as any name without a credential. Unset DEV_LOGIN, or point APP_URL at localhost."}
	}
	return nil
}

func (p Posture) servedOnlyToThisMachine() bool {
	origin := httpx.Origin(p.AppURL)
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	return strings.EqualFold(host, "localhost") || (ip != nil && ip.IsLoopback())
}

func (p Posture) publicDevelopmentRefusals() []string {
	if !p.Public() {
		return nil
	}
	var refusals []string
	for _, dev := range []struct {
		set  bool
		name string
	}{
		{!p.SecureCookies, "COOKIE_INSECURE=1"},
		{p.DevCORSOrigin != "", "DEV_CORS_ORIGIN"},
		{p.ImportAllowInsecure, "IMPORT_ALLOW_INSECURE=1"},
		{p.ImportExtraHosts != "", "IMPORT_EXTRA_HOSTS"},
	} {
		if dev.set {
			refusals = append(refusals, dev.name+" on a deployment whose APP_URL is https: it is a local-development "+
				"setting that weakens a public deployment. Unset it.")
		}
	}
	return refusals
}
