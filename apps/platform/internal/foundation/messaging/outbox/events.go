package outbox

// The event vocabulary (contracts/events/domain-events.md §3, DDD-012).
//
// It lives here rather than in internal/run because the catalogue is a contract
// between producers and consumers, and neither side should own it: internal/eval
// routes on these names without importing the run context, and the run context
// emits them without knowing who listens.
//
// The set is closed in three places that must agree — this file, the DB CHECK in
// db/migrations/0035, and the catalogue's §3 table. events_test.go compares all
// three, because a vocabulary enforced in only one of them drifts in the other two.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// Event is the Outbox-owned delivery contract. Generated persistence rows stay
// inside this package.
type Event struct {
	EventID          pgtype.UUID
	EventType        string
	EventVersion     int32
	OccurredAt       pgtype.Timestamptz
	CorrelationID    pgtype.UUID
	CausationID      pgtype.UUID
	WorkspaceID      pgtype.UUID
	AggregateType    string
	AggregateID      pgtype.UUID
	Payload          []byte
	PublishedAt      pgtype.Timestamptz
	DeliveryAttempts int32
	DeadLetteredAt   pgtype.Timestamptz
}

// NewEvent is the write-side contract for an event committed with a domain
// change. Delivery metadata is assigned by Outbox persistence.
type NewEvent struct {
	EventType     string
	EventVersion  int32
	CorrelationID pgtype.UUID
	CausationID   pgtype.UUID
	WorkspaceID   pgtype.UUID
	AggregateType string
	AggregateID   pgtype.UUID
	Payload       []byte
}

func eventFromRow(row gen.OutboxEvent) Event {
	return Event{
		EventID: row.EventID, EventType: row.EventType, EventVersion: row.EventVersion,
		OccurredAt: row.OccurredAt, CorrelationID: row.CorrelationID, CausationID: row.CausationID,
		WorkspaceID: row.WorkspaceID, AggregateType: row.AggregateType, AggregateID: row.AggregateID,
		Payload: row.Payload, PublishedAt: row.PublishedAt, DeliveryAttempts: row.DeliveryAttempts,
		DeadLetteredAt: row.DeadLetteredAt,
	}
}

// The run aggregate's status family (§3): one event per state entered.
const (
	RunQueued       = "run.queued"
	RunProvisioning = "run.provisioning"
	RunPreparing    = "run.preparing"
	RunRunning      = "run.running"
	RunEvaluating   = "run.evaluating"
	RunSucceeded    = "run.succeeded"
	RunFailed       = "run.failed"
	RunCancelled    = "run.cancelled"
	RunTimedOut     = "run.timed_out"
)

// The run aggregate's cleanup family (§3).
const (
	RunCleanupCleaned = "run.cleanup_cleaned"
	RunCleanupFailed  = "run.cleanup_failed"
)

const (
	// AggregateRun names the aggregate these events belong to. Its value equals
	// audit.ResourceRun, and that is a coincidence the domain no longer relies on:
	// the audit vocabulary and the event vocabulary are separate namespaces (§1)
	// and either may be renamed without the other.
	AggregateRun = "run"
	// EventVersion1 is the payload major version every event carries today (§2).
	// Additive payload changes do not bump it; a removed or retyped field does,
	// and then this constant gains a sibling rather than changing value.
	EventVersion1 = int32(1)
)

// EventTypes is the closed set, in catalogue order. Ordered because the
// conformance test compares it against the migration and the document as a
// sequence, which catches an entry added to one place and appended to another.
var EventTypes = []string{
	RunQueued,
	RunProvisioning,
	RunPreparing,
	RunRunning,
	RunEvaluating,
	RunSucceeded,
	RunFailed,
	RunCancelled,
	RunTimedOut,
	RunCleanupCleaned,
	RunCleanupFailed,
}

// StatusEvent maps a run status to the event announcing that the run entered it.
//
// A switch rather than "run." + string(status): concatenation makes every new
// enum value a new event type silently, which is exactly how a consumer ends up
// never hearing about a state nobody remembered to add to the catalogue. An
// unmapped status is an error the caller must handle, so the transaction that
// would have written an uncatalogued event rolls back instead (§4 rule 2).
func StatusEvent(status string) (string, error) {
	switch status {
	case "queued":
		return RunQueued, nil
	case "provisioning":
		return RunProvisioning, nil
	case "preparing":
		return RunPreparing, nil
	case "running":
		return RunRunning, nil
	case "evaluating":
		return RunEvaluating, nil
	case "succeeded":
		return RunSucceeded, nil
	case "failed":
		return RunFailed, nil
	case "cancelled":
		return RunCancelled, nil
	case "timed_out":
		return RunTimedOut, nil
	}
	return "", fmt.Errorf("no domain event for run status %q", status)
}

// CleanupEvent maps a cleanup outcome to its event. Only the two settled outcomes
// have one: `pending` and `cleaning_up` are the absence of an outcome, and
// announcing "cleanup is still running" as a fact would be announcing a guess.
func CleanupEvent(status string) (string, error) {
	switch status {
	case "cleaned":
		return RunCleanupCleaned, nil
	case "failed":
		return RunCleanupFailed, nil
	}
	return "", fmt.Errorf("no domain event for cleanup status %q", status)
}

// Insert writes one event with the caller's transaction handle.
//
// The pgx.Tx parameter is the whole point (§2, iron rule 9): calling
// (*gen.Queries).InsertOutboxEvent on a pool handle compiles, and the resulting
// event commits whether or not the change it announces did. Taking a transaction
// by type makes that mistake fail at build time rather than in production at 3am,
// when the run failed and the outbox says it succeeded.
//
// The event_type value set is enforced by the DB CHECK, not re-checked here: one
// authority, and it is the one that also covers maintenance SQL.
func Insert(ctx context.Context, tx pgx.Tx, event NewEvent) error {
	if tx == nil {
		return fmt.Errorf("outbox: transaction is not configured")
	}
	_, err := gen.New(tx).InsertOutboxEvent(ctx, gen.InsertOutboxEventParams{
		EventType: event.EventType, EventVersion: event.EventVersion,
		CorrelationID: event.CorrelationID, CausationID: event.CausationID,
		WorkspaceID: event.WorkspaceID, AggregateType: event.AggregateType,
		AggregateID: event.AggregateID, Payload: event.Payload,
	})
	return err
}
