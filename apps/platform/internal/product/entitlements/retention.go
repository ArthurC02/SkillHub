package policy

import (
	"errors"
	"time"
)

var ErrRetentionNotConfigured = errors.New("download artifact retention is not configured")

type DownloadRetention time.Duration

func (r DownloadRetention) Period() (time.Duration, error) {
	if r <= 0 {
		return 0, ErrRetentionNotConfigured
	}
	return time.Duration(r), nil
}
