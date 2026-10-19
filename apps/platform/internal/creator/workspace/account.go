package identity

import (
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrSessionInvalid = errors.New("identity: session unknown or expired")
	ErrAccountGone    = errors.New("identity: account deleted")
)

type accountLifecycle struct {
	deletedAt           pgtype.Timestamptz
	purgeStartedAt      pgtype.Timestamptz
	deletionRequestedAt pgtype.Timestamptz
	purgeAttemptedAt    pgtype.Timestamptz
	erasureExpeditedAt  pgtype.Timestamptz
}

func (a accountLifecycle) gone() bool {
	return a.deletedAt.Valid
}

func (a accountLifecycle) purging() bool {
	return a.purgeStartedAt.Valid
}

func (a accountLifecycle) standing() error {
	switch {
	case a.gone():
		return ErrAccountGone
	case a.purging():
		return ErrAccountPurging
	}
	return nil
}

func (a accountLifecycle) requestDeletion(now time.Time) (accountLifecycle, error) {
	if a.standing() != nil {
		return a, ErrAccountPurging
	}
	if !a.deletionRequestedAt.Valid {
		a.deletionRequestedAt, a.purgeAttemptedAt = pgtype.Timestamptz{Time: now, Valid: true}, pgtype.Timestamptz{}
	}
	return a, nil
}

func (a accountLifecycle) expediteErasure(now time.Time) (accountLifecycle, error) {
	a, err := a.requestDeletion(now)
	if err != nil {
		return a, err
	}
	if !a.erasureExpeditedAt.Valid {
		a.erasureExpeditedAt, a.purgeAttemptedAt = pgtype.Timestamptz{Time: now, Valid: true}, pgtype.Timestamptz{}
	}
	return a, nil
}

func (a accountLifecycle) cancelDeletion() (accountLifecycle, error) {
	if a.standing() != nil {
		return a, ErrAccountPurging
	}
	a.deletionRequestedAt, a.purgeAttemptedAt, a.erasureExpeditedAt = pgtype.Timestamptz{}, pgtype.Timestamptz{}, pgtype.Timestamptz{}
	return a, nil
}

func normalizedEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func sessionValidity(expiresAt pgtype.Timestamptz, account accountLifecycle, now time.Time) error {
	if !expiresAt.Time.After(now) {
		return ErrSessionInvalid
	}
	return account.standing()
}
