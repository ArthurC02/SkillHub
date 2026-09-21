package credit

import (
	"errors"
	"fmt"
	"math"
	"time"
)

type Config struct {
	SessionIdle time.Duration

	MicrosPerCredit int64

	MarkupBps int64

	DebtFloorCredits int64

	StartFallbackCredits int64
}

func NewConfig(usdPerCredit float64, markupBps, floor, fallback int64) (Config, error) {
	if usdPerCredit <= 0 {
		return Config{}, errors.New("credit: CREDIT_USD_PER_CREDIT must be > 0")
	}
	microsPerCredit := int64(math.Round(usdPerCredit * 1_000_000))
	if microsPerCredit <= 0 {
		return Config{}, errors.New("credit: CREDIT_USD_PER_CREDIT is too small to represent in micro-dollars")
	}

	if markupBps <= 0 {
		return Config{}, errors.New("credit: CREDIT_MARKUP_BPS must be > 0")
	}
	if markupBps > MaxMarkupBps {
		return Config{}, fmt.Errorf("credit: CREDIT_MARKUP_BPS must be <= %d", MaxMarkupBps)
	}

	if floor > 0 {
		return Config{}, errors.New("credit: CREDIT_DEBT_FLOOR must be <= 0")
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
