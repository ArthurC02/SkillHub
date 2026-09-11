package catalog

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestCatalogReadsRefuseWithoutKnowingTheCatalogWorkspaces(t *testing.T) {
	ctx := context.Background()
	s := &Service{}
	if _, _, err := s.Browse(ctx, 10, searchFilters{}); err == nil {
		t.Error("Browse listed the catalog without knowing which workspaces are the catalog")
	}
	if _, err := s.CatalogSkillRisks(ctx, []pgtype.UUID{{}}); err == nil {
		t.Error("CatalogSkillRisks answered without knowing which workspaces are the catalog")
	}
	if _, _, _, err := s.CatalogReferenceFacts(ctx, "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"); err == nil {
		t.Error("CatalogReferenceFacts answered without knowing which workspaces are the catalog")
	}
}
