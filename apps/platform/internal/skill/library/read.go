package registry

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
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

func (s Skill) TakenDown() bool { return s.TakedownAt.Valid }

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

type SkillListing struct {
	Skills    []ListedSkill
	Total     int64
	Truncated bool
}

type ListedSkill struct {
	Skill        Skill
	Risk         json.RawMessage
	Verification ScanVerification
}

type ScanVerification struct {
	State        ScanState
	ScannedAt    pgtype.Timestamptz
	AncestorName string
}

type ScanState string

const (
	ScanNotApplicable ScanState = "not_applicable"
	ScanNotMeasured   ScanState = "not_measured"
	ScanMeasured      ScanState = "measured"
	ScanInherited     ScanState = "inherited"
)

const skillListingLimit = 100

func (s *Service) Listing(ctx context.Context, workspaceID pgtype.UUID) (SkillListing, error) {
	catalogs, err := s.catalogWorkspaceIDs(ctx, s.Pool)
	if err != nil {
		return SkillListing{}, err
	}
	q := gen.New(s.Pool)
	rows, err := q.ListSkills(ctx, gen.ListSkillsParams{
		WorkspaceID: workspaceID, RowLimit: skillListingLimit + 1,
	})
	if err != nil {
		return SkillListing{}, err
	}
	listing := SkillListing{Truncated: len(rows) > skillListingLimit}
	if len(rows) > 0 {
		listing.Total = rows[0].TotalMatches
	}
	if listing.Truncated {
		rows = rows[:skillListingLimit]
	}
	ancestors, err := readScanAncestors(ctx, q, rows, catalogs)
	if err != nil {
		return SkillListing{}, err
	}
	if s.SkillRisks == nil || s.CatalogSkillRisks == nil {
		return SkillListing{}, errors.New("registry: skill risk reads not injected")
	}
	ids := make([]pgtype.UUID, 0, len(rows))
	ancestorIDs := make([]pgtype.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.Skill.ID)
		if ancestor, ok := ancestors[row.Skill.ID]; ok {
			ancestorIDs = append(ancestorIDs, ancestor.SkillID)
		}
	}
	risks, err := s.SkillRisks(ctx, workspaceID, ids)
	if err != nil {
		return SkillListing{}, err
	}
	inherited, err := s.CatalogSkillRisks(ctx, ancestorIDs)
	if err != nil {
		return SkillListing{}, err
	}
	listing.Skills = make([]ListedSkill, 0, len(rows))
	for _, row := range rows {
		ancestor, inherits := ancestors[row.Skill.ID]
		risk := risks[pgconv.UUIDString(row.Skill.ID)]
		if inherits {
			risk = inherited[pgconv.UUIDString(ancestor.SkillID)]
		}
		listing.Skills = append(listing.Skills, ListedSkill{
			Skill: skillDTO(row.Skill), Risk: risk, Verification: scanVerificationOf(row, ancestor, inherits),
		})
	}
	return listing, nil
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
	row, err := s.catalogSkillIn(ctx, s.Pool, skillID)
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

func (s *Service) LockLiveWorkspaceSkill(ctx context.Context, tx pgx.Tx, workspaceID, skillID pgtype.UUID) (Skill, bool, error) {
	root, err := LoadSkill(ctx, tx, workspaceID, skillID)
	if errors.Is(err, ErrNotFound) {
		return Skill{}, false, nil
	}
	if err != nil {
		return Skill{}, false, err
	}
	return root.Skill(), true, nil
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

func (s *Service) Versions(ctx context.Context, workspaceID, skillID pgtype.UUID) ([]Version, error) {
	rows, err := gen.New(s.Pool).ListSkillVersions(ctx, gen.ListSkillVersionsParams{
		WorkspaceID: workspaceID, SkillID: skillID,
	})
	if err != nil {
		return nil, err
	}
	versions := make([]Version, len(rows))
	for i, row := range rows {
		versions[i] = versionDTO(row)
	}
	return versions, nil
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

type VersionSummary struct {
	ID                  pgtype.UUID
	SkillID             pgtype.UUID
	SkillName           string
	VersionNumber       int32
	LatestVersionNumber int32
	AccessRestriction   *string
	Redistribution      string
}

func (s *Service) VersionSummaries(
	ctx context.Context, workspaceID pgtype.UUID, versionIDs []pgtype.UUID,
) (map[pgtype.UUID]VersionSummary, error) {
	summaries := make(map[pgtype.UUID]VersionSummary, len(versionIDs))
	if len(versionIDs) == 0 {
		return summaries, nil
	}
	rows, err := gen.New(s.Pool).ListVersionSummaries(ctx, gen.ListVersionSummariesParams{
		WorkspaceID: workspaceID, VersionIds: versionIDs,
	})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		summaries[row.ID] = VersionSummary{
			ID: row.ID, SkillID: row.SkillID, SkillName: row.SkillName,
			VersionNumber: row.VersionNumber, LatestVersionNumber: row.LatestVersionNumber,
			AccessRestriction: row.AccessRestriction, Redistribution: row.Redistribution,
		}
	}
	return summaries, nil
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
		ID: row.ID, WorkspaceID: row.WorkspaceID, Name: row.Name, Summary: pgconv.Clone(row.Summary),
		ForkedFromSkillID: row.ForkedFromSkillID, ForkedFromVersionID: row.ForkedFromVersionID,
		TakedownAt: row.TakedownAt, AccessRestriction: pgconv.Clone(row.AccessRestriction),
		Redistribution: row.Redistribution,
		CurationTier:   row.CurationTier, CuratedVersionID: row.CuratedVersionID,
		Category: pgconv.Clone(row.Category), CategorySource: pgconv.Clone(row.CategorySource),
	}
}

func versionDTO(row gen.SkillVersion) Version {
	return Version{
		ID: row.ID, WorkspaceID: row.WorkspaceID, SkillID: row.SkillID, SourceID: row.SourceID,
		VersionNumber: row.VersionNumber, ContentHash: row.ContentHash,
		PackageObjectKey: row.PackageObjectKey, LicenseExpression: pgconv.Clone(row.LicenseExpression),
		CreatedAt: row.CreatedAt, LicenseSource: pgconv.Clone(row.LicenseSource),
	}
}

type Governance struct {
	ID                pgtype.UUID
	WorkspaceID       pgtype.UUID
	Name              string
	AccessRestriction *string
	Redistribution    string
	TakedownAt        pgtype.Timestamptz
	TakedownReason    *string
}

const governanceLookupLimit = 20

func SkillsForGovernance(ctx context.Context, db gen.DBTX, skillID pgtype.UUID, namePart string) ([]Governance, error) {
	rows, err := gen.New(db).FindSkillsForGovernance(ctx, gen.FindSkillsForGovernanceParams{
		SkillID: skillID, NamePart: namePart, ResultLimit: governanceLookupLimit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Governance, 0, len(rows))
	for _, row := range rows {
		out = append(out, Governance(row))
	}
	return out, nil
}
