package objreconcile

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func noMark(context.Context, pgx.Tx, pgtype.UUID) error  { return nil }
func noList(context.Context, int32) ([]Candidate, error) { return nil, nil }
func noGuard(_ context.Context, _ string, action func(bool, pgx.Tx) error) error {
	return action(false, nil)
}

func fullyWiredService() *Service {
	return &Service{
		ListExpiredArtifacts:       noList,
		ListDownloadIntents:        noList,
		ListClaimedArtifacts:       noList,
		ListClaimedDatasets:        noList,
		RecordArtifactPurged:       noMark,
		RecordDownloadIntentPurged: noMark,
		RecordDatasetLost:          noMark,
		GuardArtifactRemoval:       noGuard,
	}
}

func TestSweepRefusesToRunWithoutAnyOwnerFunction(t *testing.T) {
	tests := []struct {
		name        string
		breakWiring func(*Service)
	}{
		{"expired artifact read", func(s *Service) { s.ListExpiredArtifacts = nil }},
		{"download intent read", func(s *Service) { s.ListDownloadIntents = nil }},
		{"claimed artifact read", func(s *Service) { s.ListClaimedArtifacts = nil }},
		{"claimed dataset read", func(s *Service) { s.ListClaimedDatasets = nil }},
		{"artifact write", func(s *Service) { s.RecordArtifactPurged = nil }},
		{"download intent write", func(s *Service) { s.RecordDownloadIntentPurged = nil }},
		{"dataset write", func(s *Service) { s.RecordDatasetLost = nil }},
		{"artifact removal guard", func(s *Service) { s.GuardArtifactRemoval = nil }},
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

func TestSweepWithoutAStoreIsANoOp(t *testing.T) {
	svc := fullyWiredService()
	if err := svc.Sweep(context.Background()); err != nil {
		t.Errorf("Sweep with no store: %v", err)
	}
}
