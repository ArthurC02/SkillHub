package httpx

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestKeysetCursorWritesUTCNanosecondsThenTheID(t *testing.T) {
	at := time.Date(2026, 8, 19, 12, 30, 0, 123456789, time.FixedZone("x", 3600))
	if got, want := KeysetCursor(at, "42"), "2026-08-19T11:30:00.123456789Z_42"; got != want {
		t.Errorf("KeysetCursor = %q, want %q", got, want)
	}
}

func TestSplitKeysetCursorAcceptsOnlyATimeAndASeparator(t *testing.T) {
	for _, c := range []struct {
		name   string
		raw    string
		wantID string
		ok     bool
	}{
		{"a cursor this package wrote", "2026-08-19T11:30:00.123456789Z_42", "42", true},
		{"an id that itself holds the separator", "2026-08-19T11:30:00Z_a_b", "a_b", true},
		{"no separator", "2026-08-19T11:30:00Z", "", false},
		{"not a time", "yesterday_42", "42", false},
		{"empty", "", "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			at, id, ok := SplitKeysetCursor(c.raw)
			if ok != c.ok || id != c.wantID {
				t.Fatalf("SplitKeysetCursor(%q) = %v, %q, %v; want id %q, ok=%v", c.raw, at, id, ok, c.wantID, c.ok)
			}
			if ok && !at.Equal(time.Date(2026, 8, 19, 11, 30, 0, at.Nanosecond(), time.UTC)) {
				t.Errorf("time = %v, want 2026-08-19 11:30 UTC", at)
			}
		})
	}
}

func TestSplitUUIDKeysetCursorAcceptsOnlyATimeAndAUUID(t *testing.T) {
	const id = "01234567-89ab-cdef-0123-456789abcdef"
	for _, c := range []struct {
		name string
		raw  string
		ok   bool
	}{
		{"a cursor this package wrote", "2026-08-19T11:30:00Z_" + id, true},
		{"an id that is not a UUID", "2026-08-19T11:30:00Z_42", false},
		{"not a time", "yesterday_" + id, false},
		{"empty", "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			at, got, ok := SplitUUIDKeysetCursor(c.raw)
			if ok != c.ok {
				t.Fatalf("SplitUUIDKeysetCursor(%q) ok = %v, want %v", c.raw, ok, c.ok)
			}
			if !ok {
				if got.Valid || !at.IsZero() {
					t.Errorf("a refused cursor = %v, %v; want the zero time and an invalid id", at, got)
				}
				return
			}
			want := pgtype.UUID{Bytes: [16]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef}, Valid: true}
			if got != want || !at.Equal(time.Date(2026, 8, 19, 11, 30, 0, 0, time.UTC)) {
				t.Errorf("SplitUUIDKeysetCursor = %v, %v; want 2026-08-19 11:30 UTC, %v", at, got, want)
			}
		})
	}
}
