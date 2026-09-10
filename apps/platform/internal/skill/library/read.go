package registry

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type Skill struct {
	ID                  pgtype.UUID
	WorkspaceID         pgtype.UUID
	Name                string
	Summary             *string
	ForkedFromSkillID   pgtype.UUID
	ForkedFromVersionID pgtype.UUID
	TakedownAt          pgtype.Timestamptz
	AccessRestriction   *string
	Redistribution      string

	CurationTier     string
	CuratedVersionID pgtype.UUID

	Category *string

	CategorySource *string
}

type Version struct {
	ID                pgtype.UUID
	WorkspaceID       pgtype.UUID
	SkillID           pgtype.UUID
	SourceID          pgtype.UUID
	VersionNumber     int32
	ContentHash       string
	PackageObjectKey  string
	LicenseExpression *string
	CreatedAt         pgtype.Timestamptz
	LicenseSource     *string
}

type RuntimeCompatibility struct {
	Capability   string
	Runtime      string
	RuntimeImage string
	MeasuredAt   pgtype.Timestamptz
}

type PreviousVersion struct {
	ID            pgtype.UUID
	SkillID       pgtype.UUID
	VersionNumber int32
}

type LineageStep struct {
	ID                  pgtype.UUID
	SkillID             pgtype.UUID
	VersionNumber       int32
	ForkedFromVersionID pgtype.UUID
}

type OldestVersion struct {
	SourceID pgtype.UUID
}

func SkillByName(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID, name string) (Skill, bool, error) {
	row, err := gen.New(tx).GetSkillByName(ctx, gen.GetSkillByNameParams{
		WorkspaceID: workspaceID, Name: name,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Skill{}, false, nil
	}
	if err != nil {
		return Skill{}, false, err
	}
	return skillDTO(row), true, nil
}

func SkillByID(ctx context.Context, tx pgx.Tx, workspaceID, skillID pgtype.UUID) (Skill, bool, error) {
	row, err := gen.New(tx).GetSkill(ctx, gen.GetSkillParams{ID: skillID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Skill{}, false, nil
	}
	if err != nil {
		return Skill{}, false, err
	}
	return skillDTO(row), true, nil
}

func VersionByContent(
	ctx context.Context, tx pgx.Tx, workspaceID, skillID pgtype.UUID, contentHash string,
) (Version, bool, error) {
	row, err := gen.New(tx).GetVersionBySkillAndHash(ctx, gen.GetVersionBySkillAndHashParams{
		WorkspaceID: workspaceID, SkillID: skillID, ContentHash: contentHash,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, false, nil
	}
	if err != nil {
		return Version{}, false, err
	}
	return versionDTO(row), true, nil
}

func (s *Service) CatalogSkill(ctx context.Context, skillID pgtype.UUID) (Skill, bool, error) {
	row, err := gen.New(s.Pool).GetCatalogSkill(ctx, skillID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Skill{}, false, nil
	}
	if err != nil {
		return Skill{}, false, err
	}
	return skillDTO(row), true, nil
}

func (s *Service) WorkspaceSkill(ctx context.Context, workspaceID, skillID pgtype.UUID) (Skill, bool, error) {
	row, err := gen.New(s.Pool).GetSkill(ctx, gen.GetSkillParams{ID: skillID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Skill{}, false, nil
	}
	if err != nil {
		return Skill{}, false, err
	}
	return skillDTO(row), true, nil
}

func (s *Service) LatestVersion(ctx context.Context, workspaceID, skillID pgtype.UUID) (Version, bool, error) {
	row, err := gen.New(s.Pool).GetLatestSkillVersion(ctx, gen.GetLatestSkillVersionParams{
		SkillID: skillID, WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, false, nil
	}
	if err != nil {
		return Version{}, false, err
	}
	return versionDTO(row), true, nil
}

func (s *Service) WorkspaceVersion(ctx context.Context, workspaceID, versionID pgtype.UUID) (Version, bool, error) {
	row, err := gen.New(s.Pool).GetSkillVersion(ctx, gen.GetSkillVersionParams{
		ID: versionID, WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Version{}, false, nil
	}
	if err != nil {
		return Version{}, false, err
	}
	return versionDTO(row), true, nil
}

func (s *Service) RuntimeCompatibility(ctx context.Context, versionID pgtype.UUID) (RuntimeCompatibility, bool, error) {
	row, err := gen.New(s.Pool).GetSkillRuntimeCompatibility(ctx, versionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return RuntimeCompatibility{}, false, nil
	}
	if err != nil {
		return RuntimeCompatibility{}, false, err
	}
	return RuntimeCompatibility{
		Capability: row.Capability, Runtime: row.Runtime,
		RuntimeImage: row.RuntimeImage, MeasuredAt: row.MeasuredAt,
	}, true, nil
}

func (s *Service) PreviousVersion(
	ctx context.Context, workspaceID, skillID pgtype.UUID, versionNumber int32,
) (PreviousVersion, bool, error) {
	row, err := gen.New(s.Pool).GetPreviousSkillVersion(ctx, gen.GetPreviousSkillVersionParams{
		SkillID: skillID, WorkspaceID: workspaceID, VersionNumber: versionNumber,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return PreviousVersion{}, false, nil
	}
	if err != nil {
		return PreviousVersion{}, false, err
	}
	return PreviousVersion{ID: row.ID, SkillID: row.SkillID, VersionNumber: row.VersionNumber}, true, nil
}

func (s *Service) VersionLineage(ctx context.Context, versionID pgtype.UUID) (LineageStep, bool, error) {
	row, err := gen.New(s.Pool).GetVersionLineage(ctx, versionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return LineageStep{}, false, nil
	}
	if err != nil {
		return LineageStep{}, false, err
	}
	return LineageStep{
		ID: row.ID, SkillID: row.SkillID, VersionNumber: row.VersionNumber,
		ForkedFromVersionID: row.ForkedFromVersionID,
	}, true, nil
}

func (s *Service) OldestVersion(ctx context.Context, skillID pgtype.UUID) (OldestVersion, bool, error) {
	row, err := gen.New(s.Pool).GetOldestSkillVersion(ctx, skillID)
	if errors.Is(err, pgx.ErrNoRows) {
		return OldestVersion{}, false, nil
	}
	if err != nil {
		return OldestVersion{}, false, err
	}
	return OldestVersion{SourceID: row.SourceID}, true, nil
}

func skillDTO(row gen.Skill) Skill {
	return Skill{
		ID: row.ID, WorkspaceID: row.WorkspaceID, Name: row.Name, Summary: row.Summary,
		ForkedFromSkillID: row.ForkedFromSkillID, ForkedFromVersionID: row.ForkedFromVersionID,
		TakedownAt: row.TakedownAt, AccessRestriction: row.AccessRestriction,
		Redistribution: row.Redistribution,
		CurationTier:   row.CurationTier, CuratedVersionID: row.CuratedVersionID,
		Category: row.Category, CategorySource: row.CategorySource,
	}
}

func versionDTO(row gen.SkillVersion) Version {
	return Version{
		ID: row.ID, WorkspaceID: row.WorkspaceID, SkillID: row.SkillID, SourceID: row.SourceID,
		VersionNumber: row.VersionNumber, ContentHash: row.ContentHash,
		PackageObjectKey: row.PackageObjectKey, LicenseExpression: row.LicenseExpression,
		CreatedAt: row.CreatedAt, LicenseSource: row.LicenseSource,
	}
}
