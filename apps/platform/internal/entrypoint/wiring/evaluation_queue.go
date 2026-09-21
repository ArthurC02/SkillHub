package wiring

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

var liveEvaluationJobStates = []rivertype.JobState{
	rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning,
	rivertype.JobStateScheduled, rivertype.JobStateRetryable,
}

func NewEvaluationEnqueue(client *river.Client[pgx.Tx]) func(context.Context, eval.JobArgs) error {
	if client == nil {
		return nil
	}
	return func(ctx context.Context, args eval.JobArgs) error {
		_, err := client.Insert(ctx, args, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: liveEvaluationJobStates}, MaxAttempts: 2})
		return err
	}
}

func NewSuggestionsAppliedEnqueue(client *river.Client[pgx.Tx]) func(context.Context, eval.SuggestionsAppliedArgs) error {
	if client == nil {
		return nil
	}
	return func(ctx context.Context, args eval.SuggestionsAppliedArgs) error {
		_, err := client.Insert(ctx, args, &river.InsertOpts{UniqueOpts: river.UniqueOpts{ByArgs: true}, MaxAttempts: eval.SuggestionsAppliedAttempts})
		return err
	}
}
