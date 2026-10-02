package wiring

import (
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

const (
	gatewayAdminTimeout    = 20 * time.Second
	sandboxProviderTimeout = 30 * time.Second
)

func GatewayFromEnv() *run.Gateway {
	budget := runBudgetFromEnv()
	tpm := runTPMLimitFromEnv()
	return run.NewGateway(run.GatewayConfig{
		AdminBaseURL: strings.TrimSuffix(os.Getenv("SKILLHUB_MODEL_GATEWAY_ADMIN_URL"), "/"),
		AdminKey:     os.Getenv("SKILLHUB_MODEL_GATEWAY_KEY"), SandboxBaseURL: strings.TrimSuffix(os.Getenv("SKILLHUB_MODEL_GATEWAY_URL"), "/"),
		Model: os.Getenv("SKILLHUB_RUN_MODEL"), MaxBudgetUSD: budget, TPMLimit: tpm, HTTP: internalHTTPClient(gatewayAdminTimeout),
	})
}

func RunDeploymentFromEnv() run.Deployment {
	budget := runBudgetFromEnv()
	cleanMode := os.Getenv("SKILLHUB_CLEAN_MODE") == "1"
	minimumIsolation := run.StrongIsolation
	if cleanMode {
		minimumIsolation = run.NoIsolation
	} else if os.Getenv("DEV_LOGIN") == "1" {
		minimumIsolation = run.WeakIsolation
	}
	return run.Deployment{Model: os.Getenv("SKILLHUB_RUN_MODEL"), GatewayURL: strings.TrimSuffix(os.Getenv("SKILLHUB_MODEL_GATEWAY_URL"), "/"), BudgetUSD: budget, MinimumIsolation: minimumIsolation, CleanMode: cleanMode, CleanModeReleases: os.Getenv("SKILLHUB_CLEAN_MODE_RELEASES")}
}

const runBudgetVariable = "SKILLHUB_RUN_MAX_BUDGET_USD"

func runBudgetFromEnv() float64 {
	raw := os.Getenv(runBudgetVariable)
	budget, err := strconv.ParseFloat(raw, 64)
	if raw != "" && (err != nil || run.Deployment{BudgetUSD: budget}.Budget() != budget) {
		slog.Warn(runBudgetVariable+" is set but unusable; the default run budget applies", "value", raw)
	}
	return budget
}

const runTPMLimitVariable = "SKILLHUB_RUN_TPM_LIMIT"

func runTPMLimitFromEnv() int {
	raw := os.Getenv(runTPMLimitVariable)
	tpm, err := strconv.Atoi(raw)
	if raw != "" && (err != nil || tpm <= 0) {
		slog.Warn(runTPMLimitVariable+" is set but unusable; the default tokens-per-minute limit applies", "value", raw)
		return 0
	}
	return tpm
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
			name, baseURL, os.Getenv("SKILLHUB_SANDBOX_TOKEN_"+strings.ToUpper(name)), internalHTTPClient(sandboxProviderTimeout)))
	}
	return run.NewRegistry(providers...)
}
