package registry

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestCuratedAncestorDoesNotGuessWhenCatalogWorkspacesCannotBeRead(t *testing.T) {
	outage := errors.New("catalog workspaces unavailable")
	svc := &Service{Pool: closedCuratedPool(t), CatalogWorkspaces: func(context.Context, gen.DBTX) ([]pgtype.UUID, error) { return nil, outage }}
	source, found, err := svc.CatalogReads().Ancestor(t.Context(), pgtype.UUID{}, pgtype.UUID{})
	if !errors.Is(err, outage) || found || source != (SkillRef{}) {
		t.Fatalf("unreadable catalog became an absent ancestor: %+v %v %v", source, found, err)
	}
}

func TestCuratedAncestorDoesNotGuessWhenLineageCannotBeRead(t *testing.T) {
	svc := &Service{Pool: closedCuratedPool(t), CatalogWorkspaces: func(context.Context, gen.DBTX) ([]pgtype.UUID, error) {
		return []pgtype.UUID{{Bytes: [16]byte{1}, Valid: true}}, nil
	}}
	source, found, err := svc.CatalogReads().Ancestor(t.Context(), pgtype.UUID{}, pgtype.UUID{})
	if err == nil || !strings.Contains(err.Error(), "closed pool") || found || source != (SkillRef{}) {
		t.Fatalf("unreadable lineage became an absent ancestor: %+v %v %v", source, found, err)
	}
}

func closedCuratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), "postgres://unused@127.0.0.1:1/unused?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	return pool
}
