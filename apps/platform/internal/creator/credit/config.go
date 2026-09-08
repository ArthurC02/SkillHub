package credit

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
)

// Config is the four CREDIT_* settings ADR-068 leaves to deployment: the
// face value of a credit, the markup applied before a charge, the debt
// floor gate ② enforces, and the fallback threshold gate ① uses when there
// are not yet MinStatSamples statistics to derive one from.
type Config struct {
	// MicrosPerCredit is how many USD-micros one credit costs, derived from
	// CREDIT_USD_PER_CREDIT (default 0.001 USD, i.e. 1000 micros/credit).
	MicrosPerCredit int64
	// MarkupBps is the basis-points markup applied to a real cost before it
	// is converted to credits (default 13000 = 1.3x, ADR-068 decision 2).
	MarkupBps int64
	// DebtFloorCredits is gate ②'s ceiling on negative balance; always <= 0
	// (default -50 — the owner's literal "負債不能超過 -50", decision 7).
	DebtFloorCredits int64
	// StartFallbackCredits is gate ①'s threshold when fewer than
	// MinStatSamples samples exist yet (default 70).
	StartFallbackCredits int64
}

// ConfigFromEnv reads the four CREDIT_* settings, applying the ADR-068
// defaults when a variable is unset and failing closed — an error, never a
// silently substituted default — when one is set but does not parse or
// sits outside the range the decision it implements requires. A
// misconfigured markup or floor is a billing bug, not something to shrug
// past (CRED-001).
func ConfigFromEnv() (Config, error) {
	usdPerCredit, err := envFloat("CREDIT_USD_PER_CREDIT", 0.001)
	if err != nil {
		return Config{}, fmt.Errorf("credit: CREDIT_USD_PER_CREDIT: %w", err)
	}
	if usdPerCredit <= 0 {
		return Config{}, errors.New("credit: CREDIT_USD_PER_CREDIT must be > 0")
	}
	microsPerCredit := int64(math.Round(usdPerCredit * 1_000_000))
	if microsPerCredit <= 0 {
		return Config{}, errors.New("credit: CREDIT_USD_PER_CREDIT is too small to represent in micro-dollars")
	}

	markupBps, err := envInt("CREDIT_MARKUP_BPS", 13000)
	if err != nil {
		return Config{}, fmt.Errorf("credit: CREDIT_MARKUP_BPS: %w", err)
	}
	if markupBps <= 0 {
		return Config{}, errors.New("credit: CREDIT_MARKUP_BPS must be > 0")
	}

	floor, err := envInt("CREDIT_DEBT_FLOOR", -50)
	if err != nil {
		return Config{}, fmt.Errorf("credit: CREDIT_DEBT_FLOOR: %w", err)
	}
	if floor > 0 {
		return Config{}, errors.New("credit: CREDIT_DEBT_FLOOR must be <= 0")
	}

	fallback, err := envInt("CREDIT_MIN_START_FALLBACK", 70)
	if err != nil {
		return Config{}, fmt.Errorf("credit: CREDIT_MIN_START_FALLBACK: %w", err)
	}
	if fallback < 0 {
		return Config{}, errors.New("credit: CREDIT_MIN_START_FALLBACK must be >= 0")
	}

	return Config{
		MicrosPerCredit:      microsPerCredit,
		MarkupBps:            markupBps,
		DebtFloorCredits:     floor,
		StartFallbackCredits: fallback,
	}, nil
}

func envFloat(name string, def float64) (float64, error) {
	raw, ok := os.LookupEnv(name)
	if !ok || raw == "" {
		return def, nil
	}
	return strconv.ParseFloat(raw, 64)
}

func envInt(name string, def int64) (int64, error) {
	raw, ok := os.LookupEnv(name)
	if !ok || raw == "" {
		return def, nil
	}
	return strconv.ParseInt(raw, 10, 64)
}
