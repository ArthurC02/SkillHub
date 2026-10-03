package run

import (
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestEvaluationRunCarriesTheTerminalVerdict(t *testing.T) {
	want := map[gen.RunStatus]bool{
		gen.RunStatusQueued:       false,
		gen.RunStatusProvisioning: false,
		gen.RunStatusPreparing:    false,
		gen.RunStatusRunning:      false,
		gen.RunStatusEvaluating:   false,
		gen.RunStatusSucceeded:    true,
		gen.RunStatusFailed:       true,
		gen.RunStatusCancelled:    true,
		gen.RunStatusTimedOut:     true,
	}
	if len(want) != len(AllStatuses) {
		t.Fatalf("the table covers %d statuses, the product has %d; a new status needs a verdict here", len(want), len(AllStatuses))
	}
	for status, terminal := range want {
		t.Run(string(status), func(t *testing.T) {
			facts := evaluationRun(gen.Run{Status: status})
			if facts.Status != string(status) || facts.Terminal != terminal {
				t.Fatalf("status %s: got status %q, terminal %v; want terminal %v", status, facts.Status, facts.Terminal, terminal)
			}
		})
	}
}

func TestAnEvaluationReadsOnlyLiveArtifactsAndCountsWhyTheOthersAreMissing(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) pgtype.Timestamptz { return pgtype.Timestamptz{Time: now.Add(d), Valid: true} }
	rows := []gen.Artifact{
		{FileName: "live.csv", ExpiresAt: at(time.Nanosecond)},
		{FileName: "expiring-now.csv", ExpiresAt: at(0)},
		{FileName: "purged.csv", ExpiresAt: at(time.Hour), PurgedAt: at(-time.Minute)},
		{FileName: "deleted.csv", ExpiresAt: at(time.Hour), DeletedAt: at(-time.Minute)},
		{FileName: "deleted-and-expired.csv", ExpiresAt: at(-time.Hour), DeletedAt: at(-time.Minute)},
	}

	artifacts, absent := evaluationArtifacts(rows, now)

	if len(artifacts) != 1 || artifacts[0].FileName != "live.csv" {
		t.Fatalf("readable artifacts = %+v, want only live.csv", artifacts)
	}
	if absent != (EvaluationArtifactAbsence{Deleted: 2, Expired: 2}) {
		t.Fatalf("absent = %+v, want 2 deleted and 2 expired", absent)
	}
}
