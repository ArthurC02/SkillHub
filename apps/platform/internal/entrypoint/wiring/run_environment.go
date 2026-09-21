package wiring

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func GatewayFromEnv() *run.Gateway {
	budget, _ := strconv.ParseFloat(os.Getenv("SKILLHUB_RUN_MAX_BUDGET_USD"), 64)
	tpm, _ := strconv.Atoi(os.Getenv("SKILLHUB_RUN_TPM_LIMIT"))
	return run.NewGateway(run.GatewayConfig{
		AdminBaseURL: strings.TrimSuffix(os.Getenv("SKILLHUB_MODEL_GATEWAY_ADMIN_URL"), "/"),
		AdminKey:     os.Getenv("SKILLHUB_MODEL_GATEWAY_KEY"), SandboxBaseURL: strings.TrimSuffix(os.Getenv("SKILLHUB_MODEL_GATEWAY_URL"), "/"),
		Model: os.Getenv("SKILLHUB_RUN_MODEL"), MaxBudgetUSD: budget, TPMLimit: tpm, HTTP: &http.Client{Timeout: 20 * time.Second},
	})
}

func RunDeploymentFromEnv() run.Deployment {
	budget, _ := strconv.ParseFloat(os.Getenv("SKILLHUB_RUN_MAX_BUDGET_USD"), 64)
	return run.Deployment{Model: os.Getenv("SKILLHUB_RUN_MODEL"), GatewayURL: strings.TrimSuffix(os.Getenv("SKILLHUB_MODEL_GATEWAY_URL"), "/"), BudgetUSD: budget}
}

func NewRunRegistryFromEnv() *run.Registry {
	providers := make([]run.SandboxProvider, 0)
	for _, entry := range strings.Split(os.Getenv("SKILLHUB_SANDBOX_PROVIDERS"), ",") {
		name, baseURL, ok := strings.Cut(strings.TrimSpace(entry), "=")
		name, baseURL = strings.TrimSpace(name), strings.TrimSpace(baseURL)
		if !ok || name == "" || baseURL == "" {
			continue
		}
		providers = append(providers, run.NewProviderWithClient(
			name, baseURL, os.Getenv("SKILLHUB_SANDBOX_TOKEN_"+strings.ToUpper(name)), &http.Client{Timeout: 30 * time.Second}))
	}
	return run.NewRegistry(providers...)
}
