package wiring

import "testing"

func TestPostureFromEnvReadsOnlyItsExplicitOptIns(t *testing.T) {
	t.Setenv("APP_URL", "https://app.example.test")
	t.Setenv("COOKIE_INSECURE", "")
	t.Setenv("DEV_LOGIN", "true")
	t.Setenv("DEV_CORS_ORIGIN", " https://web.example.test ")
	t.Setenv("IMPORT_ALLOW_INSECURE", "1")
	t.Setenv("IMPORT_EXTRA_HOSTS", " files.example.test ")

	posture := PostureFromEnv()
	if posture.AppURL != "https://app.example.test" || !posture.SecureCookies || posture.DevLogin ||
		posture.DevCORSOrigin != "https://web.example.test" || !posture.ImportAllowInsecure || posture.ImportExtraHosts != "files.example.test" {
		t.Fatalf("posture = %+v", posture)
	}
}
