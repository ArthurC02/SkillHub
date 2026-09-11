package packaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

const PackagerVersion = "0.2.0"

const objectCleanupTimeout = 5 * time.Second

const (
	BlockedLicenseHold        = "license_hold"
	BlockedNotRedistributable = "not_redistributable"
	BlockedLicenseUnknown     = "license_unknown"
	BlockedValidation         = "validation_blocked"

	BlockedFileRemoved = "file_removed_by_packager"
)

const (
	RedistributionAllowed      = "allowed"
	RedistributionBlocked      = "blocked"
	RedistributionSelfSupplied = "self_supplied"
	RedistributionGenerated    = "generated"
)

var (
	ErrNotFound = errors.New("skill version not found")

	ErrUnknownTarget = errors.New("unknown packaging target")

	ErrNoProfile = errors.New("this deployment has no configuration for that packaging target")

	ErrNoStore = errors.New("no object store is configured, so package bytes cannot be read")

	ErrRetentionNotConfigured = policy.ErrRetentionNotConfigured
	errOwnerReadNotConfigured = errors.New("packaging: owner reads not injected")
	errVersionSummaryMissing  = errors.New("packaging: a download's skill version is gone")
)

type ObjectStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key string, data []byte) error
	Remove(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

type Service struct {
	Pool  *pgxpool.Pool
	Store ObjectStore

	TestLab         *testlab.Service
	MayStoreObjects func(context.Context, gen.DBTX, pgtype.UUID) (bool, error)

	ClearSightings func(ctx context.Context, tx pgx.Tx, ids []pgtype.UUID) error

	Profiles Profiles

	Retention policy.DownloadRetention

	AppliedSuggestions func(ctx context.Context, versionID, workspaceID pgtype.UUID) ([]AppliedSuggestion, error)
	SourceLineage      func(ctx context.Context, sourceID pgtype.UUID) (LineageSource, error)

	CuratedSource     func(ctx context.Context, skillID pgtype.UUID) (CuratedSource, bool, error)
	ReadSkill         func(context.Context, pgtype.UUID, pgtype.UUID) (SkillFacts, bool, error)
	ReadVersion       func(context.Context, pgtype.UUID, pgtype.UUID) (VersionFacts, bool, error)
	ReadCompatibility func(context.Context, pgtype.UUID) (RuntimeCompatibility, bool, error)
	ReadPrevious      func(context.Context, pgtype.UUID, pgtype.UUID, int32) (PreviousVersion, bool, error)
	ReadLineage       func(context.Context, pgtype.UUID) (LineageStep, bool, error)
	ReadOldest        func(context.Context, pgtype.UUID) (OldestVersion, bool, error)

	ReadVersionSummaries func(context.Context, pgtype.UUID, []pgtype.UUID) (map[pgtype.UUID]VersionSummary, error)
	ReadDisplayNames     func(context.Context, []pgtype.UUID) (map[pgtype.UUID]string, error)
}

type VersionSummary struct {
	SkillID             pgtype.UUID
	VersionNumber       int32
	LatestVersionNumber int32
	AccessRestriction   *string
	Redistribution      string
}

type SkillFacts struct {
	ID                  pgtype.UUID
	Name                string
	ForkedFromSkillID   pgtype.UUID
	ForkedFromVersionID pgtype.UUID
	AccessRestriction   *string
	Redistribution      string
}

type VersionFacts struct {
	ID                pgtype.UUID
	SkillID           pgtype.UUID
	SourceID          pgtype.UUID
	VersionNumber     int32
	ContentHash       string
	PackageObjectKey  string
	LicenseExpression *string
	LicenseSource     *string
	CreatedAt         pgtype.Timestamptz
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

func (s *Service) requireOwnerReads() error {
	if s.TestLab == nil || s.AppliedSuggestions == nil || s.SourceLineage == nil || s.ReadSkill == nil ||
		s.ReadVersion == nil || s.ReadCompatibility == nil || s.ReadPrevious == nil ||
		s.ReadLineage == nil || s.ReadOldest == nil || s.CuratedSource == nil || s.ReadVersionSummaries == nil {
		return errOwnerReadNotConfigured
	}
	return nil
}

type AppliedSuggestion struct {
	EvaluationID pgtype.UUID
	Category     string
	TargetPath   string
}

type CuratedSource struct {
	SkillID     pgtype.UUID
	WorkspaceID pgtype.UUID
}

type LineageSource struct {
	SourceType  string
	SourceURL   *string
	SourceRef   *string
	ContentHash string
	FetchedAt   pgtype.Timestamptz
}

func checkProducedSize(n int) error {
	if n <= MaxProducedZipBytes {
		return nil
	}
	metrics.PackageSizeRefused.WithLabelValues(metrics.CeilingProduced).Inc()
	return fmt.Errorf("the produced package is %d bytes, over the %d byte limit", n, MaxProducedZipBytes)
}

const MaxProducedZipBytes = skillpkg.MaxZipBytes + 8<<20

type Plan struct {
	Skill   SkillFacts
	Version VersionFacts
	Profile Profile

	IncludeTestCases bool
	Allowed          bool
	BlockedReason    string
	BlockedMessage   string

	Retention time.Duration

	LatestVersionNumber int32

	Validation ManifestValidation
	Included   []IncludedTestCase
	Excluded   []ExcludedTestCase

	ExcludedFiles []ExcludedFile

	Dependencies []string

	Zip          []byte
	FileName     string
	ContentHash  string
	ManifestHash string
}

func referencedUnder(referenced map[string]bool, dirPath string) bool {
	for ref := range referenced {
		if strings.HasPrefix(ref, dirPath) {
			return true
		}
	}
	return false
}

func (s *Service) Plan(
	ctx context.Context, ws identity.Workspace, skillID, versionID pgtype.UUID,
	target string, includeTestCases bool,
) (*Plan, error) {
	if !isTargetID(target) {
		return nil, ErrUnknownTarget
	}
	profile, ok := s.Profiles[target]
	if !ok {
		return nil, ErrNoProfile
	}
	if s.Store == nil {
		return nil, ErrNoStore
	}

	retention, err := s.Retention.Period()
	if err != nil {
		return nil, err
	}
	if err := s.requireOwnerReads(); err != nil {
		return nil, err
	}
	q := gen.New(s.Pool)

	skill, found, err := s.ReadSkill(ctx, ws.ID, skillID)
	if !found && err == nil {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	version, found, err := s.ReadVersion(ctx, ws.ID, versionID)
	if !found && err == nil {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if version.SkillID != skill.ID {
		return nil, ErrNotFound
	}
	summaries, err := s.ReadVersionSummaries(ctx, ws.ID, []pgtype.UUID{version.ID})
	if err != nil {
		return nil, err
	}
	summary, found := summaries[version.ID]
	if !found {
		return nil, ErrNotFound
	}

	p := &Plan{
		Skill: skill, Version: version, Profile: profile,
		IncludeTestCases: includeTestCases, Retention: retention,
		LatestVersionNumber: summary.LatestVersionNumber,
		Validation:          ManifestValidation{Errors: []ManifestFinding{}, Warnings: []ManifestFinding{}, Infos: []ManifestFinding{}},
		Included:            []IncludedTestCase{}, Excluded: []ExcludedTestCase{},
		Dependencies: []string{},
	}
	if reason, msg := gate(skill); reason != "" {
		p.BlockedReason, p.BlockedMessage = reason, msg
		return p, nil
	}
	if err := s.build(ctx, q, ws, p); err != nil {
		return nil, err
	}
	return p, nil
}

func gate(skill SkillFacts) (reason, message string) {
	return gateFlags(skill.AccessRestriction, skill.Redistribution)
}

func gateFlags(accessRestriction *string, redistribution string) (reason, message string) {
	if accessRestriction != nil && *accessRestriction != "" {
		return BlockedLicenseHold,
			"這個 Skill 的內容因授權問題尚未釐清而被保留，所以無法從中產出套件"
	}
	switch redistribution {
	case RedistributionAllowed:
		return "", ""
	case RedistributionSelfSupplied:

		return "", ""
	case RedistributionGenerated:

		return "", ""
	case RedistributionBlocked:
		return BlockedNotRedistributable,
			"這個 Skill 的授權不允許再散布，所以 Skill Hub 不會提供副本。" +
				"人工確認過授權，不代表這份授權允許再散布"
	default:
		return BlockedLicenseUnknown,
			"沒有人確認過這個 Skill 可不可以再散布，未確認的授權視同不允許"
	}
}

func (s *Service) build(ctx context.Context, q *gen.Queries, ws identity.Workspace, p *Plan) error {
	data, err := s.Store.Get(ctx, p.Version.PackageObjectKey)
	if err != nil {
		return fmt.Errorf("stored package unreadable: %w", err)
	}
	fsys, err := skillpkg.PackageFS(data)
	if err != nil {
		return fmt.Errorf("stored package unreadable: %w", err)
	}

	source := skillpkg.Validate(fsys)
	sourceBlocked := false
	for _, finding := range source.Findings {
		if finding.Severity == skillpkg.SeverityError && !pathUsesExcludedDir(finding.Path) {
			sourceBlocked = true
			break
		}
	}
	if sourceBlocked {
		cat := source.Categorize()
		p.Validation = ManifestValidation{
			Blocked: true, Errors: toManifestFindings(cat.Errors),
			Warnings: toManifestFindings(cat.Warnings), Infos: toManifestFindings(cat.Infos),
		}
		p.BlockedReason = BlockedValidation
		p.BlockedMessage = "儲存的來源套件已經不符合匯入驗證"
		return nil
	}
	files, dropped, err := collect(fsys)
	if err != nil {
		return err
	}

	referenced := map[string]bool{}
	for _, ref := range skillpkg.SkillMDReferences(fsys) {
		referenced[ref] = true
	}
	p.ExcludedFiles = make([]ExcludedFile, 0, len(dropped))
	var broke []string
	for _, e := range dropped {
		e.ReferencedBySkillMD = referenced[e.Path] ||
			(strings.HasSuffix(e.Path, "/") && referencedUnder(referenced, e.Path))
		if e.ReferencedBySkillMD {
			broke = append(broke, e.Path)
		}
		p.ExcludedFiles = append(p.ExcludedFiles, e.withWords())
	}
	if len(broke) > 0 {
		p.BlockedReason = BlockedFileRemoved
		p.BlockedMessage = "SKILL.md 指向 " + strings.Join(broke, "、") +
			"，而打包器不會把它帶進套件——這一份下載回去會缺少它自己說明要用的東西。" +
			"把檔案移出被排除的目錄、或改用實體檔案取代連結之後再打包一次。"
		return nil
	}

	skillName := p.Skill.Name
	for i, f := range files {
		if f.path != "SKILL.md" {
			continue
		}

		patched, err := addFrontmatter(f.data, p.Profile.FrontmatterAdditions)
		if err != nil {
			return err
		}
		files[i].data = patched
	}

	included, excluded, caseFiles, err := s.selectTestCases(ctx, ws, p.Skill, p.IncludeTestCases)
	if err != nil {
		return err
	}
	p.Included, p.Excluded = included, excluded
	files = append(files, caseFiles...)

	report := validate(files)
	p.Dependencies = dependencyNotes(report)
	files = append(files, exportFile{
		path: InstallFile,
		data: []byte(renderInstall(p.Profile, skillName, p.Dependencies)),
	})

	files = dedupeByPath(files)

	manifest, err := s.buildManifest(ctx, ws, p, report, files)
	if err != nil {
		return err
	}

	files = dedupeByPath(append(files, manifest))

	zipped, err := writeZip(files, p.Profile.topLevelDir(skillName))
	if err != nil {
		return err
	}
	if err := checkProducedSize(len(zipped)); err != nil {
		return err
	}

	produced, err := skillpkg.PackageFS(zipped)
	if err != nil {
		return fmt.Errorf("the produced package could not be re-opened: %w", err)
	}
	final := skillpkg.Validate(produced)
	if final.Blocked {
		p.Validation = carriedValidation(final)
		p.BlockedReason = BlockedValidation
		p.BlockedMessage = "這些設定會產出無法通過匯入驗證的套件，因此不能當成有效套件提供"
		return nil
	}

	p.Validation = carriedValidation(report)

	p.Allowed = true
	p.Zip = zipped
	p.ContentHash = sha256Hex(zipped)
	p.FileName = fmt.Sprintf("%s-v%d-%s.zip", skillName, p.Version.VersionNumber, p.Profile.ID)
	return nil
}

func validate(files []exportFile) skillpkg.Report {
	return skillpkg.Validate(exportFS(files))
}

var dependencyCodes = map[string]bool{
	"dependency-file": true, "package-dependencies": true, "undeclared-dependency": true,
}

func dependencyNotes(r skillpkg.Report) []string {
	var out []string
	for _, f := range r.Findings {
		if dependencyCodes[f.Code] {
			line := f.Message
			if f.Path != "" {
				line = f.Path + ": " + f.Message
			}
			out = append(out, line)
			out = append(out, f.Details...)
		}
	}
	return out
}

func dedupeByPath(files []exportFile) []exportFile {
	seen := map[string]int{}
	out := make([]exportFile, 0, len(files))
	for _, f := range files {
		if i, dup := seen[f.path]; dup {
			out[i] = f
			continue
		}
		seen[f.path] = len(out)
		out = append(out, f)
	}
	return out
}

func carriedValidation(r skillpkg.Report) ManifestValidation {
	cat := r.Categorize()
	return ManifestValidation{
		Blocked:  r.Blocked,
		Errors:   toManifestFindings(cat.Errors),
		Warnings: toManifestFindings(cat.Warnings),
		Infos:    toManifestFindings(cat.Infos),
	}
}

func (s *Service) buildManifest(
	ctx context.Context, ws identity.Workspace, p *Plan,
	report skillpkg.Report, files []exportFile,
) (exportFile, error) {
	origin, err := s.originOf(ctx, ws, p.Skill, p.Version)
	if err != nil {
		return exportFile{}, err
	}
	compat, err := s.compatibilityOf(ctx, p.Version.ID)
	if err != nil {
		return exportFile{}, err
	}
	hash, err := manifestHash(files)
	if err != nil {
		return exportFile{}, err
	}
	cat := report.Categorize()
	if !p.Version.CreatedAt.Valid {
		return exportFile{}, errors.New("skill version has no creation timestamp")
	}

	m := Manifest{
		SchemaVersion:          ManifestSchemaVersion,
		PackagedAt:             p.Version.CreatedAt.Time.UTC().Format(time.RFC3339),
		SourceVersionCreatedAt: p.Version.CreatedAt.Time.UTC().Format(time.RFC3339),
		PackagerVersion:        PackagerVersion,
		ProfileID:              p.Profile.ID,
		ProfileVersion:         p.Profile.Version,
		Source: ManifestSource{
			SkillID:        pgconv.UUIDString(p.Skill.ID),
			SkillVersionID: pgconv.UUIDString(p.Version.ID),
			VersionNumber:  p.Version.VersionNumber,
			ContentHash:    p.Version.ContentHash,
			Origin:         origin,
		},
		License: ManifestLicense{

			Expression:  p.Version.LicenseExpression,
			SourceTier:  p.Version.LicenseSource,
			Disclosures: licenseDisclosures(cat.Infos),
		},
		Validation:        carriedValidation(report),
		Compatibility:     compat,
		IncludedTestCases: p.Included,
		ExcludedTestCases: p.Excluded,
		ExcludedFiles:     p.ExcludedFiles,
		ManifestHash:      hash,
	}
	p.ManifestHash = hash

	body, err := marshalManifest(m)
	if err != nil {
		return exportFile{}, err
	}
	return exportFile{path: ManifestFile, data: body}, nil
}

type Result struct {
	Artifact  Artifact
	Duplicate bool
	Plan      *Plan
}

type Artifact struct {
	ArtifactID        string `json:"artifact_id"`
	SkillID           string `json:"skill_id"`
	SkillVersionID    string `json:"skill_version_id"`
	Target            string `json:"target"`
	FileName          string `json:"file_name"`
	SizeBytes         int64  `json:"size_bytes"`
	ContentHash       string `json:"content_hash"`
	ManifestHash      string `json:"manifest_hash"`
	Status            string `json:"status"`
	ExpiresAt         string `json:"expires_at"`
	CreatedAt         string `json:"created_at"`
	DownloadCount     int64  `json:"download_count"`
	IncludesTestCases bool   `json:"includes_test_cases"`
	PackagerVersion   string `json:"packager_version,omitempty"`
	ProfileVersion    string `json:"profile_version,omitempty"`

	Servable   bool     `json:"servable"`
	ServeState labelled `json:"serve_state"`

	VersionNumber       int32    `json:"version_number"`
	LatestVersionNumber int32    `json:"latest_version_number"`
	VersionState        labelled `json:"version_state"`
}

type labelled struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Note  string `json:"note"`
}

func (a Artifact) withVersionState() Artifact {
	if a.LatestVersionNumber > a.VersionNumber {
		a.VersionState = labelled{"superseded",
			fmt.Sprintf("v%d(這個 Skill 已經到 v%d)", a.VersionNumber, a.LatestVersionNumber),
			fmt.Sprintf("這一份是 v%d 的內容,而且不會改變——版本是不可變的。"+
				"要拿 v%d 的內容,回到該 Skill 對 v%d 重新打包一次。",
				a.VersionNumber, a.LatestVersionNumber, a.LatestVersionNumber)}
		return a
	}
	a.VersionState = labelled{"current", fmt.Sprintf("v%d(最新)", a.VersionNumber), ""}
	return a
}

func (a Artifact) withServeState(expiresAt, purgedAt time.Time) Artifact {
	purged := !purgedAt.IsZero()

	lost := purged && !expiresAt.IsZero() && purgedAt.Before(expiresAt)
	switch {
	case a.Status == "quarantined":
		a.ServeState = labelled{"quarantined", "檢查中(尚未可下載)",
			"打包完成,驗證還沒結束。這是暫時狀態(ADR-003 隔離)。"}
	case a.Status == "rejected":
		a.ServeState = labelled{"rejected", "已拒絕(打包後未通過驗證)",
			"這一份不會被提供。要再拿到同樣的內容,回到該版本重新打包一次。"}

	case lost:
		a.ServeState = labelled{"lost", "檔案遺失,不再提供下載",
			"這不是保存期到期——檔案在保存期內就不見了,是平台這一側的問題。" +
				"同一版本重新打包一次可以拿回同樣的內容(打包是冪等的);" +
				"如果再次發生,請回報。"}
	case !expiresAt.IsZero() && !expiresAt.After(time.Now()):
		a.ServeState = labelled{"expired", "已過期,不再提供下載",
			"檔案已刪除,這筆紀錄保留。「已過期」與「沒有這一筆」不是同一件事。" +
				"同一版本隨時可以再打包一次。"}
	case purged:
		a.ServeState = labelled{"purged", "檔案已不存在,紀錄保留",
			"儲存的位元組已經不在了,而這一列還在。同一版本可以再打包一次。"}
	default:
		a.Servable = true
		a.ServeState = labelled{"available", "可下載", ""}
	}
	return a
}

func (s *Service) Create(
	ctx context.Context, ws identity.Workspace, skillID, versionID pgtype.UUID,
	target string, includeTestCases bool,
) (Result, error) {

	retention, err := s.Retention.Period()
	if err != nil {
		return Result{}, err
	}
	if err := s.requireOwnerReads(); err != nil {
		return Result{}, err
	}

	skill, found, err := s.ReadSkill(ctx, ws.ID, skillID)
	if !found && err == nil {
		return Result{}, ErrNotFound
	}
	if err != nil {
		return Result{}, err
	}
	if reason, msg := gate(skill); reason != "" {
		return Result{Plan: &Plan{
			Skill: skill, BlockedReason: reason, BlockedMessage: msg,
			Validation: ManifestValidation{Errors: []ManifestFinding{}, Warnings: []ManifestFinding{}, Infos: []ManifestFinding{}},
			Included:   []IncludedTestCase{}, Excluded: []ExcludedTestCase{},
			Dependencies: []string{},
		}}, nil
	}

	if !isTargetID(target) {
		return Result{}, ErrUnknownTarget
	}
	p, err := s.Plan(ctx, ws, skillID, versionID, target, includeTestCases)
	if err != nil {
		return Result{}, err
	}
	if !p.Allowed {

		return Result{Plan: p}, nil
	}
	return s.persist(ctx, ws, p, retention)
}

func (s *Service) persist(
	ctx context.Context, ws identity.Workspace, p *Plan, retention time.Duration,
) (Result, error) {
	objectKey := "downloads/" + pgconv.UUIDString(ws.ID) + "/" + p.ContentHash + ".zip"
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return Result{}, err
	}
	workspaceLocked, objectLocked := false, false
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), objectCleanupTimeout)
		defer cancel()
		// A session lock that fails to release must not go back to the pool
		// still held; hijacking and closing the connection instead forces the
		// pool to open a fresh one.
		q := gen.New(conn)
		if objectLocked {
			if _, err := q.UnlockDownloadObjectKeySession(unlockCtx, downloadObjectLockKey(objectKey)); err != nil {
				slog.Error("download object lock could not be released; closing connection", "error", err)
				_ = conn.Hijack().Close(context.Background())
				return
			}
		}
		if workspaceLocked {
			if _, err := q.UnlockPackagingWorkspaceObjectsSession(unlockCtx, ws.ID); err != nil {
				slog.Error("packaging workspace lock could not be released; closing connection", "error", err)
				_ = conn.Hijack().Close(context.Background())
				return
			}
		}
		conn.Release()
	}()
	q := gen.New(conn)
	if err := q.LockPackagingWorkspaceObjectsSession(ctx, ws.ID); err != nil {
		return Result{}, err
	}
	workspaceLocked = true
	if s.MayStoreObjects == nil {
		return Result{}, errors.New("packaging: identity lifecycle read is not configured")
	}
	allowed, err := s.MayStoreObjects(ctx, conn, ws.ID)
	if err != nil {
		return Result{}, err
	}
	if !allowed {
		return Result{}, ErrNotFound
	}
	lockKey := downloadObjectLockKey(objectKey)
	if err := q.LockDownloadObjectKeySession(ctx, lockKey); err != nil {
		return Result{}, err
	}
	objectLocked = true
	if existing, err := q.FindReusableDownloadArtifact(ctx, gen.FindReusableDownloadArtifactParams{
		WorkspaceID: ws.ID, SkillVersionID: p.Version.ID, Target: p.Profile.ID,
		PackagerVersion: PackagerVersion, IncludesTestCases: p.IncludeTestCases,
		ContentHash: p.ContentHash,
	}); err == nil {
		return Result{Artifact: reusedArtifact(p, existing), Duplicate: true}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Result{}, err
	}
	exists, err := s.Store.Exists(ctx, objectKey)
	if err != nil {
		return Result{}, err
	}
	ownsObject, commitAttempted := false, false
	intentCreated := false

	// Removes the object only if this call put it there and never committed;
	// an object another artifact already shares is left alone.
	defer func() {
		if ownsObject && !commitAttempted {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), objectCleanupTimeout)
			defer cancel()
			if err := s.Store.Remove(cleanupCtx, objectKey); err != nil {
				slog.Error("failed to compensate download package object", "key", objectKey, "error", err)
				return
			}
			if intentCreated {
				if err := gen.New(conn).DeleteDownloadCleanupIntent(cleanupCtx, gen.DeleteDownloadCleanupIntentParams{
					ObjectKey: objectKey, WorkspaceID: ws.ID,
				}); err != nil {
					slog.Error("failed to clear compensated download cleanup intent", "key", objectKey, "error", err)
				}
			}
		}
	}()
	if _, err := q.CreateDownloadCleanupIntent(ctx, gen.CreateDownloadCleanupIntentParams{
		WorkspaceID: ws.ID, ObjectKey: objectKey,
	}); err != nil {
		return Result{}, err
	}
	intentCreated = true
	ownsObject = !exists

	if err := s.Store.Put(ctx, objectKey, p.Zip); err != nil {
		return Result{}, err
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q = gen.New(tx)

	row, err := q.CreateDownloadArtifactRow(ctx, gen.CreateDownloadArtifactRowParams{
		WorkspaceID: ws.ID,
		FileName:    p.FileName,
		ContentType: "application/zip",
		SizeBytes:   int64(len(p.Zip)),
		ContentHash: p.ContentHash,
		ObjectKey:   objectKey,
		ExpiresAt:   pgtype.Timestamptz{Time: time.Now().Add(retention), Valid: true},
	})
	if err != nil {
		return Result{}, err
	}
	if err := q.CreateDownloadArtifactDetail(ctx, gen.CreateDownloadArtifactDetailParams{
		ArtifactID: row.ID, WorkspaceID: ws.ID, SkillVersionID: p.Version.ID,
		Target: p.Profile.ID, ProfileVersion: p.Profile.Version,
		PackagerVersion: PackagerVersion, ManifestHash: p.ManifestHash,
		IncludesTestCases: p.IncludeTestCases,
	}); err != nil {
		return Result{}, err
	}

	if err := q.MarkDownloadArtifactAvailable(ctx, gen.MarkDownloadArtifactAvailableParams{
		ID: row.ID, WorkspaceID: ws.ID,
	}); err != nil {
		return Result{}, err
	}
	if intentCreated {
		if err := q.DeleteDownloadCleanupIntent(ctx, gen.DeleteDownloadCleanupIntentParams{
			ObjectKey: objectKey, WorkspaceID: ws.ID,
		}); err != nil {
			return Result{}, err
		}
	}
	commitAttempted = true
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}

	return Result{Plan: p, Artifact: Artifact{
		ArtifactID:          pgconv.UUIDString(row.ID),
		SkillID:             pgconv.UUIDString(p.Skill.ID),
		SkillVersionID:      pgconv.UUIDString(p.Version.ID),
		Target:              p.Profile.ID,
		FileName:            p.FileName,
		SizeBytes:           int64(len(p.Zip)),
		ContentHash:         p.ContentHash,
		ManifestHash:        p.ManifestHash,
		Status:              "available",
		ExpiresAt:           rfc3339(row.ExpiresAt),
		CreatedAt:           rfc3339(row.CreatedAt),
		DownloadCount:       0,
		IncludesTestCases:   p.IncludeTestCases,
		PackagerVersion:     PackagerVersion,
		ProfileVersion:      p.Profile.Version,
		VersionNumber:       p.Version.VersionNumber,
		LatestVersionNumber: p.LatestVersionNumber,
	}.withVersionState().withServeState(row.ExpiresAt.Time, time.Time{})}, nil
}

func reusedArtifact(p *Plan, row gen.FindReusableDownloadArtifactRow) Artifact {
	return Artifact{
		ArtifactID:          pgconv.UUIDString(row.ArtifactID),
		SkillID:             pgconv.UUIDString(p.Skill.ID),
		SkillVersionID:      pgconv.UUIDString(row.SkillVersionID),
		Target:              row.Target,
		FileName:            row.FileName,
		SizeBytes:           row.SizeBytes,
		ContentHash:         row.ContentHash,
		ManifestHash:        row.ManifestHash,
		Status:              row.ScanStatus,
		ExpiresAt:           rfc3339(row.ExpiresAt),
		CreatedAt:           rfc3339(row.CreatedAt),
		DownloadCount:       row.DownloadCount,
		IncludesTestCases:   row.IncludesTestCases,
		PackagerVersion:     row.PackagerVersion,
		ProfileVersion:      row.ProfileVersion,
		VersionNumber:       p.Version.VersionNumber,
		LatestVersionNumber: p.LatestVersionNumber,
	}.withVersionState().withServeState(row.ExpiresAt.Time, time.Time{})
}
