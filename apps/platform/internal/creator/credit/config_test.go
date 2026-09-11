package credit

import "testing"

func clearCreditEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"CREDIT_USD_PER_CREDIT", "CREDIT_MARKUP_BPS",
		"CREDIT_DEBT_FLOOR", "CREDIT_MIN_START_FALLBACK",
	} {
		t.Setenv(k, "")
	}
}

func TestConfigFromEnvDefaults(t *testing.T) {
	clearCreditEnv(t)
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv() with nothing set: %v", err)
	}
	want := Config{MicrosPerCredit: 1000, MarkupBps: 13000, DebtFloorCredits: -50, StartFallbackCredits: 70}
	if cfg != want {
		t.Fatalf("defaults = %+v, want %+v", cfg, want)
	}
}

func TestConfigFromEnvFailsClosedOnMalformedValue(t *testing.T) {
	clearCreditEnv(t)
	t.Setenv("CREDIT_MARKUP_BPS", "not-a-number")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("a malformed CREDIT_MARKUP_BPS must fail closed, not silently fall back to the default")
	}
}

func TestConfigFromEnvFailsClosedOnPositiveDebtFloor(t *testing.T) {
	clearCreditEnv(t)
	t.Setenv("CREDIT_DEBT_FLOOR", "10")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("a positive CREDIT_DEBT_FLOOR must fail closed: the floor caps how negative a balance may go")
	}
}

func TestConfigFromEnvFailsClosedOnNonPositiveMarkup(t *testing.T) {
	clearCreditEnv(t)
	t.Setenv("CREDIT_MARKUP_BPS", "0")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("a zero or negative CREDIT_MARKUP_BPS must fail closed")
	}
}

func TestConfigFromEnvRespectsOverride(t *testing.T) {
	clearCreditEnv(t)
	t.Setenv("CREDIT_USD_PER_CREDIT", "0.01")
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MicrosPerCredit != 10000 {
		t.Fatalf("MicrosPerCredit = %d, want 10000 for CREDIT_USD_PER_CREDIT=0.01", cfg.MicrosPerCredit)
	}
}

func TestConfigFromEnvAcceptsTheLargestMarkupAChargeCanUse(t *testing.T) {
	clearCreditEnv(t)
	t.Setenv("CREDIT_MARKUP_BPS", "1000000")
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("CREDIT_MARKUP_BPS=1000000: %v", err)
	}
	if cfg.MarkupBps != 1_000_000 {
		t.Fatalf("MarkupBps = %d, want 1000000", cfg.MarkupBps)
	}
}

func TestConfigFromEnvFailsClosedOnAMarkupNoChargeCanUse(t *testing.T) {
	clearCreditEnv(t)
	t.Setenv("CREDIT_MARKUP_BPS", "1000001")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("CREDIT_MARKUP_BPS=1000001 must fail at startup: every charge would be refused and every call would go unbilled")
	}
}
