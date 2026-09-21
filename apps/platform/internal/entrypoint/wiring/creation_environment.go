package wiring

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
)

func CreationLimitsFromEnv() (creation.Limits, error) {
	return creation.LimitsFromJSON(os.Getenv("CREATION_LIMITS_JSON"))
}

func CreationExposedFromEnv() bool { return os.Getenv("CREATION_EXPOSED") == "on" }

func CreationTransientFromEnv(limits creation.Limits) func(context.Context, creation.JobArgs, *creation.Diagram) error {
	timeout := limits.CallTimeout + 30*time.Second
	return creation.TransientClientWithHTTP(
		os.Getenv("CREATION_WORKER_INTERNAL_URL"), os.Getenv("CREATION_WORKER_INTERNAL_TOKEN"), timeout,
		&http.Client{Timeout: timeout},
	)
}
