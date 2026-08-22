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

// Import-side writes. `skills` and `skill_versions` are registry's tables, but
// the only path allowed to create rows in them is ingest's import pipeline
// (AGENTS.md: "版本寫入的唯一驗證路徑"). Until now ingest reached past the owner
// and called the queries itself — one of the tolerated drifts in
// db/query-owners.yaml. These three functions close that hole the way ADR-033
// clearance path 2 describes: validation stays in ingest, the write comes back
// to the owner.
//
// They take the caller's transaction instead of opening their own on purpose.
// One import commits the version row, the search projection (INGEST-009 requires
// the same transaction) and the audit event together (iron rule 9); a registry
// function that began its own transaction would split that single guarantee into
// three that can each fail alone.
//
// Every one of them takes a skillpkg.Report rather than loose strings, so the
// argument that decides what gets written is the validation result itself and
// cannot be assembled by a caller that skipped validation. The Report must come
// from a completed, passing skillpkg.Validate run — the guard below only catches
// the obvious forgeries (blocked report, absent manifest), it cannot prove the
// scan and license resolution actually ran.

// ErrUnvalidatedPackage: the report handed in is not a passing validation
// result, so there is nothing here that may become an immutable version.
var ErrUnvalidatedPackage = errors.New("registry: package report is not a passing validation result")

func validatedManifest(report skillpkg.Report) (*skillpkg.Manifest, error) {
	if report.Blocked || report.Manifest == nil {
		return nil, ErrUnvalidatedPackage
	}
	return report.Manifest, nil
}

// NewVersion is one validated package about to become the next immutable
// version of SkillID. The version number is allocated by the query, never by
// the caller.
type NewVersion struct {
	WorkspaceID      pgtype.UUID
	SkillID          pgtype.UUID
	SourceID         pgtype.UUID // NULL for content that has no skill_sources row
	ContentHash      string
	PackageObjectKey string
	Report           skillpkg.Report
}

// RedistributionSelfSupplied is skills.redistribution's fourth value (0036):
// this workspace supplied the bytes, so handing them back is retrieval and not
// redistribution. It releases the packaging gate without asserting anything
// about the licence, which is why it is a separate value from `allowed`.
//
// registry owns the writes to skills, so the constant lives here. delivery and
// discovery each carry their own copy of this vocabulary already — the database
// CHECK is what actually holds them together.
const RedistributionSelfSupplied = "self_supplied"

// CreateSkillFromPackage inserts the skills row for a package being imported
// under a name that does not exist in the workspace yet. Lineage columns stay
// unset: an import carries none.
//
// `redistribution` is a parameter and not a default, since 0036. An import
// carries no verdict about the licence — that part of the 0027 reasoning still
// holds — but it does carry one fact the column has to be able to express: who
// supplied the bytes. The caller knows it and this function does not, so it is
// passed in rather than guessed. Empty means "say nothing", which lands on the
// column's own conservative default.
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

// CreateVersionFromPackage writes the immutable version row. Existing versions
// are never touched (iron rule 4); the caller is responsible for having already
// established that this content is not a duplicate.
func CreateVersionFromPackage(ctx context.Context, tx pgx.Tx, v NewVersion) (Version, error) {
	manifest, err := validatedManifest(v.Report)
	if err != nil {
		return Version{}, err
	}
	// The manifest column is the snapshot's own truth: the skills row keeps its
	// name (a fork's name differs from its manifest), the version keeps what the
	// package actually declared.
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

// UpdateSummaryFromPackage refreshes the mutable summary on an existing skill
// after a new version landed. Workspace scoped, so a skill id from another
// tenant updates nothing rather than the wrong row.
func UpdateSummaryFromPackage(ctx context.Context, tx pgx.Tx, skillID, workspaceID pgtype.UUID, report skillpkg.Report) error {
	manifest, err := validatedManifest(report)
	if err != nil {
		return err
	}
	return gen.New(tx).UpdateSkillSummary(ctx, gen.UpdateSkillSummaryParams{
		ID: skillID, WorkspaceID: workspaceID, Summary: &manifest.Description,
	})
}

// versionLicense splits the resolved license into the two columns ADR-021 keeps
// apart: "MIT" declared in frontmatter and "MIT" read off a repository-level
// file are not the same claim, so the provenance tier travels with the
// expression instead of being flattened into it. An unresolved license leaves
// both columns NULL — DISC-003 has to be able to say "unknown", and a string
// there would read like a declaration nobody made.
func versionLicense(report skillpkg.Report) (expression, source *string) {
	if report.LicenseExpression == "" {
		return nil, nil
	}
	return &report.LicenseExpression, &report.LicenseSource
}
