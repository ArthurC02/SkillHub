package wiring

import (
	"fmt"
	"os"

	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

const JudgePanelEnv = "JUDGE_PANEL"

func JudgePanelFromEnv() ([]string, error) {
	switch value := os.Getenv(JudgePanelEnv); value {
	case "", "off":
		return nil, nil
	case "on":
		return eval.PanelRoles, nil
	default:
		return nil, fmt.Errorf("%s must be on or off, not %q", JudgePanelEnv, value)
	}
}
