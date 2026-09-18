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
	events, err := s.ReadEventsOfType(ctx, outbox.SkillVersionAdded,
		now.Add(-provenanceRecoveryWindow), provenanceRecoveryBatch)
	if err != nil {
		return 0, err
	}
	settled := now.Add(-RecoveryStaleAfter)

	var errs []error
	for _, event := range events {
		if event.OccurredAt.Time.After(settled) {
			continue
		}
		args, improved, err := suggestionsApplied(event)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !improved {
			continue
		}
		recorded, err := s.queries().ListSuggestionsAppliedToVersion(ctx, gen.ListSuggestionsAppliedToVersionParams{
			AppliedSkillVersionID: args.SkillVersionID, WorkspaceID: args.WorkspaceID,
		})
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if len(recorded) > 0 {
			continue
		}
		if err := s.ConsumeSuggestionsApplied(ctx, args, false); err != nil {
			errs = append(errs, err)
			continue
		}
		recovered++
		slog.Warn("a version's suggestion provenance was recorded by the recovery sweep, not by its delivery",
			"skill_version_id", pgconv.UUIDString(args.SkillVersionID),
			"evaluation_id", pgconv.UUIDString(args.EvaluationID))
	}
	return recovered, errors.Join(errs...)
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
