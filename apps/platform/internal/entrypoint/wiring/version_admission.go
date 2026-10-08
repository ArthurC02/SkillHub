package wiring

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	registry "github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func WireRunVersionAdmission(runs *run.Service, registryService *registry.Service) {
	runs.VersionAdmission = runVersionAdmission{registryService}
}

type runVersionAdmission struct{ service *registry.Service }

func (a runVersionAdmission) Disabled(ctx context.Context, workspaceID, versionID pgtype.UUID) (bool, error) {
	disabled, err := a.service.VersionDisabled(ctx, workspaceID, versionID)
	if errors.Is(err, registry.ErrNotFound) {
		return false, run.ErrPreflightTargetNotFound
	}
	return disabled, err
}

func (a runVersionAdmission) Admit(ctx context.Context, tx pgx.Tx, workspaceID, versionID pgtype.UUID) error {
	err := a.service.AdmitVersion(ctx, tx, workspaceID, versionID)
	switch {
	case errors.Is(err, registry.ErrNotFound):
		return run.ErrPreflightTargetNotFound
	case errors.Is(err, registry.ErrVersionDisabled):
		return run.ErrVersionDisabled
	default:
		return err
	}
}
