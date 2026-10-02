package wiring

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
)

func CreationLimitsFromEnv() (creation.Limits, error) {
	raw := os.Getenv("CREATION_LIMITS_JSON")
	limits, err := creation.LimitsFromJSON(raw)
	if err != nil && raw != "" {
		slog.Warn("CREATION_LIMITS_JSON is set but unusable; interactive creation stays off", "error", err)
	}
	return limits, err
}

func CreationExposedFromEnv() bool { return os.Getenv("CREATION_EXPOSED") == "on" }

const transientCallSlack = 30 * time.Second

func CreationTransientFromEnv(limits creation.Limits) func(context.Context, creation.JobArgs, *creation.Diagram) error {
	timeout := limits.CallTimeout + transientCallSlack
	return creation.TransientClientWithHTTP(
		os.Getenv("CREATION_WORKER_INTERNAL_URL"), os.Getenv("CREATION_WORKER_INTERNAL_TOKEN"), timeout,
		internalHTTPClient(timeout),
	)
}
