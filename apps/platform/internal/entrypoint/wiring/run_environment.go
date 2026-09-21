package wiring

import (
	"os"
	"strings"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func NewRunRegistryFromEnv() *run.Registry {
	providers := make([]run.SandboxProvider, 0)
	for _, entry := range strings.Split(os.Getenv("SKILLHUB_SANDBOX_PROVIDERS"), ",") {
		name, baseURL, ok := strings.Cut(strings.TrimSpace(entry), "=")
		name, baseURL = strings.TrimSpace(name), strings.TrimSpace(baseURL)
		if !ok || name == "" || baseURL == "" {
			continue
		}
		providers = append(providers, run.NewProvider(
			name, baseURL, os.Getenv("SKILLHUB_SANDBOX_TOKEN_"+strings.ToUpper(name))))
	}
	return run.NewRegistry(providers...)
}
