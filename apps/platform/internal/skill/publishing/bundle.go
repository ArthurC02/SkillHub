package publishing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

var semverRule = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)

type BundleMember struct {
	SkillID       pgtype.UUID
	VersionID     pgtype.UUID
	VersionNumber int32
	Name          string
	ContentHash   string
}

type BundleVersion struct {
	ID          pgtype.UUID
	Bundle      string
	Version     string
	Description string
	ContentHash string
	CreatedAt   time.Time
	Members     []BundleMember
}

type BundleInput struct {
	Name             string
	Version          string
	Description      string
	MemberVersionIDs []pgtype.UUID
}

type BundlePublishInput struct {
	Name           string
	Version        string
	RightsAttested bool
}

type PluginRequest struct {
	Name        string
	Version     string
	Description string
	Members     []BundleMember
}

type BundleProblem string

const (
	BundleVersionShape       BundleProblem = "version_shape"
	BundleDescriptionMissing BundleProblem = "description_missing"
	BundleNoMembers          BundleProblem = "no_members"
	BundleDuplicateSkill     BundleProblem = "duplicate_skill"
	BundleDuplicateName      BundleProblem = "duplicate_manifest_name"
	BundleVersionExists      BundleProblem = "version_exists"
)

var bundleProblemWords = map[BundleProblem]string{
	BundleVersionShape:       "版本字串要是 semver：MAJOR.MINOR.PATCH，可以帶預發佈標籤，例如 1.0.0 或 1.1.0-beta.1",
	BundleDescriptionMissing: "Bundle 需要一段說明",
	BundleNoMembers:          "Bundle 至少要有一個成員",
	BundleDuplicateSkill:     "同一個 Bundle Version 裡不能有兩個成員來自同一個 Skill",
	BundleDuplicateName:      "兩個成員的名稱相同，匯出時會撞在同一個目錄",
	BundleVersionExists:      "這個 Bundle 已經有這個版本字串；Bundle Version 建立後不能改，請用新的版本字串",
}

type BundleError struct {
	Problem BundleProblem
	Member  string
}

func (e *BundleError) Error() string {
	if e.Member != "" {
		return bundleProblemWords[e.Problem] + "（" + e.Member + "）"
	}
	return bundleProblemWords[e.Problem]
}

func bundleInputProblem(in BundleInput) error {
	if problem := publicationNameProblem(in.Name); problem != "" {
		return &NameError{Problem: problem}
	}
	if !semverRule.MatchString(in.Version) {
		return &BundleError{Problem: BundleVersionShape}
	}
	if strings.TrimSpace(in.Description) == "" {
		return &BundleError{Problem: BundleDescriptionMissing}
	}
	if len(in.MemberVersionIDs) == 0 {
		return &BundleError{Problem: BundleNoMembers}
	}
	return nil
}

func bundleContentHash(members []BundleMember) string {
	h := sha256.New()
	for _, m := range members {
		h.Write([]byte(m.Name + "\t" + m.ContentHash + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Service) CreateBundleVersion(ctx context.Context, ws identity.Workspace, in BundleInput) (BundleVersion, error) {
	if err := bundleInputProblem(in); err != nil {
		return BundleVersion{}, err
	}
	members, err := s.resolveMembers(ctx, ws, in.MemberVersionIDs)
	if err != nil {
		return BundleVersion{}, err
	}
	out := BundleVersion{
		Bundle: in.Name, Version: in.Version, Description: strings.TrimSpace(in.Description),
		ContentHash: bundleContentHash(members), Members: members,
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return BundleVersion{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)
	if err := q.EnsureBundle(ctx, gen.EnsureBundleParams{WorkspaceID: ws.ID, Name: in.Name}); err != nil {
		return BundleVersion{}, err
	}
	bundle, err := q.LockBundle(ctx, gen.LockBundleParams{WorkspaceID: ws.ID, Name: in.Name})
	if err != nil {
		return BundleVersion{}, err
	}
	row, err := q.CreateBundleVersion(ctx, gen.CreateBundleVersionParams{
		Version: out.Version, Description: out.Description, ContentHash: out.ContentHash,
		CreatedBy: ws.OwnerUserID, BundleID: bundle.ID, WorkspaceID: ws.ID,
	})
	if violated, _ := uniqueViolation(err); violated {
		return BundleVersion{}, &BundleError{Problem: BundleVersionExists}
	}
	if err != nil {
		return BundleVersion{}, err
	}
	for i, m := range members {
		if err := q.InsertBundleMember(ctx, gen.InsertBundleMemberParams{
			SkillID: m.SkillID, SkillVersionID: m.VersionID, VersionNumber: m.VersionNumber,
			ManifestName: m.Name, ContentHash: m.ContentHash, Position: int32(i),
			BundleVersionID: row.ID, WorkspaceID: ws.ID,
		}); err != nil {
			return BundleVersion{}, err
		}
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor: ws.OwnerUserID, Workspace: ws.ID,
		Action: audit.ActionBundleVersionCreate, ResourceType: audit.ResourceBundle, ResourceID: bundle.ID,
		Metadata: map[string]any{
			"name": in.Name, "version": in.Version, "content_hash": out.ContentHash,
			"bundle_version_id": pgconv.UUIDString(row.ID), "members": len(members),
		},
	}); err != nil {
		return BundleVersion{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BundleVersion{}, err
	}
	out.ID, out.CreatedAt = row.ID, row.CreatedAt.Time
	return out, nil
}

func (s *Service) resolveMembers(ctx context.Context, ws identity.Workspace, versionIDs []pgtype.UUID) ([]BundleMember, error) {
	members := make([]BundleMember, 0, len(versionIDs))
	seenSkill, seenName := map[pgtype.UUID]bool{}, map[string]bool{}
	for _, id := range versionIDs {
		version, found, err := s.ReadVersion(ctx, ws.ID, id)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, ErrNotFound
		}
		skill, found, err := s.ReadSkill(ctx, ws.ID, version.SkillID)
		if err != nil {
			return nil, err
		}
		if !found || skill.TakenDown {
			return nil, ErrNotFound
		}
		if seenSkill[skill.ID] {
			return nil, &BundleError{Problem: BundleDuplicateSkill, Member: skill.Name}
		}
		report, err := s.readPackage(ctx, version)
		if err != nil {
			return nil, err
		}
		name := skill.Name
		if report.Manifest != nil && report.Manifest.Name != "" {
			name = report.Manifest.Name
		}
		if seenName[name] {
			return nil, &BundleError{Problem: BundleDuplicateName, Member: name}
		}
		seenSkill[skill.ID], seenName[name] = true, true
		members = append(members, BundleMember{
			SkillID: skill.ID, VersionID: version.ID, VersionNumber: version.VersionNumber,
			Name: name, ContentHash: version.ContentHash,
		})
	}
	return members, nil
}

func (s *Service) Bundles(ctx context.Context, ws identity.Workspace) ([]BundleVersion, error) {
	q := gen.New(s.Pool)
	rows, err := q.ListWorkspaceBundleVersions(ctx, ws.ID)
	if err != nil {
		return nil, err
	}
	ids := make([]pgtype.UUID, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	members, err := membersOf(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	out := make([]BundleVersion, len(rows))
	for i, row := range rows {
		out[i] = BundleVersion{
			ID: row.ID, Bundle: row.BundleName, Version: row.Version, Description: row.Description,
			ContentHash: row.ContentHash, CreatedAt: row.CreatedAt.Time, Members: members[row.ID],
		}
	}
	return out, nil
}

func membersOf(ctx context.Context, q *gen.Queries, bundleVersionIDs []pgtype.UUID) (map[pgtype.UUID][]BundleMember, error) {
	out := map[pgtype.UUID][]BundleMember{}
	if len(bundleVersionIDs) == 0 {
		return out, nil
	}
	rows, err := q.ListBundleMembers(ctx, bundleVersionIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.BundleVersionID] = append(out[row.BundleVersionID], BundleMember{
			SkillID: row.SkillID, VersionID: row.SkillVersionID, VersionNumber: row.VersionNumber,
			Name: row.ManifestName, ContentHash: row.ContentHash,
		})
	}
	return out, nil
}

func bundleVersionsByID(ctx context.Context, q *gen.Queries, ids []pgtype.UUID) (map[pgtype.UUID]BundleVersion, error) {
	rows, err := q.ListReleasedBundleVersions(ctx, ids)
	if err != nil {
		return nil, err
	}
	members, err := membersOf(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[pgtype.UUID]BundleVersion, len(rows))
	for _, row := range rows {
		out[row.ID] = BundleVersion{
			ID: row.ID, Bundle: row.BundleName, Version: row.Version, Description: row.Description,
			ContentHash: row.ContentHash, CreatedAt: row.CreatedAt.Time, Members: members[row.ID],
		}
	}
	return out, nil
}

func (s *Service) bundleVersion(ctx context.Context, q *gen.Queries, ws identity.Workspace, name, version string) (BundleVersion, error) {
	var (
		row gen.GetBundleVersionRow
		err error
	)
	if version == "" {
		var newest gen.GetNewestBundleVersionRow
		newest, err = q.GetNewestBundleVersion(ctx, gen.GetNewestBundleVersionParams{WorkspaceID: ws.ID, BundleName: name})
		row = gen.GetBundleVersionRow(newest)
	} else {
		row, err = q.GetBundleVersion(ctx, gen.GetBundleVersionParams{WorkspaceID: ws.ID, BundleName: name, Version: version})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return BundleVersion{}, ErrNotFound
	}
	if err != nil {
		return BundleVersion{}, err
	}
	members, err := membersOf(ctx, q, []pgtype.UUID{row.ID})
	if err != nil {
		return BundleVersion{}, err
	}
	return BundleVersion{
		ID: row.ID, Bundle: row.BundleName, Version: row.Version, Description: row.Description,
		ContentHash: row.ContentHash, CreatedAt: row.CreatedAt.Time, Members: members[row.ID],
	}, nil
}

func (s *Service) PublishBundle(ctx context.Context, ws identity.Workspace, bundleName string, in BundlePublishInput) (Publication, error) {
	bundleVersion, err := s.bundleVersion(ctx, gen.New(s.Pool), ws, bundleName, in.Version)
	if err != nil {
		return Publication{}, err
	}
	findings, err := s.scanMembers(ctx, ws, bundleVersion.Members)
	if err != nil {
		return Publication{}, err
	}
	encodedFindings, err := json.Marshal(findings)
	if err != nil {
		return Publication{}, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Publication{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)
	if _, err := q.LockPublisherByWorkspace(ctx, ws.ID); errors.Is(err, pgx.ErrNoRows) {
		return Publication{}, ErrNoPublisher
	} else if err != nil {
		return Publication{}, err
	}
	bundle, err := q.LockBundle(ctx, gen.LockBundleParams{WorkspaceID: ws.ID, Name: bundleName})
	if errors.Is(err, pgx.ErrNoRows) {
		return Publication{}, ErrNotFound
	}
	if err != nil {
		return Publication{}, err
	}
	if refused, err := s.memberGate(ctx, tx, ws, bundleVersion.Members, in.RightsAttested); err != nil || refused != nil {
		if refused != nil {
			return Publication{}, refused
		}
		return Publication{}, err
	}
	publication, err := bundlePublicationToRelease(ctx, q, ws, bundle, in.Name)
	if err != nil {
		return Publication{}, err
	}
	release, err := q.InsertBundleRelease(ctx, gen.InsertBundleReleaseParams{
		BundleVersionID: bundleVersion.ID, ContentHash: bundleVersion.ContentHash, Findings: encodedFindings,
		RightsAttested: in.RightsAttested, ReleasedBy: ws.OwnerUserID,
		PublicationID: publication.ID, WorkspaceID: ws.ID,
	})
	if err != nil {
		return Publication{}, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor: ws.OwnerUserID, Workspace: ws.ID,
		Action: audit.ActionPublicationRelease, ResourceType: audit.ResourcePublication, ResourceID: publication.ID,
		Metadata: map[string]any{
			"name":              publication.Name,
			"bundle":            bundleName,
			"bundle_version_id": pgconv.UUIDString(bundleVersion.ID),
			"version":           bundleVersion.Version,
			"content_hash":      bundleVersion.ContentHash,
			"rights_attested":   in.RightsAttested,
			"release_id":        pgconv.UUIDString(release.ID),
		},
	}); err != nil {
		return Publication{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Publication{}, err
	}
	own, _, err := s.OwnBundlePublication(ctx, ws, bundleName)
	return own, err
}

func (s *Service) scanMembers(ctx context.Context, ws identity.Workspace, members []BundleMember) (skillpkg.CategorizedFindings, error) {
	all := skillpkg.CategorizedFindings{Errors: []skillpkg.Finding{}, Warnings: []skillpkg.Finding{}, Infos: []skillpkg.Finding{}}
	for _, m := range members {
		version, found, err := s.ReadVersion(ctx, ws.ID, m.VersionID)
		if err != nil {
			return all, err
		}
		if !found {
			return all, memberWithdrawn(m.Name)
		}
		findings, err := s.scan(ctx, version)
		if err != nil {
			return all, err
		}
		if len(findings.Errors) > 0 {
			return all, &RefusedError{RefusedValidation, "成員 " + m.Name + "：這一版的套件沒有通過規格驗證，所以不能發佈"}
		}
		prefix := "skills/" + m.Name + "/"
		for _, bucket := range []struct{ from, to *[]skillpkg.Finding }{
			{&findings.Warnings, &all.Warnings}, {&findings.Infos, &all.Infos},
		} {
			for _, f := range *bucket.from {
				f.Path = prefix + f.Path
				*bucket.to = append(*bucket.to, f)
			}
		}
	}
	return all, nil
}

func (s *Service) memberGate(ctx context.Context, tx pgx.Tx, ws identity.Workspace, members []BundleMember, rightsAttested bool) (*RefusedError, error) {
	ordered := slices.Clone(members)
	slices.SortFunc(ordered, func(a, b BundleMember) int {
		return strings.Compare(pgconv.UUIDString(a.SkillID), pgconv.UUIDString(b.SkillID))
	})
	refusals := map[pgtype.UUID]*RefusedError{}
	for _, m := range ordered {
		skill, found, err := s.LockSkillForRelease(ctx, tx, ws.ID, m.SkillID)
		if err != nil {
			return nil, err
		}
		if !found || skill.TakenDown {
			refusals[m.SkillID] = memberWithdrawn(m.Name)
			continue
		}
		if refused := releaseGate(skill, rightsAttested); refused != nil {
			refusals[m.SkillID] = &RefusedError{refused.Reason, "成員 " + m.Name + "：" + refused.Message}
		}
	}
	for _, m := range members {
		if refused := refusals[m.SkillID]; refused != nil {
			return refused, nil
		}
	}
	return nil, nil
}

func memberWithdrawn(name string) *RefusedError {
	return &RefusedError{RefusedMemberWithdrawn, "成員 " + name + " 已經不在你的工作區，或已被平台下架"}
}

func bundlePublicationToRelease(ctx context.Context, q *gen.Queries, ws identity.Workspace, bundle gen.Bundle, requestedName string) (gen.Publication, error) {
	existing, err := q.LockPublicationForBundle(ctx, gen.LockPublicationForBundleParams{WorkspaceID: ws.ID, BundleID: bundle.ID})
	if err == nil {
		return republished(ctx, q, ws, existing, requestedName)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return gen.Publication{}, err
	}
	name := requestedName
	if name == "" {
		name = bundle.Name
	}
	if problem := publicationNameProblem(name); problem != "" {
		return gen.Publication{}, &NameError{Problem: problem}
	}
	created, err := q.CreateBundlePublication(ctx, gen.CreateBundlePublicationParams{
		Name: name, BundleID: bundle.ID, Status: string(StatusPublished), WorkspaceID: ws.ID,
	})
	if violated, _ := uniqueViolation(err); violated {
		return gen.Publication{}, ErrNameTaken
	}
	return created, err
}

func (s *Service) DelistBundle(ctx context.Context, ws identity.Workspace, bundleName string) (Publication, error) {
	bundle, err := gen.New(s.Pool).LockBundle(ctx, gen.LockBundleParams{WorkspaceID: ws.ID, Name: bundleName})
	if errors.Is(err, pgx.ErrNoRows) {
		return Publication{}, ErrNotFound
	}
	if err != nil {
		return Publication{}, err
	}
	err = s.delist(ctx, ws, func(q *gen.Queries) (gen.Publication, error) {
		return q.LockPublicationForBundle(ctx, gen.LockPublicationForBundleParams{WorkspaceID: ws.ID, BundleID: bundle.ID})
	}, map[string]any{"bundle": bundleName})
	if err != nil {
		return Publication{}, err
	}
	own, _, err := s.OwnBundlePublication(ctx, ws, bundleName)
	return own, err
}

func (s *Service) OwnBundlePublication(ctx context.Context, ws identity.Workspace, bundleName string) (Publication, bool, error) {
	q := gen.New(s.Pool)
	versions, err := q.ListWorkspaceBundleVersions(ctx, ws.ID)
	if err != nil {
		return Publication{}, false, err
	}
	var bundleID pgtype.UUID
	for _, v := range versions {
		if v.BundleName == bundleName {
			bundleID = v.BundleID
			break
		}
	}
	if !bundleID.Valid {
		return Publication{}, false, nil
	}
	row, err := q.GetPublicationForBundle(ctx, gen.GetPublicationForBundleParams{WorkspaceID: ws.ID, BundleID: bundleID})
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
	return Publication{
		Publisher: row.PublisherName, Name: row.Name, BundleID: row.BundleID,
		Status: Status(row.Status), StatusChangedAt: row.StatusChangedAt.Time, Releases: releases,
	}, true, nil
}

func (s *Service) ExportBundle(ctx context.Context, ws identity.Workspace, bundleName, version string) (Acquisition, error) {
	bundleVersion, err := s.bundleVersion(ctx, gen.New(s.Pool), ws, bundleName, version)
	if err != nil {
		return Acquisition{}, err
	}
	return s.PackagePluginForRecipient(ctx, ws, ws.ID, pluginRequestOf(bundleVersion))
}

func pluginRequestOf(v BundleVersion) PluginRequest {
	return PluginRequest{Name: v.Bundle, Version: v.Version, Description: v.Description, Members: v.Members}
}

func (s *Service) bundleAvailability(ctx context.Context, owner pgtype.UUID, status Status, current *BundleVersion) (Availability, string, error) {
	if status == StatusDelisted {
		return AvailabilityDelisted, "", nil
	}
	if current == nil {
		return AvailabilityWithdrawn, "", nil
	}
	for _, m := range current.Members {
		skill, found, err := s.ReadSkill(ctx, owner, m.SkillID)
		if err != nil {
			return "", "", err
		}
		if availability := availabilityOf(status, skill, found); availability != AvailabilityAvailable {
			return availability, m.Name, nil
		}
		if _, found, err := s.ReadVersion(ctx, owner, m.VersionID); err != nil || !found {
			return AvailabilityWithdrawn, m.Name, err
		}
	}
	return AvailabilityAvailable, "", nil
}

func SkillVersionsInBundles(ctx context.Context, db gen.DBTX, versionIDs []pgtype.UUID) ([]pgtype.UUID, error) {
	return gen.New(db).ListSkillVersionsInBundles(ctx, versionIDs)
}
