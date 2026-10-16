package identity

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func fullyWiredPurge() *Service {
	purge := func(context.Context, pgx.Tx, pgtype.UUID) error { return nil }
	keys := func(context.Context, gen.DBTX, pgtype.UUID) ([]string, error) { return nil, nil }
	return &Service{
		WorkspacePurges: WorkspacePurges{
			PurgeAnalytics: purge, PurgeTestData: purge, PurgeRunArtifacts: purge, PurgeDownloads: purge,
			PurgeCreation: purge, PurgeGraders: purge, PurgePublications: purge, PurgeSkills: purge, PurgeImportSources: purge,
		},
		DatasetObjectKeys: keys, RunArtifactObjectKeys: keys, DownloadArtifactObjectKeys: keys,
		WorkspaceQuiescent: func(context.Context, gen.DBTX, pgtype.UUID) (bool, error) { return true, nil },
	}
}

func TestAnAccountPurgeMissingAnyOwnersStepRefusesToStart(t *testing.T) {
	if err := fullyWiredPurge().requirePurgeSteps(); err != nil {
		t.Fatalf("fully wired purge refused: %v", err)
	}
	for _, tc := range []struct {
		names  string
		unwire func(*Service)
	}{
		{"analytics", func(s *Service) { s.PurgeAnalytics = nil }},
		{"testlab", func(s *Service) { s.PurgeTestData = nil }},
		{"run", func(s *Service) { s.PurgeRunArtifacts = nil }},
		{"packaging", func(s *Service) { s.PurgeDownloads = nil }},
		{"creation", func(s *Service) { s.PurgeCreation = nil }},
		{"eval", func(s *Service) { s.PurgeGraders = nil }},
		{"publishing", func(s *Service) { s.PurgePublications = nil }},
		{"registry", func(s *Service) { s.PurgeSkills = nil }},
		{"ingest", func(s *Service) { s.PurgeImportSources = nil }},
		{"testlab (object keys)", func(s *Service) { s.DatasetObjectKeys = nil }},
		{"run (object keys)", func(s *Service) { s.RunArtifactObjectKeys = nil }},
		{"packaging (object keys)", func(s *Service) { s.DownloadArtifactObjectKeys = nil }},
		{"run (purge readiness)", func(s *Service) { s.WorkspaceQuiescent = nil }},
	} {
		t.Run(tc.names, func(t *testing.T) {
			s := fullyWiredPurge()
			tc.unwire(s)
			err := s.requirePurgeSteps()
			if err == nil || !strings.Contains(err.Error(), "not injected for "+tc.names+";") {
				t.Errorf("purge without %s = %v, want a refusal naming it", tc.names, err)
			}
		})
	}
}

func TestPurgingExpiredAccountsChecksItsStepsBeforeTouchingAnyAccount(t *testing.T) {
	unwired := fullyWiredPurge()
	unwired.PurgeGraders = nil
	if purged, err := unwired.PurgeExpiredAccounts(context.Background(), nil, 0, 1); err == nil || purged != 0 {
		t.Fatalf("PurgeExpiredAccounts without the eval step = %d, %v; want a refusal before any account is touched", purged, err)
	}
}
