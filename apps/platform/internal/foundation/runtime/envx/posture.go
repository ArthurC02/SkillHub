package envx

import (
	"os"
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

func PostureFromEnv() Posture {
	return Posture{
		AppURL:              os.Getenv("APP_URL"),
		SecureCookies:       os.Getenv("COOKIE_INSECURE") != "1",
		DevLogin:            os.Getenv("DEV_LOGIN") == "1",
		DevCORSOrigin:       strings.TrimSpace(os.Getenv("DEV_CORS_ORIGIN")),
		ImportAllowInsecure: os.Getenv("IMPORT_ALLOW_INSECURE") == "1",
		ImportExtraHosts:    strings.TrimSpace(os.Getenv("IMPORT_EXTRA_HOSTS")),
	}
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

func (p Posture) WorkerRefusals() []string {
	return append(p.devLoginRefusals(), p.publicDevelopmentRefusals()...)
}

func (p Posture) devLoginRefusals() []string {
	if !p.DevLogin || !p.SecureCookies {
		return nil
	}
	return []string{"DEV_LOGIN=1 with secure session cookies: it marks a development deployment, where the API lets " +
		"anybody sign in as any name without a credential and the worker dispatches runs to sandboxes that share " +
		"the host kernel. Unset DEV_LOGIN, or set COOKIE_INSECURE=1 if this really is plain-http local dev."}
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
