package identity

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrSessionInvalid = errors.New("identity: session unknown or expired")
	ErrAccountGone    = errors.New("identity: account deleted")
)

type accountLifecycle struct {
	deletedAt      pgtype.Timestamptz
	purgeStartedAt pgtype.Timestamptz
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

func sessionValidity(expiresAt pgtype.Timestamptz, account accountLifecycle, now time.Time) error {
	if !expiresAt.Time.After(now) {
		return ErrSessionInvalid
	}
	return account.standing()
}
