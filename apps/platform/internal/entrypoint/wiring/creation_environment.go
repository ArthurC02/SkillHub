package wiring

import (
	"os"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
)

func CreationLimitsFromEnv() (creation.Limits, error) {
	return creation.LimitsFromJSON(os.Getenv("CREATION_LIMITS_JSON"))
}

func CreationExposedFromEnv() bool { return os.Getenv("CREATION_EXPOSED") == "on" }
