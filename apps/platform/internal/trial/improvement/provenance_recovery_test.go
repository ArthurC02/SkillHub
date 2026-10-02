package eval

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func readsEventsFrom(pool *pgxpool.Pool) func(context.Context, outbox.EventPage) ([]outbox.Event, error) {
	return func(ctx context.Context, page outbox.EventPage) ([]outbox.Event, error) {
		return outbox.EventsOfType(ctx, pool, page)
	}
}

func publishVersionAdded(t *testing.T, s *Service, m material, versionID, evaluationID pgtype.UUID,
	suggestionIDs []pgtype.UUID, occurredAt time.Time,
) {
	t.Helper()
	payload := map[string]any{"version_id": pgconvString(versionID)}
	if evaluationID.Valid {
		payload["improved_by"] = map[string]any{
			"evaluation_id":  pgconvString(evaluationID),
			"suggestion_ids": stringsOf(suggestionIDs),
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Pool.Exec(context.Background(), `
		INSERT INTO outbox_events
			(event_type, event_version, occurred_at, correlation_id, workspace_id,
			 aggregate_type, aggregate_id, payload, published_at)
		VALUES ($1, 1, $2, gen_random_uuid(), $3, 'skill', $4, $5, $2)`,
		outbox.SkillVersionAdded, occurredAt, m.run.WorkspaceID, m.run.ID, encoded)
	if err != nil {
		t.Fatalf("publish version_added: %v", err)
	}
}

func pgconvString(id pgtype.UUID) string {
	text, err := id.MarshalJSON()
	if err != nil {
		return ""
	}
	return string(text[1 : len(text)-1])
}

func stringsOf(ids []pgtype.UUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, pgconvString(id))
	}
	return out
}

func appliedSuggestionCount(t *testing.T, s *Service, workspaceID, versionID pgtype.UUID) int {
	t.Helper()
	rows, err := s.queries().ListSuggestionsAppliedToVersion(context.Background(),
		gen.ListSuggestionsAppliedToVersionParams{AppliedSkillVersionID: versionID, WorkspaceID: workspaceID})
	if err != nil {
		t.Fatalf("read applications: %v", err)
	}
	return len(rows)
}

func TestProvenanceThatRanOutOfDeliveryTriesIsStillRecordedByTheSweep(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	s.ReadEventsOfType = readsEventsFrom(s.Pool)
	m := seedRun(t, s.Pool)
	evaluation := beginAndComplete(t, s, m, aVerdict("complete", OverallMet))
	suggestion := seedSuggestion(t, s, m.run.WorkspaceID, evaluation.ID, "X")
	version := seedImprovedVersion(t, s.Pool, m.run.ID, 2, t.Name()+"-improved")

	publishVersionAdded(t, s, m, version, evaluation.ID, []pgtype.UUID{suggestion.ID},
		time.Now().Add(-RecoveryStaleAfter-time.Minute))
	if got := appliedSuggestionCount(t, s, m.run.WorkspaceID, version); got != 0 {
		t.Fatalf("%d applications before the sweep, want 0: the letter is meant to be lost", got)
	}

	if err := s.RecoverPending(context.Background()); err != nil {
		t.Fatalf("recovery: %v", err)
	}

	if got := appliedSuggestionCount(t, s, m.run.WorkspaceID, version); got != 1 {
		t.Fatalf("%d applications after the sweep, want 1: the version still has no one who knows "+
			"which suggestion it came from, and evaluations cannot be rewritten later", got)
	}

	again, err := s.RecoverLostSuggestionProvenance(context.Background())
	if err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if again != 0 {
		t.Errorf("the second sweep recovered %d, want 0: a version whose provenance is already "+
			"recorded is read again on every sweep", again)
	}
	if got := appliedSuggestionCount(t, s, m.run.WorkspaceID, version); got != 1 {
		t.Errorf("%d applications after sweeping twice, want the same 1", got)
	}
}

func TestTheSweepReachesALostDeliveryBehindAFullPageOfOlderVersions(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	s.ReadEventsOfType = readsEventsFrom(s.Pool)
	m := seedRun(t, s.Pool)
	evaluation := beginAndComplete(t, s, m, aVerdict("complete", OverallMet))
	suggestion := seedSuggestion(t, s, m.run.WorkspaceID, evaluation.ID, "X")
	version := seedImprovedVersion(t, s.Pool, m.run.ID, 2, t.Name()+"-improved")

	lostAt := time.Now().Add(-RecoveryStaleAfter - time.Minute)
	for i := range provenanceRecoveryBatch {
		publishVersionAdded(t, s, m, pgtype.UUID{Bytes: [16]byte{15: byte(i), 14: byte(i >> 8)}, Valid: true},
			pgtype.UUID{}, nil, lostAt.Add(-time.Hour-time.Duration(i)*time.Second))
	}
	publishVersionAdded(t, s, m, version, evaluation.ID, []pgtype.UUID{suggestion.ID}, lostAt)

	if _, err := s.RecoverLostSuggestionProvenance(context.Background()); err != nil {
		t.Fatalf("recovery sweep: %v", err)
	}
	if got := appliedSuggestionCount(t, s, m.run.WorkspaceID, version); got != 1 {
		t.Errorf("%d applications after the sweep, want 1: a full page of older versions hid the lost "+
			"delivery, so the sweep only reaches it once those versions leave the window", got)
	}
}

func TestTheSweepLeavesADeliveryThatStillHasTimeAlone(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	s.ReadEventsOfType = readsEventsFrom(s.Pool)
	m := seedRun(t, s.Pool)
	evaluation := beginAndComplete(t, s, m, aVerdict("complete", OverallMet))
	suggestion := seedSuggestion(t, s, m.run.WorkspaceID, evaluation.ID, "X")
	version := seedImprovedVersion(t, s.Pool, m.run.ID, 2, t.Name()+"-improved")

	publishVersionAdded(t, s, m, version, evaluation.ID, []pgtype.UUID{suggestion.ID},
		time.Now().Add(-RecoveryStaleAfter+time.Minute))

	recovered, err := s.RecoverLostSuggestionProvenance(context.Background())
	if err != nil {
		t.Fatalf("recovery sweep: %v", err)
	}
	if recovered != 0 || appliedSuggestionCount(t, s, m.run.WorkspaceID, version) != 0 {
		t.Errorf("the sweep recovered %d, want 0: the delivery still had attempts left and the sweep raced it",
			recovered)
	}
}

func TestTheSweepPassesOverAVersionThatCameFromNoSuggestion(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	s.ReadEventsOfType = readsEventsFrom(s.Pool)
	m := seedRun(t, s.Pool)
	version := seedImprovedVersion(t, s.Pool, m.run.ID, 2, t.Name()+"-plain")

	publishVersionAdded(t, s, m, version, pgtype.UUID{}, nil,
		time.Now().Add(-RecoveryStaleAfter-time.Minute))

	if _, err := s.RecoverLostSuggestionProvenance(context.Background()); err != nil {
		t.Fatalf("a version with no suggestions behind it made the sweep fail: %v", err)
	}
	if got := appliedSuggestionCount(t, s, m.run.WorkspaceID, version); got != 0 {
		t.Errorf("%d applications for a version nobody improved, want 0", got)
	}
}

func TestAStaleEvaluationThatCannotBeRecoveredDoesNotStarveTheProvenanceSweep(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	s.ReadEventsOfType = readsEventsFrom(s.Pool)
	var stuck gen.Evaluation
	t.Run("stuck", func(t *testing.T) {
		var err error
		if stuck, err = s.begin(context.Background(), seedRun(t, s.Pool)); err != nil {
			t.Fatal(err)
		}
	})
	t.Cleanup(func() {
		_, _ = s.Pool.Exec(context.Background(), "DELETE FROM evaluations WHERE id = $1", stuck.ID)
	})
	if _, err := s.Pool.Exec(context.Background(), "UPDATE evaluations SET created_at = $2 WHERE id = $1",
		stuck.ID, time.Now().Add(-RecoveryStaleAfter-time.Minute)); err != nil {
		t.Fatal(err)
	}
	m := seedRun(t, s.Pool)
	evaluation := beginAndComplete(t, s, m, aVerdict("complete", OverallMet))
	suggestion := seedSuggestion(t, s, m.run.WorkspaceID, evaluation.ID, "X")
	version := seedImprovedVersion(t, s.Pool, m.run.ID, 2, t.Name()+"-improved")
	publishVersionAdded(t, s, m, version, evaluation.ID, []pgtype.UUID{suggestion.ID},
		time.Now().Add(-RecoveryStaleAfter-time.Minute))

	if err := s.RecoverPending(context.Background()); err == nil {
		t.Error("recovery reported success although a stale evaluation could not be recovered")
	}
	if got := appliedSuggestionCount(t, s, m.run.WorkspaceID, version); got != 1 {
		t.Errorf("%d applications after the sweep, want 1: one stuck evaluation kept the provenance sweep from running", got)
	}
}
