package eval

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func auditedProvenanceLosses(t *testing.T, s *Service, workspaceID pgtype.UUID) int {
	t.Helper()
	var n int
	err := s.Pool.QueryRow(context.Background(),
		"SELECT count(*) FROM audit_events WHERE workspace_id = $1 AND action = $2",
		workspaceID, actionSuggestionProvenanceLost).Scan(&n)
	if err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	return n
}

func TestLostSuggestionProvenanceIsAuditedOnlyWhenTheLastTryStillFails(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	m := seedRun(t, s.Pool)
	evaluation := beginAndComplete(t, s, m, aVerdict("complete", OverallMet))
	suggestion := seedSuggestion(t, s, m.run.WorkspaceID, evaluation.ID, "X")
	version := seedImprovedVersion(t, s.Pool, m.run.ID, 2, t.Name()+"-improved")
	ctx := context.Background()

	var unknown pgtype.UUID
	if err := unknown.Scan(uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	lost := SuggestionsAppliedArgs{
		WorkspaceID: m.run.WorkspaceID, SkillID: m.run.ID, SkillVersionID: version,
		EvaluationID: unknown, SuggestionIDs: []pgtype.UUID{suggestion.ID},
	}

	if err := s.ConsumeSuggestionsApplied(ctx, lost, false); err == nil {
		t.Fatal("an unknown evaluation was consumed without an error; the delivery would never be retried")
	}
	if got := auditedProvenanceLosses(t, s, m.run.WorkspaceID); got != 0 {
		t.Fatalf("%d provenance losses audited after a try that can still be repeated, want 0", got)
	}

	if err := s.ConsumeSuggestionsApplied(ctx, lost, true); err == nil {
		t.Fatal("the last try reported success although the evaluation is unknown")
	}
	if got := auditedProvenanceLosses(t, s, m.run.WorkspaceID); got != 1 {
		t.Fatalf("%d provenance losses audited after the last try failed, want 1", got)
	}

	applied := lost
	applied.EvaluationID = evaluation.ID
	if err := s.ConsumeSuggestionsApplied(ctx, applied, true); err != nil {
		t.Fatalf("recording the applied suggestions on the last try: %v", err)
	}
	if got := auditedProvenanceLosses(t, s, m.run.WorkspaceID); got != 1 {
		t.Fatalf("%d provenance losses audited after a successful last try, want the earlier 1 and no more", got)
	}
}

func TestTheAuditOnlyWaitsUntilTheDeliveryHasNoTryLeft(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	m := seedRun(t, s.Pool)
	version := seedImprovedVersion(t, s.Pool, m.run.ID, 2, t.Name()+"-improved")
	w := &SuggestionsAppliedWorker{Svc: s}
	ctx := context.Background()

	var unknown pgtype.UUID
	if err := unknown.Scan(uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	args := SuggestionsAppliedArgs{
		WorkspaceID: m.run.WorkspaceID, SkillID: m.run.ID, SkillVersionID: version, EvaluationID: unknown,
	}
	work := func(attempt int) {
		t.Helper()
		if err := w.Work(ctx, &river.Job[SuggestionsAppliedArgs]{
			JobRow: &rivertype.JobRow{Attempt: attempt, MaxAttempts: suggestionsAppliedAttempts},
			Args:   args,
		}); err == nil {
			t.Fatalf("attempt %d reported success although the evaluation is unknown", attempt)
		}
	}

	work(suggestionsAppliedAttempts - 1)
	if got := auditedProvenanceLosses(t, s, m.run.WorkspaceID); got != 0 {
		t.Fatalf("%d provenance losses audited while one delivery attempt is left, want 0", got)
	}

	work(suggestionsAppliedAttempts)
	if got := auditedProvenanceLosses(t, s, m.run.WorkspaceID); got != 1 {
		t.Fatalf("%d provenance losses audited on the final attempt, want 1", got)
	}
}
