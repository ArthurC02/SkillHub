package objreconcile

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// Nil Pool and nil Store are deliberate throughout: both checks under test must
// be reached before anything is queried, and a nil dereference would be a louder
// failure than a passing assertion.

func noMark(context.Context, pgx.Tx, pgtype.UUID) error  { return nil }
func noList(context.Context, int32) ([]Candidate, error) { return nil, nil }

func fullyWiredService() *Service {
	return &Service{
		ListExpiredArtifacts: noList,
		ListClaimedArtifacts: noList,
		ListClaimedDatasets:  noList,
		RecordArtifactPurged: noMark,
		RecordDatasetLost:    noMark,
	}
}

// The reads and corrections belong to packaging and testlab and arrive by injection
// (ADR-033 clearance path 4), which makes "nobody wired them" a silent failure
// mode by construction: they are plain function values, and a sweep that skipped
// its writes would log exactly what a clean sweep logs. So it refuses instead.
func TestSweepRefusesToRunWithoutAnyOwnerFunction(t *testing.T) {
	tests := []struct {
		name        string
		breakWiring func(*Service)
	}{
		{"expired artifact read", func(s *Service) { s.ListExpiredArtifacts = nil }},
		{"claimed artifact read", func(s *Service) { s.ListClaimedArtifacts = nil }},
		{"claimed dataset read", func(s *Service) { s.ListClaimedDatasets = nil }},
		{"artifact write", func(s *Service) { s.RecordArtifactPurged = nil }},
		{"dataset write", func(s *Service) { s.RecordDatasetLost = nil }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := fullyWiredService()
			tc.breakWiring(svc)
			if err := svc.Sweep(context.Background()); err == nil {
				t.Error("Sweep ran with an owner function missing")
			}
		})
	}
}

// A deployment with no object store is a working one: there is nothing to
// reconcile against, so the sweep does nothing and says so by succeeding. This
// pins the order of the two checks — putting the fail-closed one after it would
// let a mis-wired service hide behind an absent store.
func TestSweepWithoutAStoreIsANoOp(t *testing.T) {
	svc := fullyWiredService()
	if err := svc.Sweep(context.Background()); err != nil {
		t.Errorf("Sweep with no store: %v", err)
	}
}
