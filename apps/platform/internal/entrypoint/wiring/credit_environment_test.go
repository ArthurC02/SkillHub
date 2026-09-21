package wiring

import "testing"

func TestCreditConfigFromEnvDefaultsAndRejectsMalformedMarkup(t *testing.T) {
	t.Setenv("CREDIT_USD_PER_CREDIT", "")
	t.Setenv("CREDIT_MARKUP_BPS", "")
	t.Setenv("CREDIT_DEBT_FLOOR", "")
	t.Setenv("CREDIT_MIN_START_FALLBACK", "")
	if cfg, err := CreditConfigFromEnv(); err != nil || cfg.MarkupBps != 13000 {
		t.Fatalf("defaults = %+v, %v", cfg, err)
	}
	t.Setenv("CREDIT_MARKUP_BPS", "not-a-number")
	if _, err := CreditConfigFromEnv(); err == nil {
		t.Fatal("malformed markup enabled credit configuration")
	}
}
