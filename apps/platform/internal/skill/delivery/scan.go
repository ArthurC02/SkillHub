package packaging

type ScanStatus string

const (
	ScanQuarantined ScanStatus = "quarantined"
	ScanAvailable   ScanStatus = "available"
	ScanRejected    ScanStatus = "rejected"
)

func AllScanStatuses() []ScanStatus {
	return []ScanStatus{ScanQuarantined, ScanAvailable, ScanRejected}
}
