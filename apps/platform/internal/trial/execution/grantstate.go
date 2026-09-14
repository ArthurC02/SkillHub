package run

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5/pgtype"
)

type ObjectGrantState string

const (
	ObjectGrantStateLegacyUnknown ObjectGrantState = "legacy_unknown"
	ObjectGrantStateUnissued      ObjectGrantState = "unissued"
	ObjectGrantStateRecorded      ObjectGrantState = "recorded"
	ObjectGrantStateClosed        ObjectGrantState = "closed"
)

var ErrObjectGrantTransition = errors.New("run: the attempt's object grants cannot move that way")

func AllObjectGrantStates() []ObjectGrantState {
	return []ObjectGrantState{
		ObjectGrantStateLegacyUnknown,
		ObjectGrantStateUnissued,
		ObjectGrantStateRecorded,
		ObjectGrantStateClosed,
	}
}

var objectGrantSuccessors = map[ObjectGrantState][]ObjectGrantState{
	ObjectGrantStateUnissued: {ObjectGrantStateRecorded, ObjectGrantStateClosed},
}

func ParseObjectGrantState(s string) (ObjectGrantState, bool) {
	candidate := ObjectGrantState(s)
	if slices.Contains(AllObjectGrantStates(), candidate) {
		return candidate, true
	}
	return "", false
}

func CanTransitionObjectGrant(from, to ObjectGrantState) bool {
	if _, known := ParseObjectGrantState(string(from)); !known {
		return false
	}
	if _, known := ParseObjectGrantState(string(to)); !known {
		return false
	}
	if from == to {
		return true
	}
	return slices.Contains(objectGrantSuccessors[from], to)
}

const purgeClockTolerance = time.Minute

func objectGrantsExpiredOnArrival() time.Time {
	return time.Now().UTC().Add(-2 * purgeClockTolerance)
}

func (s *Service) recordObjectGrantExpiry(ctx context.Context, attempt gen.RunAttempt, expires time.Time) error {
	_, err := s.commandRun(ctx, attempt.WorkspaceID, attempt.RunID, pgtype.UUID{}, func(r *Run) error {
		r.RecordGrantExpiry(attempt.ID, expires)
		return nil
	})
	return err
}
