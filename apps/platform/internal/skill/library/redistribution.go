package registry

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type RedistributionBefore struct {
	WorkspaceID    pgtype.UUID
	Redistribution string
}

func SetRedistribution(ctx context.Context, tx pgx.Tx, skillID pgtype.UUID, value string) (RedistributionBefore, error) {
	q := gen.New(tx)
	before, err := q.LockSkillForOperatorWrite(ctx, skillID)
	if errors.Is(err, pgx.ErrNoRows) {
		return RedistributionBefore{}, ErrNotFound
	}
	if err != nil {
		return RedistributionBefore{}, err
	}
	if err := q.SetSkillRedistribution(ctx, gen.SetSkillRedistributionParams{
		ID: skillID, Redistribution: value,
	}); err != nil {
		return RedistributionBefore{}, err
	}
	return RedistributionBefore{
		WorkspaceID: before.WorkspaceID, Redistribution: before.Redistribution,
	}, nil
}

func LicenseEvidence(ctx context.Context, tx pgx.Tx, skillID pgtype.UUID) (expression, source string, err error) {
	row, err := gen.New(tx).GetLatestVersionLicense(ctx, skillID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", err
	}
	if row.LicenseExpression != nil {
		expression = *row.LicenseExpression
	}
	if row.LicenseSource != nil {
		source = *row.LicenseSource
	}
	return expression, source, nil
}
