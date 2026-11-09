package httpx

import (
	"strings"
	"time"
)

const keysetSeparator = "_"

func KeysetCursor(at time.Time, id string) string {
	return at.UTC().Format(time.RFC3339Nano) + keysetSeparator + id
}

func SplitKeysetCursor(raw string) (at time.Time, id string, ok bool) {
	rawAt, id, found := strings.Cut(raw, keysetSeparator)
	at, err := time.Parse(time.RFC3339Nano, rawAt)
	return at, id, found && err == nil
}
