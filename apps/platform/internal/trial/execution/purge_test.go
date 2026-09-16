package run

import (
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestAnAccountPurgeWaitsForSettledRunsAndProvablyClosedGrants(t *testing.T) {
	readiness := accountPurgeReadiness(pgtype.UUID{Bytes: [16]byte{1}, Valid: true})
	if want := []string{"succeeded", "failed", "cancelled", "timed_out"}; !slices.Equal(readiness.TerminalStatuses, want) {
		t.Errorf("terminal statuses = %v, want %v", readiness.TerminalStatuses, want)
	}
	if readiness.SettledCleanupStatus != string(gen.RunCleanupStatusCleaned) {
		t.Errorf("settled cleanup status = %q, want cleaned", readiness.SettledCleanupStatus)
	}
	if !slices.Equal(readiness.UnprovableGrantStates, []string{"legacy_unknown"}) {
		t.Errorf("unprovable grant states = %v, want only legacy_unknown", readiness.UnprovableGrantStates)
	}
	if readiness.ClockTolerance.Microseconds != time.Minute.Microseconds() {
		t.Errorf("clock tolerance = %dµs, want one minute", readiness.ClockTolerance.Microseconds)
	}
}

func TestARecordedAttemptsPossibleUploadIsDueWhenItsArtifactsExpire(t *testing.T) {
	var runID, attemptID pgtype.UUID
	runID.Bytes[0], attemptID.Bytes[0] = 1, 2
	runID.Valid, attemptID.Valid = true, true
	expires := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	finite := artifactUploadIntent(gen.RunAttempt{
		ID: attemptID, RunID: runID, ObjectGrantsExpireAt: pgtype.Timestamptz{Time: expires, Valid: true},
	})

	wantKey := "run-artifacts/01000000-0000-0000-0000-000000000000/02000000-0000-0000-0000-000000000000/artifacts.tar"
	if finite.ObjectKey != wantKey || finite.RunAttemptID != attemptID {
		t.Fatalf("intent = %q for %v, want %q for the attempt", finite.ObjectKey, finite.RunAttemptID, wantKey)
	}
	if !finite.NotBefore.Time.Equal(expires.Add(90 * 24 * time.Hour)) {
		t.Fatalf("not before = %v, want 90 days after the grants expire", finite.NotBefore.Time)
	}
}
