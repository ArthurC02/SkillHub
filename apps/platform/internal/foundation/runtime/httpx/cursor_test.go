package httpx

import (
	"testing"
	"time"
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
