package httpx

import (
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
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

func SplitUUIDKeysetCursor(raw string) (time.Time, pgtype.UUID, bool) {
	var id pgtype.UUID
	at, rawID, ok := SplitKeysetCursor(raw)
	if !ok || id.Scan(rawID) != nil {
		return time.Time{}, pgtype.UUID{}, false
	}
	return at, id, true
}
