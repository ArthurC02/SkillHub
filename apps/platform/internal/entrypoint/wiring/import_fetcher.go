package wiring

import (
	"strings"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/envx"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
)

func ImportFetcher(posture envx.Posture) *ingest.URLFetcher {
	fetcher := &ingest.URLFetcher{Allowed: ingest.DefaultAllowedHosts(), AllowInsecure: posture.ImportAllowInsecure}
	for _, host := range strings.Split(posture.ImportExtraHosts, ",") {
		if host = strings.TrimSpace(strings.ToLower(host)); host != "" {
			fetcher.Allowed[host] = true
		}
	}
	return fetcher
}
