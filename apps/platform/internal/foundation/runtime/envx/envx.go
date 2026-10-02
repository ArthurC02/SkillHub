package envx

import (
	"log/slog"
	"os"
	"strconv"
)

func Or(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func PositiveInt32(key string, fallback int32) int32 {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n <= 0 {
		slog.Warn(key+" is set but is not a positive whole number; the default applies", "value", raw, "default", fallback)
		return fallback
	}
	return int32(n)
}
