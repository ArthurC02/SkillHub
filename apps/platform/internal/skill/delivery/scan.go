package packaging

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type ScanStatus string

func servableAt(scan ScanStatus, deletedAt, purgedAt, expiresAt pgtype.Timestamptz, now time.Time) bool {
	return scan == ScanAvailable && !deletedAt.Valid && !purgedAt.Valid && expiresAt.Time.After(now)
}

const (
	ScanQuarantined ScanStatus = "quarantined"
	ScanAvailable   ScanStatus = "available"
	ScanRejected    ScanStatus = "rejected"
)

func AllScanStatuses() []ScanStatus {
	return []ScanStatus{ScanQuarantined, ScanAvailable, ScanRejected}
}
