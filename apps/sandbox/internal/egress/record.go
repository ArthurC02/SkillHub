package egress

import "time"

const (
	schemaVersion = "1.0"

	flowRecord       = "egress_flow"
	addressRecord    = "run_address"
	addressAssigned  = "assigned"
	addressReleased  = "released"
	acceptedDecision = "accepted"
	blockedDecision  = "blocked"
)

type flow struct {
	SchemaVersion   string    `json:"schema_version"`
	Record          string    `json:"record"`
	At              time.Time `json:"at"`
	Decision        string    `json:"decision"`
	BlockedBy       string    `json:"blocked_by,omitempty"`
	Protocol        string    `json:"protocol"`
	Source          string    `json:"source"`
	Destination     string    `json:"destination"`
	DestinationPort int       `json:"destination_port,omitempty"`
	PacketsOut      int64     `json:"packets_out"`
	BytesOut        int64     `json:"bytes_out"`
	PacketsIn       int64     `json:"packets_in"`
	BytesIn         int64     `json:"bytes_in"`
}

const Message = "egress record"

func AddressAssigned(runID string, attempt int, address string, at time.Time) []any {
	return addressFields(addressAssigned, runID, attempt, address, at)
}

func AddressReleased(runID string, attempt int, address string, at time.Time) []any {
	return addressFields(addressReleased, runID, attempt, address, at)
}

func addressFields(state, runID string, attempt int, address string, at time.Time) []any {
	return []any{
		"schema_version", schemaVersion,
		"record", addressRecord,
		"at", at.UTC(),
		"state", state,
		"run_id", runID,
		"attempt", attempt,
		"address", address,
	}
}
