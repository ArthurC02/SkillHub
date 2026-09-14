package registry

import (
	"encoding/json"
	"errors"

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
	SourceID         pgtype.UUID
	ContentHash      string
	PackageObjectKey string
	Report           skillpkg.Report
}

type Redistribution string

const (
	RedistributionAllowed      Redistribution = "allowed"
	RedistributionBlocked      Redistribution = "blocked"
	RedistributionUnknown      Redistribution = "unknown"
	RedistributionSelfSupplied Redistribution = "self_supplied"
	RedistributionGenerated    Redistribution = "generated"
)

func AllRedistributions() []Redistribution {
	return []Redistribution{
		RedistributionAllowed, RedistributionBlocked, RedistributionUnknown,
		RedistributionSelfSupplied, RedistributionGenerated,
	}
}

type VersionContent struct {
	sourceID         pgtype.UUID
	contentHash      string
	packageObjectKey string
	manifest         []byte
	license          *string
	licenseSource    *string
	summary          string
	generated        bool
}

func SkillFromPackage(workspaceID pgtype.UUID, report skillpkg.Report, redistribution Redistribution) (*SkillRoot, error) {
	manifest, err := validatedManifest(report)
	if err != nil {
		return nil, err
	}
	return startSkill(gen.Skill{WorkspaceID: workspaceID, Name: manifest.Name, Summary: &manifest.Description}, redistribution), nil
}

func ContentFromPackage(v NewVersion, generated bool) (VersionContent, error) {
	manifest, err := validatedManifest(v.Report)
	if err != nil {
		return VersionContent{}, err
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return VersionContent{}, err
	}
	license, licenseSource := versionLicense(v.Report)
	return VersionContent{
		sourceID: v.SourceID, contentHash: v.ContentHash, packageObjectKey: v.PackageObjectKey,
		manifest: encoded, license: license, licenseSource: licenseSource,
		summary: manifest.Description, generated: generated,
	}, nil
}

func copiedContent(from gen.SkillVersion, generated bool) VersionContent {
	return VersionContent{
		contentHash: from.ContentHash, packageObjectKey: from.PackageObjectKey, manifest: from.Manifest,
		license: from.LicenseExpression, licenseSource: from.LicenseSource, generated: generated,
	}
}

func versionLicense(report skillpkg.Report) (expression, source *string) {
	if report.LicenseExpression == "" {
		return nil, nil
	}
	tier := string(report.LicenseSource)
	return &report.LicenseExpression, &tier
}
