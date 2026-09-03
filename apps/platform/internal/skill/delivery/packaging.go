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
	// BlockedFileRemoved is the one refusal on this list the platform caused
	// itself: SKILL.md points at a file that was in the version and that the
	// exporter removed. Everything else here is a decision about the content
	// that the user cannot argue with; this one they can fix in a minute.
	BlockedFileRemoved = "file_removed_by_packager"
)

// Redistribution values (0027 / ADR-027 decision 4, extended by 0036 and 0037).
//
// Three of the five release, and they release for three different reasons:
// `allowed` is a verdict about the licence, `self_supplied` is a fact about who
// brought the bytes, `generated` is a fact about who wrote them. Keeping them
// apart is the point — a publish-to-catalogue path must be able to tell
// "somebody established this may be copied" from "the owner is getting their
// own file back" from "a model wrote this and nobody has said who owns it".
const (
	RedistributionAllowed      = "allowed"
	RedistributionBlocked      = "blocked"
	RedistributionSelfSupplied = "self_supplied"
	RedistributionGenerated    = "generated"
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
	TestLab         *testlab.Service
	MayStoreObjects func(context.Context, gen.DBTX, pgtype.UUID) (bool, error)
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

// checkProducedSize applies the produced ceiling.
//
// A function and not four lines inline because build() needs a transaction and a
// store, and 「a package between the import ceiling and the produced ceiling is
// ACCEPTED」 is the entire content of 03:PACK-012 — a claim that has to be
// reachable by a test that does not need a MaxZipBytes-sized fixture and a database.
//
// The refusal is counted apart from the two import ceilings (03:INGEST-016): a
// package too big here is our packager having added more than the produced
// ceiling allows, not a creator having sent something too big, and reading our
// defect in the same series as their behaviour is how a wrong ceiling gets
// blamed on users.
func checkProducedSize(n int) error {
	if n <= MaxProducedZipBytes {
		return nil
	}
	metrics.PackageSizeRefused.WithLabelValues(metrics.CeilingProduced).Inc()
	return fmt.Errorf("the produced package is %d bytes, over the %d byte limit", n, MaxProducedZipBytes)
}

// MaxProducedZipBytes caps the package the packager BUILDS. skillpkg.MaxZipBytes
// caps what a creator HANDS US, and until 2026-08-25 this check used that one
// (03:PACK-012).
//
// They cannot be the same number. Packaging always adds — INSTALL.md, the
// manifest, and any test-cases/ that travel — so produced > source by
// construction, and produced_cap == import_cap therefore leaves a dead zone at
// ANY value of the import cap: a package sitting just under it imports, runs,
// evaluates, and is then refused at the last step with every bit of the work
// already done. That is independent of what 05 R-13 decides MaxZipBytes should
// be, which is why this does not wait for it.
//
// Decoupling upward cannot refuse anything that succeeds today: every value
// above MaxZipBytes is a strict relaxation of the check it replaces.
//
// It does not weaken PACK-009 either. That invariant is 「產出的位元組要能被匯入的
// 方式重新打開」, asserted below by re-opening through skillpkg.PackageFS — which
// checks entry count, per-entry size, depth and UNCOMPRESSED total, and not
// compressed total. This constant is not an input to it.
//
// # Where the value comes from, and where it stops
//
// The upper bound "import cap + what packaging can add" does not close. Written
// out rather than rounded away, because a picked number that looks derived is
// worse than a picked number that says it was picked:
//
//   - The platform's own two files ARE bounded by the source. INSTALL.md is a
//     fixed template plus dependency notes; the manifest lists findings,
//     excluded files and external URLs — all drawn from an archive already under
//     MaxZipBytes and holding at most 2,000 paths. 1 MiB is generous for both.
//   - One test case's case.json IS bounded, by testlab's own caps: MaxNameBytes
//     200 + MaxPromptBytes 32 KiB + MaxCriteria 50 × MaxCriterionBytes 2,000 +
//     the rubric ≈ 150 KiB.
//   - THE NUMBER OF TEST CASES PER SKILL IS NOT BOUNDED, and that is what stops
//     the arithmetic. Nothing enforces a count — testlab's page() comment says
//     the list is "already bounded by the per-skill test case count" and no such
//     limit exists anywhere in the repo. PDM-005 §5.1 bounds one test case's
//     files (20 files, 25 MiB each, 100 MiB total); it never bounds how many
//     test cases a Skill may have, and §5.2 is Run resources, not this.
//   - And the additions are not always text. 「不可散布的資料集會被排除」 holds for a
//     creator's own workspace; for the CATALOG workspace selectTestCases ships
//     the dataset bytes themselves (see testcase.go — the ws.IsCatalog branch),
//     up to testlab.MaxTestCaseBytes per case.
//
// So: 1 MiB for the platform's two files plus roughly 48 test cases at their
// text ceiling. 48 is the part with nothing behind it — a count nothing
// enforces, put far past any plausible Skill and far under the 100 MB
// uncompressed ceiling PackageFS already puts on these same bytes. If this cap
// is ever actually reached, the number to revisit is not this one: it is the
// per-skill test case cap that does not exist.
const MaxProducedZipBytes = skillpkg.MaxZipBytes + 8<<20

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

	// Retention is how long the artifact this plan would produce is kept, read
	// from the same policy.DownloadRetention that Create writes into expires_at.
	// Always positive on a plan that was returned at all — Plan refuses when the
	// deployment has no ratified period, so there is no "unset" to represent here
	// (03:PACK-011).
	Retention time.Duration

	Validation ManifestValidation
	Included   []IncludedTestCase
	Excluded   []ExcludedTestCase
	// ExcludedFiles is what the exporter removed from the author's own tree,
	// served on the preview so the answer arrives before the download rather
	// than as a surprise inside it.
	ExcludedFiles []ExcludedFile
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

// referencedUnder reports whether SKILL.md points at anything inside a removed
// directory. Directories are recorded as one entry (a vendored dependency tree
// is tens of thousands of files), so the membership test has to be a prefix.
func referencedUnder(referenced map[string]bool, dirPath string) bool {
	for ref := range referenced {
		if strings.HasPrefix(ref, dirPath) {
			return true
		}
	}
	return false
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
	// Asked here and not only in Create (03:PACK-011). Two reasons, and the second
	// is the one that made this a bug rather than a tidy-up:
	//
	//  1. The preview is where the retention gets DISCLOSED, and a number shown on
	//     a screen has to be the number the server enforces (設計 §2.2 顯示與強制成
	//     對). With no ratified period there is no number, so there is nothing to
	//     disclose and the honest answer is the same 503 the create call gives.
	//  2. Without it the preview answered `allowed: true` on a deployment where
	//     POST .../packaging returns 503 — the exact "a preview that says yes and a
	//     create that refuses cannot both happen" this file's header forbids.
	//     ErrNoProfile and ErrNoStore were already checked here; retention was the
	//     one deployment fault that was not.
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

	p := &Plan{
		Skill: skill, Version: version, Profile: profile,
		IncludeTestCases: includeTestCases, Retention: retention,
		Validation: ManifestValidation{Errors: []ManifestFinding{}, Warnings: []ManifestFinding{}, Infos: []ManifestFinding{}},
		Included:   []IncludedTestCase{}, Excluded: []ExcludedTestCase{},
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
	case RedistributionSelfSupplied:
		// Not a licence verdict, and it does not need to be. This workspace
		// supplied the package; handing it back is retrieval, and the second
		// party a licence protects does not exist in that transaction (0036,
		// ADR-045). The workspace scope every read on this path already carries
		// is what keeps it that way — nothing here widens it.
		return "", ""
	case RedistributionGenerated:
		// The platform wrote these bytes for this workspace, at its request. No
		// upstream author exists for a licence to protect, so the question a
		// licence answers is not the one being asked (ADR-047 決策 4). Separate
		// from self_supplied on purpose: that one asks whether the user had the
		// right to redistribute somebody else's bytes, this one asks who owns
		// what a model wrote, and one value for two questions eventually gets
		// one of them released by an answer to the other.
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
	files, dropped, err := collect(fsys)
	if err != nil {
		return err
	}
	// What the exporter removed, and whether SKILL.md needed any of it.
	//
	// The two halves are one check on purpose. Listing the removals is a
	// disclosure the author can act on; refusing when the document points at one
	// of them is the platform declining to hand over a package **it** broke.
	// A reference that was already dangling when the version was imported is a
	// different fact and does not reach here — that one is the author's, and
	// skillpkg reports it as a warning and lets it ship (02:SKILL-002 asks for
	// the severities to be shown apart, not for this one to block).
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
	// Deduped again, and for the same reason: skillhub-manifest.json is an
	// outward-facing contract (ADR-027 decision 5), so a source package that
	// carries a file by that name must not get it shipped alongside the
	// platform's. Appending it before the dedupe rather than after is what makes
	// the platform's copy the surviving one — and stops a real source file from
	// being excluded from manifest_hash by a name it merely happens to share.
	files = dedupeByPath(append(files, manifest))

	zipped, err := writeZip(files, p.Profile.topLevelDir(skillName))
	if err != nil {
		return err
	}
	if err := checkProducedSize(len(zipped)); err != nil {
		return err
	}

	// The invariant of PACK-009, asserted on the bytes that would be handed out:
	// re-opening them the way import does must produce a package import would
	// accept.
	produced, err := skillpkg.PackageFS(zipped)
	if err != nil {
		return fmt.Errorf("the produced package could not be re-opened: %w", err)
	}
	final := skillpkg.Validate(produced)
	if final.Blocked {
		p.Validation = carriedValidation(final)
		p.BlockedReason = BlockedValidation
		p.BlockedMessage = "the package these settings would produce does not pass the validation " +
			"an import has to pass, so it must not be presented as a valid package"
		return nil
	}
	// 02:NFR-007 clause 3: the account printed inside skillhub-manifest.json and
	// the account the preview page shows are ONE list, from one run. `final` is
	// the gate — it decides whether these bytes may be handed over at all — but
	// it cannot also be the list, because it runs over a file set that includes
	// the manifest, and the manifest quotes URLs out of its own findings. Copying
	// final into the manifest would change the manifest, which would change
	// final: measured, not assumed (see the agreement test).
	//
	// So the reported list is the one over the package's CONTENT. What it leaves
	// out is INSTALL.md's own link and the manifest's echo of URLs already
	// disclosed one line above — the platform's two files talking about
	// themselves, which was never information about the Skill.
	p.Validation = carriedValidation(report)

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

// carriedValidation is the one validation account of a package: the block
// printed inside skillhub-manifest.json and the block the API returns for the
// preview. One function because two constructions of "the same list" is how the
// document and the screen came to disagree.
func carriedValidation(r skillpkg.Report) ManifestValidation {
	cat := r.Categorize()
	return ManifestValidation{
		Blocked:  r.Blocked,
		Errors:   toManifestFindings(cat.Errors),
		Warnings: toManifestFindings(cat.Warnings),
		Infos:    toManifestFindings(cat.Infos),
	}
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

	// VersionNumber and LatestVersionNumber are the two numbers that turn this
	// row from "a uuid" into "v3, and this skill is on v5" (04 丙-42). Both are
	// read from the same query as the rest of the row; nothing here is computed
	// from a second request the client would have to make and could skip.
	VersionNumber       int32    `json:"version_number"`
	LatestVersionNumber int32    `json:"latest_version_number"`
	VersionState        labelled `json:"version_state"`
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
// Expiry outranks the purge it caused, because expiry is the reason and that
// purge is its consequence. A purge expiry did NOT cause outranks both — see the
// `lost` case, which is also where the row's own timestamps say which is which.
// withVersionState answers "am I looking at the newest content", which is a
// different question from withServeState's "can I have these bytes" and is kept
// on its own axis for that reason (設計系統 §2.12). A superseded package is still
// perfectly downloadable and downloading it can be exactly what was wanted.
//
// The note carries the numbers because the reader's next question is always
// "how far behind", and a label that says 已被取代 without saying by what sends
// them to another page to find out.
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
	// Which of the two purge writers stamped this row, asked of the row itself
	// (04 丙-91). `purged_at` is written by retention and by the reconciler, and
	// nothing recorded which — so this used to be inferred from the ABSENCE of
	// an expiry, and the inference expired with the row: a package the platform
	// lost in June was retold as 「已過期」 in September, which is a failure
	// described to its owner as policy.
	//
	// The timestamps already separate them and keep separating them. Retention
	// only ever selects rows whose `expires_at` has passed
	// (ListArtifactsPastRetention), so a retention purge is always stamped at or
	// after the deadline; the reconciler only ever acts on rows still claiming
	// to be downloadable (ListArtifactsClaimingObject requires `expires_at >
	// now()`), so a loss is stamped before it. A purge before the deadline
	// therefore has exactly one possible author, and the comparison keeps that
	// answer after the deadline goes by. The one row this misreads is one that
	// expires between the sweep listing it and the sweep writing — it reads as
	// expired, in the same second it was going to be.
	lost := purged && !expiresAt.IsZero() && purgedAt.Before(expiresAt)
	switch {
	case a.Status == "quarantined":
		a.ServeState = labelled{"quarantined", "檢查中(尚未可下載)",
			"打包完成,驗證還沒結束。這是暫時狀態(ADR-003 隔離)。"}
	case a.Status == "rejected":
		a.ServeState = labelled{"rejected", "已拒絕(打包後未通過驗證)",
			"這一份不會被提供。要再拿到同樣的內容,回到該版本重新打包一次。"}
	// Above expiry on purpose. A row can be both, and once it is, 「已過期」 is
	// the true sentence that answers the wrong question: it tells the owner this
	// was expected and asks nothing of them, while the platform is the party
	// that lost something. NFR-002 3 wants the deletion traceable; a state that
	// names the wrong cause is worse than no state, because the person who could
	// have reported it now has no reason to.
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
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return Result{}, err
	}
	workspaceLocked, objectLocked := false, false
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), objectCleanupTimeout)
		defer cancel()
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
		return Result{Artifact: reusedArtifact(p.Skill.ID, existing), Duplicate: true}, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Result{}, err
	}
	exists, err := s.Store.Exists(ctx, objectKey)
	if err != nil {
		return Result{}, err
	}
	ownsObject, commitAttempted := false, false
	intentCreated := false
	// Workspace/content addressing plus the advisory lock make this compensation
	// private to this writer. An ambiguous Commit result is deliberately retained:
	// the row may actually have committed and deleting its object would be worse.
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
	// Existence alone does not prove a prior interrupted Put left complete bytes.
	// This key is content-addressed and locked, so overwriting it with the exact
	// planned ZIP is idempotent and repairs an ambiguous partial object.
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
	detail, err := q.CreateDownloadArtifactDetail(ctx, gen.CreateDownloadArtifactDetailParams{
		ArtifactID: row.ID, WorkspaceID: ws.ID, SkillVersionID: p.Version.ID,
		Target: p.Profile.ID, ProfileVersion: p.Profile.Version,
		PackagerVersion: PackagerVersion, ManifestHash: p.ManifestHash,
		IncludesTestCases: p.IncludeTestCases,
	})
	if err != nil {
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
		LatestVersionNumber: detail.LatestVersionNumber,
	}.withVersionState().withServeState(row.ExpiresAt.Time, time.Time{})}, nil
}

func reusedArtifact(skillID pgtype.UUID, row gen.FindReusableDownloadArtifactRow) Artifact {
	return Artifact{
		ArtifactID:          pgconv.UUIDString(row.ArtifactID),
		SkillID:             pgconv.UUIDString(skillID),
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
		VersionNumber:       row.VersionNumber,
		LatestVersionNumber: row.LatestVersionNumber,
		// FindReusableDownloadArtifact filters `purged_at IS NULL`, so a reused row
		// is by construction not purged — the query is where that is enforced.
	}.withVersionState().withServeState(row.ExpiresAt.Time, time.Time{})
}
