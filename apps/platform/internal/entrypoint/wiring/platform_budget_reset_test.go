package wiring

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTheGatewayBudgetResetsDailyAsEveryRefusalTellsTheUser(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "infra", "compose", "litellm-config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Settings struct {
			BudgetDuration string `yaml:"budget_duration"`
		} `yaml:"litellm_settings"`
	}
	if err := yaml.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}

	if config.Settings.BudgetDuration != "1d" {
		t.Fatalf("budget_duration = %q; the budget refusals promise a reset at 08:00 Taipei (UTC midnight) every day, "+
			"so change their wording together with this setting", config.Settings.BudgetDuration)
	}
}
