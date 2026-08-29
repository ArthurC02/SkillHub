package registry

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// Skill is the Registry-owned shape exposed to bounded-context consumers.
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
	// CurationTier is the PDM-002 verdict and CuratedVersionID is the version it
	// examined (0042). Both travel together on purpose: the verdict alone cannot
	// say whether it is still about the bytes a reader is looking at.
	CurationTier     string
	CuratedVersionID pgtype.UUID
}

// Version is Registry's immutable version fact.
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

// RuntimeCompatibility is the newest measured compatibility for one version.
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

// SkillByName reads through the caller's transaction so a Skill created earlier
// in the same import remains visible.
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

// SkillByID returns a workspace-scoped Skill through the caller's transaction.
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

// VersionByContent returns an existing immutable version in the same
// transaction used to create a replacement when no duplicate exists.
//
// workspaceID is taken even though skill_id already narrows the row to one
// skill: the query file this reads from states that every read there is
// workspace scoped, and a read that only happens to be safe because of who
// calls it today is the cross-tenant read waiting for its second caller.
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

// CatalogSkill returns only rows whose workspace is public catalog scope.
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

// WorkspaceSkill returns one live Skill from the caller's workspace.
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

// LatestVersion returns the newest immutable version under workspace scope.
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

// WorkspaceVersion returns one immutable version under workspace scope.
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

// RuntimeCompatibility returns the newest measurement for an already-scoped
// version. Absence is the normal "unverified" state.
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

// VersionLineage deliberately crosses workspace scope but exposes only lineage
// identifiers; fork ancestry necessarily lives in another workspace.
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
