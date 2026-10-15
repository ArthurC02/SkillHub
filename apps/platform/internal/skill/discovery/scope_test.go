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
	if _, err := s.CatalogReferenceFacts(ctx, "00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002"); err == nil {
		t.Error("CatalogReferenceFacts answered without knowing which workspaces are the catalog")
	}
}

func TestAnExposedSkillsDetailAsksOnlyForThatSkill(t *testing.T) {
	id := mustUUID(t, "00000000-0000-0000-0000-0000000000a1")
	var asked []pgtype.UUID
	s := &Service{ExposedSkills: func(_ context.Context, skillID pgtype.UUID) ([]ExposedSkill, error) {
		asked = append(asked, skillID)
		return nil, nil
	}}

	if _, found, err := s.exposedSkill(context.Background(), id); err != nil || found {
		t.Fatalf("exposedSkill = found %v, err %v; want not found when nothing is exposed", found, err)
	}
	if len(asked) != 1 || asked[0] != id {
		t.Errorf("exposure asked for %v, want only the requested skill", asked)
	}
}
