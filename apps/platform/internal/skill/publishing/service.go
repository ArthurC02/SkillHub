package publishing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

const (
	auditKeyName        = "name"
	auditKeyContentHash = "content_hash"
	auditKeyReleaseID   = "release_id"
)

type SkillFacts struct {
	ID               pgtype.UUID
	Name             string
	Summary          string
	TakenDown        bool
	AccessRestricted bool
	Redistribution   string
}

type VersionFacts struct {
	ID                pgtype.UUID
	SkillID           pgtype.UUID
	VersionNumber     int32
	ContentHash       string
	PackageObjectKey  string
	SourcePath        string
	LicenseExpression string
	LicenseSource     string
}

type PackageStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
}

type SkillRef struct {
	WorkspaceID pgtype.UUID
	SkillID     pgtype.UUID
}

type Service struct {
	Pool  *pgxpool.Pool
	Store PackageStore

	LockSkillForRelease func(ctx context.Context, tx pgx.Tx, workspaceID, skillID pgtype.UUID) (SkillFacts, bool, error)
	ReadSkill           func(ctx context.Context, workspaceID, skillID pgtype.UUID) (SkillFacts, bool, error)
	ReadSkills          func(ctx context.Context, refs []SkillRef) (map[SkillRef]SkillFacts, error)
	ReadSearchSnapshot  func(ctx context.Context, skillID pgtype.UUID) (SearchSnapshot, bool, error)
	ReadVersion         func(ctx context.Context, workspaceID, versionID pgtype.UUID) (VersionFacts, bool, error)
	LatestVersion       func(ctx context.Context, workspaceID, skillID pgtype.UUID) (VersionFacts, bool, error)

	PackageForRecipient       func(ctx context.Context, recipient identity.Workspace, ownerWorkspaceID, skillID, versionID pgtype.UUID) (Acquisition, error)
	PackagePluginForRecipient func(ctx context.Context, recipient identity.Workspace, ownerWorkspaceID pgtype.UUID, plugin PluginRequest) (Acquisition, error)
}

type Acquisition struct {
	ArtifactID  string
	FileName    string
	SizeBytes   int64
	ContentHash string
	ExpiresAt   string
	Duplicate   bool
}

type Publisher struct {
	Name      string
	CreatedAt time.Time
}

type Release struct {
	VersionID      pgtype.UUID
	VersionNumber  int32
	Bundle         *BundleVersion
	ContentHash    string
	Findings       skillpkg.CategorizedFindings
	RightsAttested bool
	ReleasedAt     time.Time
}

type Publication struct {
	Publisher         string
	Name              string
	SkillID           pgtype.UUID
	BundleID          pgtype.UUID
	Status            Status
	StatusChangedAt   time.Time
	Releases          []Release
	Availability      Availability
	UnavailableMember string
}

type PublicationSummary struct {
	SkillID           pgtype.UUID
	Publisher         string
	Name              string
	Status            Status
	StatusChangedAt   time.Time
	LatestRelease     *ReleaseSummary
	CatalogExposure   CatalogExposure
	Availability      Availability
	UnavailableMember string
}

type ReleaseSummary struct {
	VersionID     pgtype.UUID
	VersionNumber int32
	ReleasedAt    time.Time
}

type PublicPublication struct {
	Publication
	OwnerWorkspaceID pgtype.UUID
	Exposed          bool
	Skill            SkillFacts
	Version          VersionFacts
}

type publicationDelivery struct {
	Availability      Availability
	UnavailableMember string
	Skill             SkillFacts
	Version           VersionFacts
}

type PublishInput struct {
	Name           string
	VersionID      pgtype.UUID
	RightsAttested bool
}

func (in PublishInput) attestation() rightsAttestation {
	if in.RightsAttested {
		return rightsAttested
	}
	return rightsNotAttested
}

func (s *Service) Publisher(ctx context.Context, ws identity.Workspace) (Publisher, bool, error) {
	row, err := gen.New(s.Pool).GetPublisherByWorkspace(ctx, ws.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Publisher{}, false, nil
	}
	if err != nil {
		return Publisher{}, false, err
	}
	return Publisher{Name: row.Name, CreatedAt: row.CreatedAt.Time}, true, nil
}

func (s *Service) RegisterPublisher(ctx context.Context, ws identity.Workspace, name string) (Publisher, error) {
	if problem := publisherNameProblem(name); problem != "" {
		return Publisher{}, &NameError{Problem: problem}
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Publisher{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row, err := gen.New(tx).CreatePublisher(ctx, gen.CreatePublisherParams{WorkspaceID: ws.ID, Name: name})
	if constraint, violated := pgconv.UniqueViolation(err); violated {
		if constraint == "publishers_workspace_id_key" {
			return Publisher{}, ErrPublisherExists
		}
		return Publisher{}, ErrNameTaken
	}
	if err != nil {
		return Publisher{}, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor: ws.OwnerUserID, Workspace: ws.ID,
		Action: audit.ActionPublisherRegister, ResourceType: audit.ResourcePublisher, ResourceID: row.ID,
		Metadata: map[string]any{auditKeyName: row.Name},
	}); err != nil {
		return Publisher{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Publisher{}, err
	}
	return Publisher{Name: row.Name, CreatedAt: row.CreatedAt.Time}, nil
}

func (s *Service) Publish(ctx context.Context, ws identity.Workspace, skillID pgtype.UUID, in PublishInput) (Publication, error) {
	version, err := s.versionToRelease(ctx, ws, skillID, in.VersionID)
	if err != nil {
		return Publication{}, err
	}
	findings, err := s.scan(ctx, version)
	if err != nil {
		return Publication{}, err
	}
	if len(findings.Errors) > 0 {
		return Publication{}, &RefusedError{RefusedValidation, "這一版的套件沒有通過規格驗證，所以不能發佈"}
	}
	encodedFindings, err := json.Marshal(findings)
	if err != nil {
		return Publication{}, err
	}
	if err := s.release(ctx, ws, skillID, in, releaseCandidate{version: version, findings: encodedFindings}); err != nil {
		return Publication{}, err
	}
	own, _, err := s.OwnPublication(ctx, ws, skillID)
	return own, err
}

type releaseCandidate struct {
	version  VersionFacts
	findings []byte
}

func (s *Service) release(
	ctx context.Context, ws identity.Workspace, skillID pgtype.UUID, in PublishInput, candidate releaseCandidate,
) error {
	version := candidate.version
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	if err := lockPublisher(ctx, q, ws); err != nil {
		return err
	}
	skill, found, err := s.LockSkillForRelease(ctx, tx, ws.ID, skillID)
	if err != nil {
		return err
	}
	if !found || skill.TakenDown {
		return ErrNotFound
	}
	if refused := releaseGate(skill, in.attestation()); refused != nil {
		return refused
	}

	publication, err := publicationToRelease(ctx, q, ws, skill, in.Name)
	if err != nil {
		return err
	}
	if released, err := alreadyReleased(ctx, q, publication, version); err != nil || released {
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	release, err := q.InsertPublicationRelease(ctx, gen.InsertPublicationReleaseParams{
		PublicationID: publication.ID, WorkspaceID: ws.ID,
		SkillVersionID: version.ID, VersionNumber: &version.VersionNumber, ContentHash: version.ContentHash,
		Findings: candidate.findings, RightsAttested: in.RightsAttested, ReleasedBy: ws.OwnerUserID,
	})
	if err != nil {
		return err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor: ws.OwnerUserID, Workspace: ws.ID,
		Action: audit.ActionPublicationRelease, ResourceType: audit.ResourcePublication, ResourceID: publication.ID,
		Metadata: map[string]any{
			auditKeyName:        publication.Name,
			"skill_id":          pgconv.UUIDString(skill.ID),
			"version_id":        pgconv.UUIDString(version.ID),
			auditKeyContentHash: version.ContentHash,
			"rights_attested":   in.RightsAttested,
			auditKeyReleaseID:   pgconv.UUIDString(release.ID),
		},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func lockPublisher(ctx context.Context, q *gen.Queries, ws identity.Workspace) error {
	if _, err := q.LockPublisherByWorkspace(ctx, ws.ID); errors.Is(err, pgx.ErrNoRows) {
		return ErrNoPublisher
	} else if err != nil {
		return err
	}
	return nil
}

func (s *Service) versionToRelease(ctx context.Context, ws identity.Workspace, skillID, versionID pgtype.UUID) (VersionFacts, error) {
	var (
		version VersionFacts
		found   bool
		err     error
	)
	if versionID.Valid {
		version, found, err = s.ReadVersion(ctx, ws.ID, versionID)
		found = found && version.SkillID == skillID
	} else {
		version, found, err = s.LatestVersion(ctx, ws.ID, skillID)
	}
	if err != nil {
		return VersionFacts{}, err
	}
	if !found {
		return VersionFacts{}, ErrNotFound
	}
	return version, nil
}

func (s *Service) scan(ctx context.Context, version VersionFacts) (skillpkg.CategorizedFindings, error) {
	report, err := s.readPackage(ctx, version)
	if err != nil {
		return skillpkg.CategorizedFindings{}, err
	}
	return report.Categorize(), nil
}

func (s *Service) readPackage(ctx context.Context, version VersionFacts) (skillpkg.Report, error) {
	if s.Store == nil {
		return skillpkg.Report{}, errors.New("publishing: no object store is configured, so the package cannot be read")
	}
	data, err := s.Store.Get(ctx, version.PackageObjectKey)
	if err != nil {
		return skillpkg.Report{}, fmt.Errorf("stored package unreadable: %w", err)
	}
	fsys, err := skillpkg.SkillFS(data, version.SourcePath)
	if err != nil {
		return skillpkg.Report{}, fmt.Errorf("stored package unreadable: %w", err)
	}
	return skillpkg.Validate(fsys), nil
}

func publicationToRelease(
	ctx context.Context, q *gen.Queries, ws identity.Workspace, skill SkillFacts, requestedName string,
) (gen.Publication, error) {
	existing, err := q.LockPublicationForSkill(ctx, gen.LockPublicationForSkillParams{WorkspaceID: ws.ID, SkillID: skill.ID})
	if err == nil {
		return republished(ctx, q, ws, existing, requestedName)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return gen.Publication{}, err
	}
	name := requestedName
	if name == "" {
		name = skill.Name
	}
	if problem := publicationNameProblem(name); problem != "" {
		return gen.Publication{}, &NameError{Problem: problem}
	}
	created, err := q.CreatePublication(ctx, gen.CreatePublicationParams{
		Name: name, SkillID: skill.ID, Status: string(StatusPublished), WorkspaceID: ws.ID,
	})
	if pgconv.IsUniqueViolation(err) {
		return gen.Publication{}, ErrNameTaken
	}
	return created, err
}

func republished(ctx context.Context, q *gen.Queries, ws identity.Workspace, existing gen.Publication, requestedName string) (gen.Publication, error) {
	if requestedName != "" && requestedName != existing.Name {
		return gen.Publication{}, ErrNameIsPermanent
	}
	if _, err := q.SetPublicationStatus(ctx, gen.SetPublicationStatusParams{
		ID: existing.ID, Status: string(StatusPublished), WorkspaceID: ws.ID,
	}); err != nil {
		return gen.Publication{}, err
	}
	return existing, nil
}

func (s *Service) Delist(ctx context.Context, ws identity.Workspace, skillID pgtype.UUID) (Publication, error) {
	err := s.delist(ctx, ws, func(q *gen.Queries) (gen.Publication, error) {
		return q.LockPublicationForSkill(ctx, gen.LockPublicationForSkillParams{WorkspaceID: ws.ID, SkillID: skillID})
	}, map[string]any{"skill_id": pgconv.UUIDString(skillID)})
	if err != nil {
		return Publication{}, err
	}
	own, _, err := s.OwnPublication(ctx, ws, skillID)
	return own, err
}

func (s *Service) delist(
	ctx context.Context, ws identity.Workspace, lock func(*gen.Queries) (gen.Publication, error), subject map[string]any,
) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)
	publication, err := lock(q)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	changed, err := q.SetPublicationStatus(ctx, gen.SetPublicationStatusParams{
		ID: publication.ID, Status: string(StatusDelisted), WorkspaceID: ws.ID,
	})
	if err != nil {
		return err
	}
	if changed > 0 {
		subject["name"] = publication.Name
		if err := audit.Log(ctx, tx, audit.Event{
			Actor: ws.OwnerUserID, Workspace: ws.ID,
			Action: audit.ActionPublicationDelist, ResourceType: audit.ResourcePublication, ResourceID: publication.ID,
			Metadata: subject,
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Service) OwnPublication(ctx context.Context, ws identity.Workspace, skillID pgtype.UUID) (Publication, bool, error) {
	q := gen.New(s.Pool)
	row, err := q.GetPublicationForSkill(ctx, gen.GetPublicationForSkillParams{WorkspaceID: ws.ID, SkillID: skillID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Publication{}, false, nil
	}
	if err != nil {
		return Publication{}, false, err
	}
	releases, err := releasesOf(ctx, q, row.ID)
	if err != nil {
		return Publication{}, false, err
	}
	publication := Publication{
		Publisher: row.PublisherName, Name: row.Name, SkillID: row.SkillID,
		Status: Status(row.Status), StatusChangedAt: row.StatusChangedAt.Time, Releases: releases,
	}
	publication, err = s.withAvailability(ctx, ws.ID, publication)
	return publication, err == nil, err
}

func (s *Service) OwnPublications(ctx context.Context, ws identity.Workspace) ([]PublicationSummary, error) {
	rows, err := gen.New(s.Pool).ListWorkspaceSkillPublications(ctx, ws.ID)
	if err != nil {
		return nil, err
	}
	publications := make([]PublicationSummary, 0, len(rows))
	for _, row := range rows {
		publication := PublicationSummary{
			SkillID: row.SkillID, Publisher: row.PublisherName, Name: row.Name,
			Status: Status(row.Status), StatusChangedAt: row.StatusChangedAt.Time,
		}
		if row.LatestVersionID.Valid && row.LatestVersionNumber != nil && row.LatestReleasedAt.Valid {
			publication.LatestRelease = &ReleaseSummary{
				VersionID: row.LatestVersionID, VersionNumber: *row.LatestVersionNumber,
				ReleasedAt: row.LatestReleasedAt.Time,
			}
		}
		publication.CatalogExposure = CatalogExposure{State: CatalogUnreleased}
		if publication.LatestRelease != nil {
			publication.CatalogExposure, err = s.catalogExposure(ctx, ownerExposureStateOf(row))
			if err != nil {
				return nil, err
			}
		}
		var releases []Release
		if publication.LatestRelease != nil {
			releases = []Release{{VersionID: publication.LatestRelease.VersionID}}
		}
		delivery, err := s.publicationAvailability(ctx, ws.ID, Publication{
			SkillID: publication.SkillID, Status: publication.Status, Releases: releases,
		})
		if err != nil {
			return nil, err
		}
		publication.Availability = delivery.Availability
		publication.UnavailableMember = delivery.UnavailableMember
		publications = append(publications, publication)
	}
	return publications, nil
}

func (s *Service) withAvailability(ctx context.Context, owner pgtype.UUID, publication Publication) (Publication, error) {
	delivery, err := s.publicationAvailability(ctx, owner, publication)
	if err != nil {
		return Publication{}, err
	}
	publication.Availability = delivery.Availability
	publication.UnavailableMember = delivery.UnavailableMember
	return publication, nil
}

func (s *Service) publicationAvailability(
	ctx context.Context, owner pgtype.UUID, publication Publication,
) (publicationDelivery, error) {
	if publication.BundleID.Valid {
		var current *BundleVersion
		if len(publication.Releases) > 0 {
			current = publication.Releases[0].Bundle
		}
		availability, member, err := s.bundleAvailability(ctx, owner, publication.Status, current)
		return publicationDelivery{Availability: availability, UnavailableMember: member}, err
	}
	if publication.Status == StatusDelisted {
		return publicationDelivery{Availability: AvailabilityDelisted}, nil
	}
	skill, found, err := s.ReadSkill(ctx, owner, publication.SkillID)
	if err != nil {
		return publicationDelivery{}, err
	}
	availability := availabilityOf(publication.Status, skill, found)
	if availability != AvailabilityAvailable || len(publication.Releases) == 0 {
		if availability == AvailabilityAvailable {
			availability = AvailabilityWithdrawn
		}
		return publicationDelivery{Availability: availability, Skill: skill}, nil
	}
	version, found, err := s.ReadVersion(ctx, owner, publication.Releases[0].VersionID)
	if err != nil {
		return publicationDelivery{}, err
	}
	if !found {
		return publicationDelivery{Availability: AvailabilityWithdrawn, Skill: skill}, nil
	}
	return publicationDelivery{Availability: AvailabilityAvailable, Skill: skill, Version: version}, nil
}

func (s *Service) PublicPublication(ctx context.Context, publisherName, name string) (PublicPublication, bool, error) {
	q := gen.New(s.Pool)
	row, err := q.GetPublicPublication(ctx, gen.GetPublicPublicationParams{PublisherName: publisherName, Name: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicPublication{}, false, nil
	}
	if err != nil {
		return PublicPublication{}, false, err
	}
	releases, err := releasesOf(ctx, q, row.ID)
	if err != nil {
		return PublicPublication{}, false, err
	}
	out := PublicPublication{OwnerWorkspaceID: row.PublisherWorkspaceID, Publication: Publication{
		Publisher: row.PublisherName, Name: row.Name, SkillID: row.SkillID, BundleID: row.BundleID,
		Status: Status(row.Status), StatusChangedAt: row.StatusChangedAt.Time, Releases: releases,
	}}
	delivery, err := s.publicationAvailability(ctx, row.PublisherWorkspaceID, out.Publication)
	if err != nil {
		return PublicPublication{}, false, err
	}
	out.Availability = delivery.Availability
	out.UnavailableMember = delivery.UnavailableMember
	out.Skill = delivery.Skill
	out.Version = delivery.Version
	if row.BundleID.Valid {
		return out, true, nil
	}
	if out.Availability != AvailabilityAvailable {
		return out, true, nil
	}
	states, err := exposureStates(ctx, q, row.ID)
	if err != nil || len(states) == 0 {
		return out, err == nil, err
	}
	out.Exposed, _, err = s.exposedFor(ctx, states[0], out.Skill, true)
	return out, err == nil, err
}

func (s *Service) Acquire(ctx context.Context, recipient identity.Workspace, publisherName, name string) (Acquisition, error) {
	publication, found, err := s.PublicPublication(ctx, publisherName, name)
	if err != nil {
		return Acquisition{}, err
	}
	if !found {
		return Acquisition{}, ErrNotFound
	}
	if publication.Availability != AvailabilityAvailable {
		return Acquisition{}, &UnavailableError{Availability: publication.Availability, Member: publication.UnavailableMember}
	}
	if len(publication.Releases) == 0 {
		return Acquisition{}, ErrNotFound
	}
	if current := publication.Releases[0].Bundle; current != nil {
		return s.PackagePluginForRecipient(ctx, recipient, publication.OwnerWorkspaceID, pluginRequestOf(*current))
	}
	return s.PackageForRecipient(ctx, recipient, publication.OwnerWorkspaceID, publication.SkillID, publication.Releases[0].VersionID)
}

func PurgeWorkspace(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	q := gen.New(tx)
	if _, err := q.PurgeWorkspacePublications(ctx, workspaceID); err != nil {
		return err
	}
	_, err := q.PurgeWorkspaceBundles(ctx, workspaceID)
	return err
}

func releasesOf(ctx context.Context, q *gen.Queries, publicationID pgtype.UUID) ([]Release, error) {
	rows, err := q.ListPublicationReleases(ctx, publicationID)
	if err != nil {
		return nil, err
	}
	releases := make([]Release, 0, len(rows))
	var bundleVersionIDs []pgtype.UUID
	for _, row := range rows {
		var findings skillpkg.CategorizedFindings
		if err := json.Unmarshal(row.Findings, &findings); err != nil {
			return nil, fmt.Errorf("release %s findings: %w", pgconv.UUIDString(row.ID), err)
		}
		release := Release{
			VersionID: row.SkillVersionID, ContentHash: row.ContentHash,
			Findings: findings, RightsAttested: row.RightsAttested, ReleasedAt: row.ReleasedAt.Time,
		}
		if row.VersionNumber != nil {
			release.VersionNumber = *row.VersionNumber
		}
		if row.BundleVersionID.Valid {
			release.Bundle = &BundleVersion{ID: row.BundleVersionID}
			bundleVersionIDs = append(bundleVersionIDs, row.BundleVersionID)
		}
		releases = append(releases, release)
	}
	if len(bundleVersionIDs) == 0 {
		return releases, nil
	}
	versions, err := bundleVersionsByID(ctx, q, bundleVersionIDs)
	if err != nil {
		return nil, err
	}
	for i := range releases {
		if releases[i].Bundle != nil {
			version := versions[releases[i].Bundle.ID]
			releases[i].Bundle = &version
		}
	}
	return releases, nil
}

func alreadyReleased(ctx context.Context, q *gen.Queries, publication gen.Publication, version VersionFacts) (bool, error) {
	if publication.Status != string(StatusPublished) {
		return false, nil
	}
	releases, err := q.ListPublicationReleases(ctx, publication.ID)
	if err != nil || len(releases) == 0 {
		return false, err
	}
	latest := releases[0]
	return latest.SkillVersionID == version.ID && latest.ContentHash == version.ContentHash, nil
}
