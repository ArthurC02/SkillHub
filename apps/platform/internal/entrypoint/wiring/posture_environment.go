package wiring

import (
	"os"
	"strings"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/envx"
)

func PostureFromEnv() envx.Posture {
	return envx.Posture{
		AppURL:              os.Getenv("APP_URL"),
		SecureCookies:       os.Getenv("COOKIE_INSECURE") != "1",
		DevLogin:            os.Getenv("DEV_LOGIN") == "1",
		DevCORSOrigin:       strings.TrimSpace(os.Getenv("DEV_CORS_ORIGIN")),
		ImportAllowInsecure: os.Getenv("IMPORT_ALLOW_INSECURE") == "1",
		ImportExtraHosts:    strings.TrimSpace(os.Getenv("IMPORT_EXTRA_HOSTS")),
	}
}
