package main

import (
	"os"
	"strings"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

type deploymentPosture struct {
	appURL              string
	secureCookies       bool
	devLogin            bool
	devCORSOrigin       string
	importAllowInsecure bool
	importExtraHosts    string
}

func deploymentPostureFromEnv() deploymentPosture {
	return deploymentPosture{
		appURL:              os.Getenv("APP_URL"),
		secureCookies:       os.Getenv("COOKIE_INSECURE") != "1",
		devLogin:            os.Getenv("DEV_LOGIN") == "1",
		devCORSOrigin:       strings.TrimSpace(os.Getenv("DEV_CORS_ORIGIN")),
		importAllowInsecure: os.Getenv("IMPORT_ALLOW_INSECURE") == "1",
		importExtraHosts:    strings.TrimSpace(os.Getenv("IMPORT_EXTRA_HOSTS")),
	}
}

func (p deploymentPosture) public() bool {
	return strings.HasPrefix(httpx.Origin(p.appURL), "https://")
}

func (p deploymentPosture) refusals() []string {
	var refusals []string
	if p.devLogin && p.secureCookies {
		refusals = append(refusals, "DEV_LOGIN=1 with secure session cookies: the offline login provider "+
			"lets anybody sign in as any name without a credential, and a deployment that terminates TLS "+
			"is not a deployment that wants it. Unset DEV_LOGIN, or set COOKIE_INSECURE=1 if this really is plain-http local dev.")
	}
	if p.secureCookies && httpx.Origin(p.appURL) == "" {
		refusals = append(refusals, "secure session cookies without a valid APP_URL: the same-origin write check "+
			"has no origin to compare and would let every cross-site write through. Set APP_URL to the public URL, "+
			"or set COOKIE_INSECURE=1 if this really is plain-http local dev.")
	}
	if !p.public() {
		return refusals
	}
	for _, dev := range []struct {
		set  bool
		name string
	}{
		{!p.secureCookies, "COOKIE_INSECURE=1"},
		{p.devCORSOrigin != "", "DEV_CORS_ORIGIN"},
		{p.importAllowInsecure, "IMPORT_ALLOW_INSECURE=1"},
		{p.importExtraHosts != "", "IMPORT_EXTRA_HOSTS"},
	} {
		if dev.set {
			refusals = append(refusals, dev.name+" on a deployment whose APP_URL is https: it is a local-development "+
				"setting that weakens a public deployment. Unset it.")
		}
	}
	return refusals
}
