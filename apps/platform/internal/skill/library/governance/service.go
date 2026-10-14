package governance

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

var errSkillNotFound = errors.New("找不到這個 Skill")

var errHookMissing = errors.New("governance: a catalog or identity hook was not injected")

type Service struct {
	Pool *pgxpool.Pool

	Registry *registry.Service

	CatalogWorkspaces func(ctx context.Context, db gen.DBTX) ([]pgtype.UUID, error)

	RestrictionNote       func(reason string) string
	RedistributionDisplay func(value string) (label, note string)
}

type Handler struct {
	Svc *Service
}

func (s *Service) catalogWorkspaceIDs(ctx context.Context) ([]pgtype.UUID, error) {
	if s.CatalogWorkspaces == nil {
		return nil, errHookMissing
	}
	return s.CatalogWorkspaces(ctx, s.Pool)
}

func (s *Service) removeFromIndex(ctx context.Context, tx pgx.Tx, workspaceID, skillID pgtype.UUID) error {
	if s.Registry == nil || s.Registry.RemoveFromIndex == nil {
		return errHookMissing
	}
	return s.Registry.RemoveFromIndex(ctx, tx, workspaceID, skillID)
}

func (s *Service) refreshListing(ctx context.Context, db gen.DBTX, skillID pgtype.UUID) error {
	if s.Registry == nil || s.Registry.RefreshListing == nil {
		return errHookMissing
	}
	return s.Registry.RefreshListing(ctx, db, skillID)
}
