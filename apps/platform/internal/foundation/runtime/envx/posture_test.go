package envx

import (
	"strings"
	"testing"
)

func TestADeploymentRefusesToStartWithDevelopmentSettingsOrWithoutItsOrigin(t *testing.T) {
	public := Posture{AppURL: "https://skillhub.example", SecureCookies: true}
	local := Posture{AppURL: "http://localhost:5173", DevLogin: true, DevCORSOrigin: "http://localhost:5173",
		ImportAllowInsecure: true, ImportExtraHosts: "localhost"}
	with := func(p Posture, change func(*Posture)) Posture { change(&p); return p }
	for _, tc := range []struct {
		name    string
		posture Posture
		refuses []string
	}{
		{"a public deployment with nothing from development", public, nil},
		{"local development on plain http with every development setting", local, nil},
		{"a smoke stack with insecure cookies and no APP_URL", Posture{DevLogin: true}, nil},
		{"dev login with secure cookies", with(public, func(p *Posture) { p.DevLogin = true }), []string{"DEV_LOGIN"}},
		{"secure cookies and no APP_URL", Posture{SecureCookies: true}, []string{"APP_URL"}},
		{"secure cookies and an APP_URL with no host", Posture{AppURL: "https://", SecureCookies: true}, []string{"APP_URL"}},
		{"public with insecure cookies", with(public, func(p *Posture) { p.SecureCookies = false }), []string{"COOKIE_INSECURE"}},
		{"public with a development CORS origin", with(public, func(p *Posture) { p.DevCORSOrigin = "http://localhost:5173" }), []string{"DEV_CORS_ORIGIN"}},
		{"public importing over plain http", with(public, func(p *Posture) { p.ImportAllowInsecure = true }), []string{"IMPORT_ALLOW_INSECURE"}},
		{"public importing from extra hosts", with(public, func(p *Posture) { p.ImportExtraHosts = "localhost" }), []string{"IMPORT_EXTRA_HOSTS"}},
		{"an upper-case https origin is still public", with(public, func(p *Posture) { p.AppURL = "HTTPS://SkillHub.example"; p.DevCORSOrigin = "x" }), []string{"DEV_CORS_ORIGIN"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.posture.APIRefusals()
			if len(got) != len(tc.refuses) {
				t.Fatalf("refusals = %q, want one naming each of %q", got, tc.refuses)
			}
			for i, name := range tc.refuses {
				if !strings.Contains(got[i], name) {
					t.Errorf("refusal %d = %q, want it to name %s", i, got[i], name)
				}
			}
		})
	}
}

func TestAWorkerRefusesDevelopmentSettingsButNeedsNoOrigin(t *testing.T) {
	public := Posture{AppURL: "https://skillhub.example", SecureCookies: true}
	for _, tc := range []struct {
		name    string
		posture Posture
		refuses []string
	}{
		{"a worker given no APP_URL", Posture{SecureCookies: true}, nil},
		{"a local worker with dev login on insecure cookies", Posture{DevLogin: true}, nil},
		{"a worker with dev login and secure cookies", Posture{SecureCookies: true, DevLogin: true}, []string{"DEV_LOGIN"}},
		{"a public worker with a development import setting", Posture{AppURL: public.AppURL, SecureCookies: true, ImportExtraHosts: "localhost"}, []string{"IMPORT_EXTRA_HOSTS"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.posture.WorkerRefusals()
			if len(got) != len(tc.refuses) {
				t.Fatalf("refusals = %q, want one naming each of %q", got, tc.refuses)
			}
			for i, name := range tc.refuses {
				if !strings.Contains(got[i], name) {
					t.Errorf("refusal %d = %q, want it to name %s", i, got[i], name)
				}
			}
		})
	}
}
