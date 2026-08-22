package packaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

// PackagerVersion identifies this builder. content_hash reproduces within one
// packager version and is deliberately not promised across them (ADR-027
// decision 2), which is why the version is recorded in every manifest, in every
// download_artifacts row, and in the idempotency key.
//
// Bump it whenever the produced bytes could change for unchanged input: the
// allow-list, the zip writing, the manifest shape or the INSTALL.md template.
// 0.2.0: INSTALL.md's dependency section gained the `undeclared-dependency`
// findings (04 丙-18), which changes the produced bytes for unchanged input.
const PackagerVersion = "0.2.0"

const objectCleanupTimeout = 5 * time.Second

// Artifact creation remains disabled until a deployment explicitly configures
// retention; PDM-006's proposed duration is not a production default.
// It is intentionally not a schema or code default (m4/README §8.1), because
// that would turn a proposal into a production fact.
// The four reasons a package may not be built (public.yaml
// PackagingBlockedReason). One vocabulary from one place: the preview and the
// create call answer from these and nothing else, so a preview that says yes and
// a create that refuses cannot both happen — the same arrangement EVAL-002's
// check() has with SuggestionBlockedReason.
const (
	BlockedLicenseHold        = "license_hold"
	BlockedNotRedistributable = "not_redistributable"
	BlockedLicenseUnknown     = "license_unknown"
	BlockedValidation         = "validation_blocked"
)

// Redistribution values (0027 / ADR-027 decision 4). Only `allowed` releases.
const (
	RedistributionAllowed = "allowed"
	RedistributionBlocked = "blocked"
)

var (
	// ErrNotFound: the skill or version is not visible in the caller's
	// workspace, or the version is not this skill's. One answer for all of them —
	// existence is private (WS-006).
	ErrNotFound = errors.New("skill version not found")
	// ErrUnknownTarget: a target outside the contract's enum. A client mistake
	// (400), not a property of the content.
	ErrUnknownTarget = errors.New("unknown packaging target")
	// ErrNoProfile: the target is in the enum but this deployment has no
	// configuration for it. A deployment fault (503), never a silent fallback to
	// a compiled-in default — see profile.go.
	ErrNoProfile = errors.New("this deployment has no configuration for that packaging target")
	// ErrNoStore: no object store wired in. Nothing was checked, so it is a 500
	// and never a blocked_reason.
	ErrNoStore = errors.New("no object store is configured, so package bytes cannot be read")
	// ErrRetentionNotConfigured keeps artifact creation fail-closed while the
	// retention period remains an unratified deployment decision (PDM-006). The
	// rule and the sentinel are Policy & Usage's (ADR-032 §1, DDD-014); this alias
	// keeps it in the list of reasons a packaging call can fail, where the HTTP
	// layer looks for them.
	ErrRetentionNotConfigured = policy.ErrRetentionNotConfigured
	errOwnerReadNotConfigured = errors.New("packaging: owner reads not injected")
)

// ObjectStore is the slice of object storage packaging needs: the source
// package and any curated dataset come out, the built package goes in, and a
// package the owner deleted goes away.
type ObjectStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key string, data []byte) error
	Remove(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

type Service struct {
	Pool  *pgxpool.Pool
	Store ObjectStore
	// TestLab is the owner face for portable Test Case inputs.
	TestLab *testlab.Service
	// Profiles is the deployment's packaging target configuration. Empty is a
	// legitimate state and it means "no targets"; it never means "use defaults".
	Profiles Profiles
	// Retention must be explicitly configured by a deployment. Zero disables
	// artifact creation until PDM-006 is ratified — the rule is
	// policy.DownloadRetention.Period, not a check written here.
	Retention policy.DownloadRetention
	// AppliedSuggestions and SourceLineage are owner reads adapted by the
	// composition root; packaging must not read eval or ingest tables directly.
	AppliedSuggestions func(ctx context.Context, versionID, workspaceID pgtype.UUID) ([]AppliedSuggestion, error)
	SourceLineage      func(ctx context.Context, sourceID pgtype.UUID) (LineageSource, error)
	ReadSkill          func(context.Context, pgtype.UUID, pgtype.UUID) (SkillFacts, bool, error)
	ReadVersion        func(context.Context, pgtype.UUID, pgtype.UUID) (VersionFacts, bool, error)
	ReadCompatibility  func(context.Context, pgtype.UUID) (RuntimeCompatibility, bool, error)
	ReadPrevious       func(context.Context, pgtype.UUID, pgtype.UUID, int32) (PreviousVersion, bool, error)
	ReadLineage        func(context.Context, pgtype.UUID) (LineageStep, bool, error)
	ReadOldest         func(context.Context, pgtype.UUID) (OldestVersion, bool, error)
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
	if s.TestLab == nil || s.TestLab.Pool == nil || s.AppliedSuggestions == nil || s.SourceLineage == nil || s.ReadSkill == nil ||
		s.ReadVersion == nil || s.ReadCompatibility == nil || s.ReadPrevious == nil ||
		s.ReadLineage == nil || s.ReadOldest == nil {
		return errOwnerReadNotConfigured
	}
	return nil
}

type AppliedSuggestion struct {
	EvaluationID pgtype.UUID
	Category     string
	TargetPath   string
}

type LineageSource struct {
	SourceType  string
	SourceURL   *string
	SourceRef   *string
	ContentHash string
	FetchedAt   pgtype.Timestamptz
}

// Plan is one answered packaging question, shared by the preview and the create
// call. When Allowed is false, BlockedReason says which of the four gates closed
// and nothing was built.
type Plan struct {
	Skill   SkillFacts
	Version VersionFacts
	Profile Profile

	IncludeTestCases bool
	Allowed          bool
	BlockedReason    string
	BlockedMessage   string

	Validation ManifestValidation
	Included   []IncludedTestCase
	Excluded   []ExcludedTestCase
	// Dependencies is the same list INSTALL.md renders, served on the preview so
	// 02:PACK-002 第 1 條's "依賴" is answerable before a package exists. Empty when
	// a licensing gate closed before anything was read — there is no package to
	// have dependencies.
	Dependencies []string

	// The built bytes and what identifies them. Empty when a gate closed before
	// the build, which is every gate except validation.
	Zip          []byte
	FileName     string
	ContentHash  string
	ManifestHash string
}

// Plan answers "may this version be packaged for this target, and what would
// come out" without writing anything.
//
// The order of the gates is the order of their cost and their authority: the two
// licensing verdicts are facts about the content and are settled from two
// columns, so they are answered before a single byte is read. Validation is last
// because it is a statement about bytes that do not exist until they are built.
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

	p := &Plan{
		Skill: skill, Version: version, Profile: profile,
		IncludeTestCases: includeTestCases,
		Validation:       ManifestValidation{Errors: []ManifestFinding{}, Warnings: []ManifestFinding{}, Infos: []ManifestFinding{}},
		Included:         []IncludedTestCase{}, Excluded: []ExcludedTestCase{},
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

// gate is the licensing half of the four locks: two flags, opposite in nature,
// neither able to stand in for the other (ADR-027 decision 4).
//
// access_restriction is a hold somebody pressed by hand and covers the cases
// known today. redistribution is a property of the content and every skill needs
// an answer to it, including the one a user uploads tomorrow. Treating the hold
// as the redistribution test would mean "nobody objected, so it may be copied" —
// the direction ADR-021 §5.3 records a real misjudgement in.
//
// Both fail closed. An unrecognised restriction code still restricts, because a
// code nobody recognises must never be the way content unlocks; and any
// redistribution value that is not exactly `allowed` blocks, so a value added to
// the column tomorrow does not release content today.
func gate(skill SkillFacts) (reason, message string) {
	return gateFlags(skill.AccessRestriction, skill.Redistribution)
}

// gateFlags is gate over the two columns alone, so the download endpoint can ask
// the same question from its own joined row without loading the skill again.
// One function and not two readings of two flags: a second copy is how the
// download path ends up serving what the packaging path would refuse.
func gateFlags(accessRestriction *string, redistribution string) (reason, message string) {
	if accessRestriction != nil && *accessRestriction != "" {
		return BlockedLicenseHold,
			"this skill's materials are held back while a licensing question about them is open, " +
				"so no package can be produced from them"
	}
	switch redistribution {
	case RedistributionAllowed:
		return "", ""
	case RedistributionBlocked:
		return BlockedNotRedistributable,
			"this skill's licence does not permit redistribution, so Skill Hub does not hand out " +
				"copies of it. A manually confirmed licence is not the same statement as a " +
				"redistributable one"
	default:
		return BlockedLicenseUnknown,
			"nobody has established whether this skill may be redistributed, and an unestablished " +
				"licence is treated as one that does not permit it"
	}
}

// build produces the candidate bytes and re-validates them.
//
// It validates what it PRODUCED, not what it read. The source version passed
// validation at import, but packaging adds files and a profile may add
// frontmatter fields, so validating the source would make PACK-002 a check of
// something nobody downloads. The severities are skillpkg's own — the packager
// does not get to define a second notion of "blocking", because two definitions
// drift and the packaging side always drifts looser (it sits at the step the
// user most wants to succeed).
func (s *Service) build(ctx context.Context, q *gen.Queries, ws identity.Workspace, p *Plan) error {
	data, err := s.Store.Get(ctx, p.Version.PackageObjectKey)
	if err != nil {
		return fmt.Errorf("stored package unreadable: %w", err)
	}
	fsys, err := skillpkg.PackageFS(data)
	if err != nil {
		return fmt.Errorf("stored package unreadable: %w", err)
	}
	// Re-check the immutable source before applying the export allow-list. Import
	// normally guarantees this already, but a legacy/corrupt row must not turn a
	// newly blocking secret finding into a successful package merely because the
	// credential-shaped file would be omitted from the output.
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
		p.BlockedMessage = "the stored source package no longer passes import validation"
		return nil
	}
	files, err := collect(fsys)
	if err != nil {
		return err
	}

	skillName := p.Skill.Name
	for i, f := range files {
		if f.path != "SKILL.md" {
			continue
		}
		// The standard package has no additions by construction (profile.check),
		// so its SKILL.md comes through this loop byte for byte — which is the
		// evidence PDM-008 asks the standard package to be.
		patched, err := addFrontmatter(f.data, p.Profile.FrontmatterAdditions)
		if err != nil {
			return err
		}
		files[i].data = patched
	}

	included, excluded, caseFiles, err := s.selectTestCases(ctx, ws, p.Skill.ID, p.IncludeTestCases)
	if err != nil {
		return err
	}
	p.Included, p.Excluded = included, excluded
	files = append(files, caseFiles...)

	// A first pass over the produced content, so INSTALL.md can state the
	// dependencies the package declares and the manifest can carry the findings.
	report := validate(files)
	p.Dependencies = dependencyNotes(report)
	files = append(files, exportFile{
		path: InstallFile,
		data: []byte(renderInstall(p.Profile, skillName, p.Dependencies)),
	})

	// The platform's files are written after the source's, so a package that
	// happens to carry its own INSTALL.md does not decide what the platform says
	// about installing it. Everything else of the author's is untouched.
	files = dedupeByPath(files)

	manifest, err := s.buildManifest(ctx, ws, p, report, files)
	if err != nil {
		return err
	}
	files = append(files, manifest)

	zipped, err := writeZip(files, p.Profile.topLevelDir(skillName))
	if err != nil {
		return err
	}
	if len(zipped) > skillpkg.MaxZipBytes {
		return fmt.Errorf("the produced package is %d bytes, over the %d byte limit",
			len(zipped), skillpkg.MaxZipBytes)
	}

	// The invariant of PACK-009, asserted on the bytes that would be handed out:
	// re-opening them the way import does must produce a package import would
	// accept.
	produced, err := skillpkg.PackageFS(zipped)
	if err != nil {
		return fmt.Errorf("the produced package could not be re-opened: %w", err)
	}
	final := skillpkg.Validate(produced)
	cat := final.Categorize()
	p.Validation = ManifestValidation{
		Blocked:  final.Blocked,
		Errors:   toManifestFindings(cat.Errors),
		Warnings: toManifestFindings(cat.Warnings),
		Infos:    toManifestFindings(cat.Infos),
	}
	if final.Blocked {
		p.BlockedReason = BlockedValidation
		p.BlockedMessage = "the package these settings would produce does not pass the validation " +
			"an import has to pass, so it must not be presented as a valid package"
		return nil
	}

	p.Allowed = true
	p.Zip = zipped
	p.ContentHash = sha256Hex(zipped)
	p.FileName = fmt.Sprintf("%s-v%d-%s.zip", skillName, p.Version.VersionNumber, p.Profile.ID)
	return nil
}

// validate runs the static validation over the files as they will be laid out
// inside the package root. Used twice: once before INSTALL.md and the manifest
// exist, to source their content, and once over the finished zip, which is the
// verdict that counts.
func validate(files []exportFile) skillpkg.Report {
	return skillpkg.Validate(exportFS(files))
}

// dependencyCodes are the findings INSTALL.md's dependency section is assembled
// from. `undeclared-dependency` is in the list because it is the first reason a
// reader follows the instructions and the Skill still does not run: the package
// declares nothing, a script imports something, and until now that only appeared
// in the manifest's warnings and on the preview page — never in the document the
// person installing it actually reads (04 丙-18).
var dependencyCodes = map[string]bool{
	"dependency-file": true, "package-dependencies": true, "undeclared-dependency": true,
}

// dependencyNotes turns the dependency findings into the INSTALL.md lines
// 02:PACK-002 asks for. The findings' own text, not a paraphrase.
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

// buildManifest assembles skillhub-manifest.json. It is the last file added,
// because manifest_hash covers every other file and not itself.
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
		SchemaVersion:   ManifestSchemaVersion,
		PackagedAt:      p.Version.CreatedAt.Time.UTC().Format(time.RFC3339),
		PackagerVersion: PackagerVersion,
		ProfileID:       p.Profile.ID,
		ProfileVersion:  p.Profile.Version,
		Source: ManifestSource{
			SkillID:        pgconv.UUIDString(p.Skill.ID),
			SkillVersionID: pgconv.UUIDString(p.Version.ID),
			VersionNumber:  p.Version.VersionNumber,
			ContentHash:    p.Version.ContentHash,
			Origin:         origin,
		},
		License: ManifestLicense{
			// Recorded as a PAIR and never flattened (ADR-021 decision 1): the
			// expression alone cannot say whether the author declared it or a
			// repository root file was read for it. Both null or both set.
			Expression:  p.Version.LicenseExpression,
			SourceTier:  p.Version.LicenseSource,
			Disclosures: licenseDisclosures(cat.Infos),
		},
		Validation: ManifestValidation{
			Blocked:  false,
			Errors:   []ManifestFinding{},
			Warnings: toManifestFindings(cat.Warnings),
			Infos:    toManifestFindings(cat.Infos),
		},
		Compatibility:     compat,
		IncludedTestCases: p.Included,
		ExcludedTestCases: p.Excluded,
		ManifestHash:      hash,
	}
	p.ManifestHash = hash

	body, err := marshalManifest(m)
	if err != nil {
		return exportFile{}, err
	}
	return exportFile{path: ManifestFile, data: body}, nil
}

// Result is one answered POST .../packaging.
type Result struct {
	Artifact  Artifact
	Duplicate bool
	Plan      *Plan
}

// Artifact is public.yaml's DownloadArtifact.
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

	// Servable is whether the content endpoint would hand the bytes over right
	// now, and ServeState is the one sentence for it (04 丙-29 ⑤).
	//
	// Served rather than left to the client because **one of the three inputs is
	// not on this shape at all**: the purge. A client could see `status` and
	// `expires_at` and never `purged_at`, so every client that derived a word was
	// deriving a different predicate from the one download.go enforces — the
	// 顯示但不強制 shape 設計系統 §2.2 names as the worst of the options.
	Servable   bool     `json:"servable"`
	ServeState labelled `json:"serve_state"`
}

// labelled is the contract's Labelled.
type labelled struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Note  string `json:"note"`
}

// withServeState fills the pair from the three facts that decide it, and is the
// only place they are combined. Every Artifact leaves this package through it.
//
// Order is deliberate. `quarantined` and `rejected` already say the bytes are not
// on offer and overwriting them would lose *why* — one is over, one is not.
// Expiry outranks purge because expiry is the reason and the purge is its
// consequence; a purge with no expiry is the reconciler having found the object
// missing, which is a different sentence and gets one.
func (a Artifact) withServeState(expiresAt time.Time, purged bool) Artifact {
	switch {
	case a.Status == "quarantined":
		a.ServeState = labelled{"quarantined", "檢查中(尚未可下載)",
			"打包完成,驗證還沒結束。這是暫時狀態(ADR-003 隔離)。"}
	case a.Status == "rejected":
		a.ServeState = labelled{"rejected", "已拒絕(打包後未通過驗證)",
			"這一份不會被提供。要再拿到同樣的內容,回到該版本重新打包一次。"}
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

// Create builds and stores one Download Artifact.
//
// Idempotent: the same (version, target, packager version, test-case choice)
// that already has an unexpired, available artifact returns that one with
// duplicate set, rather than spending the bytes again. Reuse includes the full
// content hash because compatibility measurements and portable Test Cases may
// change after an immutable Skill Version is created.
//
// The gates are re-checked before the lookup, not after: a hold applied since the
// last packaging run has to stop the copy that already exists from being handed
// out again.
func (s *Service) Create(
	ctx context.Context, ws identity.Workspace, skillID, versionID pgtype.UUID,
	target string, includeTestCases bool,
) (Result, error) {
	// Asked before anything is read or built: without a ratified period there is no
	// expires_at to write, and PDM-006's proposal is not a default (GOV-RETENTION-001).
	retention, err := s.Retention.Period()
	if err != nil {
		return Result{}, err
	}
	if err := s.requireOwnerReads(); err != nil {
		return Result{}, err
	}
	// Cheap enough to run first and it settles both licensing gates, so a held
	// skill never reaches the dedupe lookup or the object store.
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
		// Nothing was written, so there is nothing to reject. The reason travels
		// in the response and the preview reproduces it on demand.
		// ponytail: 0004's `rejected` scan state stays unused — recording one
		// would need a status_reason column artifacts does not have. Add both if
		// "why did this fail" has to outlive the request.
		return Result{Plan: p}, nil
	}
	return s.persist(ctx, ws, p, retention)
}

func (s *Service) persist(
	ctx context.Context, ws identity.Workspace, p *Plan, retention time.Duration,
) (Result, error) {
	objectKey := "downloads/" + pgconv.UUIDString(ws.ID) + "/" + p.ContentHash + ".zip"
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)
	lockKey := fmt.Sprintf("download-package:%s/%s", pgconv.UUIDString(ws.ID), p.ContentHash)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", lockKey); err != nil {
		return Result{}, err
	}
	if existing, err := q.FindReusableDownloadArtifact(ctx, gen.FindReusableDownloadArtifactParams{
		WorkspaceID: ws.ID, SkillVersionID: p.Version.ID, Target: p.Profile.ID,
		PackagerVersion: PackagerVersion, IncludesTestCases: p.IncludeTestCases,
		ContentHash: p.ContentHash,
	}); err == nil {
		return Result{Artifact: reusedArtifact(p.Skill.ID, existing), Duplicate: true}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Result{}, err
	}
	exists, err := s.Store.Exists(ctx, objectKey)
	if err != nil {
		return Result{}, err
	}
	ownsObject, commitAttempted := !exists, false
	// Workspace/content addressing plus the advisory lock make this compensation
	// private to this writer. An ambiguous Commit result is deliberately retained:
	// the row may actually have committed and deleting its object would be worse.
	defer func() {
		if ownsObject && !commitAttempted {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), objectCleanupTimeout)
			defer cancel()
			if err := s.Store.Remove(cleanupCtx, objectKey); err != nil {
				slog.Error("failed to compensate download package object", "key", objectKey, "error", err)
			}
		}
	}()
	if !exists {
		if err := s.Store.Put(ctx, objectKey, p.Zip); err != nil {
			return Result{}, err
		}
	}

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
	if _, err := q.CreateDownloadArtifactDetail(ctx, gen.CreateDownloadArtifactDetailParams{
		ArtifactID: row.ID, WorkspaceID: ws.ID, SkillVersionID: p.Version.ID,
		Target: p.Profile.ID, ProfileVersion: p.Profile.Version,
		PackagerVersion: PackagerVersion, ManifestHash: p.ManifestHash,
		IncludesTestCases: p.IncludeTestCases,
	}); err != nil {
		return Result{}, err
	}
	// The quarantine release of ADR-003, in the same transaction as the rows.
	// The check that lets it out is the re-validation build() already ran: this
	// package's content is a package that passed import validation plus files the
	// platform wrote itself, so there is no second scanner to invent. Keeping the
	// state is what lets "only `available` is served" be a condition a test can
	// hold the download handler to, rather than a habit.
	if err := q.MarkDownloadArtifactAvailable(ctx, gen.MarkDownloadArtifactAvailableParams{
		ID: row.ID, WorkspaceID: ws.ID,
	}); err != nil {
		return Result{}, err
	}
	commitAttempted = true
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}

	return Result{Plan: p, Artifact: Artifact{
		ArtifactID:        pgconv.UUIDString(row.ID),
		SkillID:           pgconv.UUIDString(p.Skill.ID),
		SkillVersionID:    pgconv.UUIDString(p.Version.ID),
		Target:            p.Profile.ID,
		FileName:          p.FileName,
		SizeBytes:         int64(len(p.Zip)),
		ContentHash:       p.ContentHash,
		ManifestHash:      p.ManifestHash,
		Status:            "available",
		ExpiresAt:         rfc3339(row.ExpiresAt),
		CreatedAt:         rfc3339(row.CreatedAt),
		DownloadCount:     0,
		IncludesTestCases: p.IncludeTestCases,
		PackagerVersion:   PackagerVersion,
		ProfileVersion:    p.Profile.Version,
	}.withServeState(row.ExpiresAt.Time, false)}, nil
}

func reusedArtifact(skillID pgtype.UUID, row gen.FindReusableDownloadArtifactRow) Artifact {
	return Artifact{
		ArtifactID:        pgconv.UUIDString(row.ArtifactID),
		SkillID:           pgconv.UUIDString(skillID),
		SkillVersionID:    pgconv.UUIDString(row.SkillVersionID),
		Target:            row.Target,
		FileName:          row.FileName,
		SizeBytes:         row.SizeBytes,
		ContentHash:       row.ContentHash,
		ManifestHash:      row.ManifestHash,
		Status:            row.ScanStatus,
		ExpiresAt:         rfc3339(row.ExpiresAt),
		CreatedAt:         rfc3339(row.CreatedAt),
		DownloadCount:     row.DownloadCount,
		IncludesTestCases: row.IncludesTestCases,
		PackagerVersion:   row.PackagerVersion,
		ProfileVersion:    row.ProfileVersion,
		// FindReusableDownloadArtifact filters `purged_at IS NULL`, so a reused row
		// is by construction not purged — the query is where that is enforced.
	}.withServeState(row.ExpiresAt.Time, false)
}
