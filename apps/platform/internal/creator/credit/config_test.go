package credit

import "testing"

func TestNewConfigAcceptsDefaults(t *testing.T) {
	cfg, err := NewConfig(0.001, 13000, -50, 70)
	if err != nil {
		t.Fatalf("ConfigFromEnv() with nothing set: %v", err)
	}
	want := Config{MicrosPerCredit: 1000, MarkupBps: 13000, DebtFloorCredits: -50, StartFallbackCredits: 70}
	if cfg != want {
		t.Fatalf("defaults = %+v, want %+v", cfg, want)
	}
}

func TestNewConfigFailsClosedOnPositiveDebtFloor(t *testing.T) {
	if _, err := NewConfig(0.001, 13000, 10, 70); err == nil {
		t.Fatal("a positive CREDIT_DEBT_FLOOR must fail closed: the floor caps how negative a balance may go")
	}
}

func TestNewConfigFailsClosedOnNonPositiveMarkup(t *testing.T) {
	if _, err := NewConfig(0.001, 0, -50, 70); err == nil {
		t.Fatal("a zero or negative CREDIT_MARKUP_BPS must fail closed")
	}
}

func TestNewConfigRespectsRate(t *testing.T) {
	cfg, err := NewConfig(0.01, 13000, -50, 70)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MicrosPerCredit != 10000 {
		t.Fatalf("MicrosPerCredit = %d, want 10000 for CREDIT_USD_PER_CREDIT=0.01", cfg.MicrosPerCredit)
	}
}

func TestNewConfigAcceptsTheLargestMarkupAChargeCanUse(t *testing.T) {
	cfg, err := NewConfig(0.001, 1000000, -50, 70)
	if err != nil {
		t.Fatalf("CREDIT_MARKUP_BPS=1000000: %v", err)
	}
	if cfg.MarkupBps != 1_000_000 {
		t.Fatalf("MarkupBps = %d, want 1000000", cfg.MarkupBps)
	}
}

func TestNewConfigFailsClosedOnAMarkupNoChargeCanUse(t *testing.T) {
	if _, err := NewConfig(0.001, 1000001, -50, 70); err == nil {
		t.Fatal("CREDIT_MARKUP_BPS=1000001 must fail at startup: every charge would be refused and every call would go unbilled")
	}
}
