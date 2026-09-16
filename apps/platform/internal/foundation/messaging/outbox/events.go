package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

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

type NewEvent struct {
	EventType     string
	EventVersion  int32
	CorrelationID pgtype.UUID
	CausationID   pgtype.UUID
	WorkspaceID   pgtype.UUID
	AggregateType string
	AggregateID   pgtype.UUID
	Payload       any
}

type RunStatusChanged struct {
	ToStatus   string `json:"to_status"`
	FromStatus string `json:"from_status,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type RunCleanupChanged struct {
	CleanupStatus string `json:"cleanup_status"`
	FailureCount  int    `json:"failure_count,omitempty"`
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

const (
	RunCleanupCleaned = "run.cleanup_cleaned"
	RunCleanupFailed  = "run.cleanup_failed"
)

const (
	RunCancelRequested      = "run.cancel_requested"
	RunProviderAssigned     = "run.provider_assigned"
	RunAttemptStarted       = "run.attempt_started"
	RunAttemptDispatched    = "run.attempt_dispatched"
	RunAttemptFinished      = "run.attempt_finished"
	RunObjectGrantsRecorded = "run.object_grants_recorded"
)

const (
	EvaluationStarted            = "evaluation.started"
	EvaluationSuperseded         = "evaluation.superseded"
	EvaluationCompleted          = "evaluation.completed"
	EvaluationFailed             = "evaluation.failed"
	EvaluationFeedbackRecorded   = "evaluation.feedback_recorded"
	EvaluationSuggestionDecided  = "evaluation.suggestion_decided"
	EvaluationSuggestionsApplied = "evaluation.suggestions_applied"
)

const (
	SkillTakenDown               = "skill.taken_down"
	SkillAccessRestricted        = "skill.access_restricted"
	SkillAccessRestrictionLifted = "skill.access_restriction_lifted"
	SkillRedistributionSet       = "skill.redistribution_set"
	SkillCategorized             = "skill.categorized"
	SkillDeleted                 = "skill.deleted"
	SkillCreated                 = "skill.created"
	SkillVersionAdded            = "skill.version_added"
	SkillDescribed               = "skill.described"
)

const (
	AggregateRun        = "run"
	AggregateEvaluation = "evaluation"
	AggregateSkill      = "skill"

	EventVersion1 = int32(1)
)

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
	RunCancelRequested,
	RunProviderAssigned,
	RunAttemptStarted,
	RunAttemptDispatched,
	RunAttemptFinished,
	RunObjectGrantsRecorded,
	EvaluationStarted,
	EvaluationSuperseded,
	EvaluationCompleted,
	EvaluationFailed,
	EvaluationFeedbackRecorded,
	EvaluationSuggestionDecided,
	EvaluationSuggestionsApplied,
	SkillTakenDown,
	SkillAccessRestricted,
	SkillAccessRestrictionLifted,
	SkillRedistributionSet,
	SkillCategorized,
	SkillDeleted,
	SkillCreated,
	SkillVersionAdded,
	SkillDescribed,
}

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

func CleanupEvent(status string) (string, error) {
	switch status {
	case "cleaned":
		return RunCleanupCleaned, nil
	case "failed":
		return RunCleanupFailed, nil
	}
	return "", fmt.Errorf("no domain event for cleanup status %q", status)
}

var ErrUnknownEventType = errors.New("outbox: event type is not in the closed set")

func Insert(ctx context.Context, tx pgx.Tx, event NewEvent) error {
	if !slices.Contains(EventTypes, event.EventType) {
		return fmt.Errorf("%w: %q", ErrUnknownEventType, event.EventType)
	}
	if tx == nil {
		return fmt.Errorf("outbox: transaction is not configured")
	}
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return err
	}
	_, err = gen.New(tx).InsertOutboxEvent(ctx, gen.InsertOutboxEventParams{
		EventType: event.EventType, EventVersion: event.EventVersion,
		CorrelationID: event.CorrelationID, CausationID: event.CausationID,
		WorkspaceID: event.WorkspaceID, AggregateType: event.AggregateType,
		AggregateID: event.AggregateID, Payload: payload,
	})
	return err
}
