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
