package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/facts"
)

func TestTheEvaluationWorkerReadsTheRunOnlyOnItsFirstAttempt(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://skillhub@127.0.0.1:1/skillhub?connect_timeout=1")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	unreadable := errors.New("run facts unreadable")

	for _, tc := range []struct {
		name      string
		attempt   int
		wantAsked bool
	}{
		{"the first attempt evaluates from the run's facts", 1, true},
		{"a retry looks for the current evaluation before anything else", 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			asked := false
			svc := &eval.Service{
				Pool: pool,
				ReadEvaluationInput: func(context.Context, pgtype.UUID, pgtype.UUID) (facts.EvaluationInput, bool, error) {
					asked = true
					return facts.EvaluationInput{}, false, unreadable
				},
				ReadVersion: func(context.Context, pgtype.UUID, pgtype.UUID) (facts.Version, bool, error) {
					return facts.Version{}, false, nil
				},
				ReadSkill: func(context.Context, pgtype.UUID, pgtype.UUID) (facts.Skill, bool, error) {
					return facts.Skill{}, false, nil
				},
				ReadRuntimeCompatibility: func(context.Context, pgtype.UUID) (facts.RuntimeCompatibility, bool, error) {
					return facts.RuntimeCompatibility{}, false, nil
				},
			}
			job := &river.Job[eval.JobArgs]{
				JobRow: &rivertype.JobRow{Attempt: tc.attempt},
				Args:   eval.JobArgs{RunID: uuid.NewString(), WorkspaceID: uuid.NewString()},
			}

			err := (&EvaluationExecuteWorker{Svc: svc}).Work(context.Background(), job)
			if asked != tc.wantAsked {
				t.Fatalf("attempt %d read the run's facts = %v, want %v", tc.attempt, asked, tc.wantAsked)
			}
			if tc.wantAsked && !errors.Is(err, unreadable) {
				t.Errorf("attempt %d returned %v, want the reader's error", tc.attempt, err)
			}
			if !tc.wantAsked && err == nil {
				t.Errorf("attempt %d reported success although the database is unreachable", tc.attempt)
			}
		})
	}
}
