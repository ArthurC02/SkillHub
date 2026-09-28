package wiring

import "testing"

func TestCreditConfigWithoutOverridesUsesTheDocumentedDefaults(t *testing.T) {
	for _, name := range []string{"CREDIT_USD_PER_CREDIT", "CREDIT_MARKUP_BPS", "CREDIT_DEBT_FLOOR", "CREDIT_MIN_START_FALLBACK"} {
		t.Setenv(name, "")
	}
	cfg, err := CreditConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MicrosPerCredit != 1000 || cfg.MarkupBps != 13000 || cfg.DebtFloorCredits != -50 || cfg.StartFallbackCredits != 70 {
		t.Errorf("defaults = micros per credit %d, markup %d, debt floor %d, start fallback %d; want 1000, 13000, -50, 70",
			cfg.MicrosPerCredit, cfg.MarkupBps, cfg.DebtFloorCredits, cfg.StartFallbackCredits)
	}
}

func TestARunIsDrivenAtMostThreeTimesAndCleanedUpAtMostFive(t *testing.T) {
	if got := executeOptions().MaxAttempts; got != 3 {
		t.Errorf("run execution attempts = %d, want 3", got)
	}
	if got := cleanupOptions().MaxAttempts; got != 5 {
		t.Errorf("run cleanup attempts = %d, want 5", got)
	}
}
