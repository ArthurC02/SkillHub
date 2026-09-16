package identity

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestASessionCountsOnlyBeforeItExpiresAndWhileItsAccountStands(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) pgtype.Timestamptz { return pgtype.Timestamptz{Time: now.Add(d), Valid: true} }
	for _, tc := range []struct {
		what    string
		expires pgtype.Timestamptz
		account accountLifecycle
		want    error
	}{
		{"a live session of a standing account", at(time.Nanosecond), accountLifecycle{}, nil},
		{"a session expiring this very moment", at(0), accountLifecycle{}, ErrSessionInvalid},
		{"an expired session", at(-time.Hour), accountLifecycle{}, ErrSessionInvalid},
		{"the account is being purged", at(time.Hour), accountLifecycle{purgeStartedAt: at(-time.Minute)}, ErrAccountPurging},
		{"the account was deleted", at(time.Hour), accountLifecycle{deletedAt: at(-time.Minute)}, ErrAccountGone},
		{"a deleted account whose purge had started reads as gone", at(time.Hour), accountLifecycle{deletedAt: at(-time.Minute), purgeStartedAt: at(-time.Hour)}, ErrAccountGone},
		{"an expired session of a deleted account reads as expired", at(-time.Hour), accountLifecycle{deletedAt: at(-time.Minute)}, ErrSessionInvalid},
	} {
		if got := sessionValidity(tc.expires, tc.account, now); !errors.Is(got, tc.want) {
			t.Errorf("%s: validity = %v, want %v", tc.what, got, tc.want)
		}
	}
}
