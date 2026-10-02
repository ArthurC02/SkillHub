package eval

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

const (
	provenanceRecoveryWindow = 7 * 24 * time.Hour
	provenanceRecoveryBatch  = 200
)

func (s *Service) RecoverLostSuggestionProvenance(ctx context.Context) (recovered int, err error) {
	if s.ReadEventsOfType == nil {
		return 0, nil
	}
	now := time.Now()
	page := outbox.EventPage{
		EventType: outbox.SkillVersionAdded,
		After:     outbox.EventCursor{OccurredAt: now.Add(-provenanceRecoveryWindow)},
		Until:     now.Add(-RecoveryStaleAfter),
		Limit:     provenanceRecoveryBatch,
	}

	var errs []error
	for {
		events, err := s.ReadEventsOfType(ctx, page)
		if err != nil {
			return recovered, errors.Join(append(errs, err)...)
		}
		for _, event := range events {
			ok, err := s.recoverProvenanceOf(ctx, event)
			if err != nil {
				errs = append(errs, err)
			}
			if ok {
				recovered++
			}
		}
		if len(events) < provenanceRecoveryBatch {
			return recovered, errors.Join(errs...)
		}
		page.After = events[len(events)-1].Cursor()
	}
}

func (s *Service) recoverProvenanceOf(ctx context.Context, event outbox.Event) (bool, error) {
	args, improved, err := suggestionsApplied(event)
	if err != nil || !improved {
		return false, err
	}
	recorded, err := s.queries().ListSuggestionsAppliedToVersion(ctx, gen.ListSuggestionsAppliedToVersionParams{
		AppliedSkillVersionID: args.SkillVersionID, WorkspaceID: args.WorkspaceID,
	})
	if err != nil || len(recorded) > 0 {
		return false, err
	}
	if err := s.ConsumeSuggestionsApplied(ctx, args); err != nil {
		return false, err
	}
	slog.Warn("a version's suggestion provenance was recorded by the recovery sweep, not by its delivery",
		"skill_version_id", pgconv.UUIDString(args.SkillVersionID),
		"evaluation_id", pgconv.UUIDString(args.EvaluationID))
	return true, nil
}

func suggestionsApplied(event outbox.Event) (SuggestionsAppliedArgs, bool, error) {
	var added versionAdded
	if err := json.Unmarshal(event.Payload, &added); err != nil {
		return SuggestionsAppliedArgs{}, false, err
	}
	if added.ImprovedBy == nil {
		return SuggestionsAppliedArgs{}, false, nil
	}
	return SuggestionsAppliedArgs{
		EventID:     event.EventID,
		WorkspaceID: event.WorkspaceID, SkillID: event.AggregateID, SkillVersionID: added.VersionID,
		EvaluationID: added.ImprovedBy.EvaluationID, SuggestionIDs: added.ImprovedBy.SuggestionIDs,
	}, true, nil
}
