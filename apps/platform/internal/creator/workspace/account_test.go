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

func TestADeletionRequestStampsOnlyAFreshRequestAndNeitherChangeReachesAPurgingOrDeletedAccount(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) pgtype.Timestamptz { return pgtype.Timestamptz{Time: now.Add(d), Valid: true} }
	pending := accountLifecycle{deletionRequestedAt: at(-time.Hour), purgeAttemptedAt: at(-time.Minute)}
	staleAttempt := accountLifecycle{purgeAttemptedAt: at(-time.Minute)}
	request := func(a accountLifecycle) (accountLifecycle, error) { return a.requestDeletion(now) }
	for _, tc := range []struct {
		what    string
		change  func(accountLifecycle) (accountLifecycle, error)
		account accountLifecycle
		want    accountLifecycle
		err     error
	}{
		{"a fresh request starts with no purge attempt", request, staleAttempt, accountLifecycle{deletionRequestedAt: at(0)}, nil},
		{"a repeated request keeps its time and attempt", request, pending, pending, nil},
		{"a cancel clears the request and its attempt", accountLifecycle.cancelDeletion, pending, accountLifecycle{}, nil},
		{"a request once purging started", request, accountLifecycle{purgeStartedAt: at(-time.Minute)}, accountLifecycle{}, ErrAccountPurging},
		{"a cancel once purging started", accountLifecycle.cancelDeletion, accountLifecycle{purgeStartedAt: at(-time.Minute)}, accountLifecycle{}, ErrAccountPurging},
		{"a request of a deleted account", request, accountLifecycle{deletedAt: at(-time.Minute)}, accountLifecycle{}, ErrAccountPurging},
		{"a cancel of a deleted account", accountLifecycle.cancelDeletion, accountLifecycle{deletedAt: at(-time.Minute)}, accountLifecycle{}, ErrAccountPurging},
	} {
		got, err := tc.change(tc.account)
		if !errors.Is(err, tc.err) {
			t.Errorf("%s: error = %v, want %v", tc.what, err, tc.err)
		}
		if tc.err == nil && got != tc.want {
			t.Errorf("%s: account = %+v, want %+v", tc.what, got, tc.want)
		}
	}
}

func TestAnEmailIsStoredAndLookedUpInOneCanonicalForm(t *testing.T) {
	for given, want := range map[string]string{
		"alice@example.com":       "alice@example.com",
		"Alice@Example.COM":       "alice@example.com",
		" \talice@example.com \n": "alice@example.com",
	} {
		if got := normalizedEmail(given); got != want {
			t.Errorf("normalizedEmail(%q) = %q, want %q", given, got, want)
		}
	}
}
