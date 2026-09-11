package credit

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"
)

type Config struct {
	SessionIdle time.Duration

	MicrosPerCredit int64

	MarkupBps int64

	DebtFloorCredits int64

	StartFallbackCredits int64
}

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
	if markupBps > MaxMarkupBps {
		return Config{}, fmt.Errorf("credit: CREDIT_MARKUP_BPS must be <= %d", MaxMarkupBps)
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
