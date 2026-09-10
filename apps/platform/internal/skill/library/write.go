package registry

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

var ErrUnvalidatedPackage = errors.New("registry: package report is not a passing validation result")

func validatedManifest(report skillpkg.Report) (*skillpkg.Manifest, error) {
	if report.Blocked || report.Manifest == nil {
		return nil, ErrUnvalidatedPackage
	}
	return report.Manifest, nil
}

type NewVersion struct {
	WorkspaceID      pgtype.UUID
	SkillID          pgtype.UUID
	SourceID         pgtype.UUID
	ContentHash      string
	PackageObjectKey string
	Report           skillpkg.Report
}

const RedistributionSelfSupplied = "self_supplied"

const RedistributionGenerated = "generated"

func CreateSkillFromPackage(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID, report skillpkg.Report, redistribution string) (Skill, error) {
	manifest, err := validatedManifest(report)
	if err != nil {
		return Skill{}, err
	}
	var verdict *string
	if redistribution != "" {
		verdict = &redistribution
	}

	row, err := gen.New(tx).CreateSkill(ctx, gen.CreateSkillParams{
		WorkspaceID:    workspaceID,
		Name:           manifest.Name,
		Summary:        &manifest.Description,
		Redistribution: verdict,
	})
	if err != nil {
		return Skill{}, err
	}
	return skillDTO(row), nil
}

func CreateVersionFromPackage(ctx context.Context, tx pgx.Tx, v NewVersion) (Version, error) {
	manifest, err := validatedManifest(v.Report)
	if err != nil {
		return Version{}, err
	}

	encoded, err := json.Marshal(manifest)
	if err != nil {
		return Version{}, err
	}
	license, licenseSource := versionLicense(v.Report)
	row, err := gen.New(tx).CreateSkillVersion(ctx, gen.CreateSkillVersionParams{
		WorkspaceID:       v.WorkspaceID,
		SkillID:           v.SkillID,
		SourceID:          v.SourceID,
		ContentHash:       v.ContentHash,
		PackageObjectKey:  v.PackageObjectKey,
		Manifest:          encoded,
		LicenseExpression: license,
		LicenseSource:     licenseSource,
	})
	if err != nil {
		return Version{}, err
	}
	return versionDTO(row), nil
}

func UpdateSummaryFromPackage(ctx context.Context, tx pgx.Tx, skillID, workspaceID pgtype.UUID, report skillpkg.Report) error {
	manifest, err := validatedManifest(report)
	if err != nil {
		return err
	}
	return gen.New(tx).UpdateSkillSummary(ctx, gen.UpdateSkillSummaryParams{
		ID: skillID, WorkspaceID: workspaceID, Summary: &manifest.Description,
	})
}

func versionLicense(report skillpkg.Report) (expression, source *string) {
	if report.LicenseExpression == "" {
		return nil, nil
	}
	return &report.LicenseExpression, &report.LicenseSource
}
