package registry

// Skill identity and lineage: fork, soft delete, takedown. These write the
// mutable `skills` row; the one place here that writes an immutable version row
// is Fork. See doc.go for the aggregate rules it has to respect.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/pgconv"
)

var (
	// ErrNotFound: skill not visible in the caller's scope. Deliberately the
	// same answer for "does not exist" and "not yours" (WS-006).
	ErrNotFound = errors.New("skill not found")
	// ErrNameTaken: the caller already has a skill with this name.
	ErrNameTaken = errors.New("a skill with this name already exists in your workspace")
)

// ObjectStore is the slice of object storage the registry needs (diff reads).
type ObjectStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
}

type Service struct {
	Pool  *pgxpool.Pool
	Store ObjectStore
}

// Fork clones the latest version of a readable skill into ws. Provenance
// lives in forked_from_skill_id / forked_from_version_id; the package object
// is shared, not copied.
func (s *Service) Fork(ctx context.Context, ws gen.Workspace, skillID pgtype.UUID) (gen.Skill, gen.SkillVersion, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.Skill{}, gen.SkillVersion{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	// Readable scope is the caller's own workspace plus the public catalog:
	// forking a catalog entry into your own workspace is the whole point of
	// DISC/WS-001, and looking only at the caller's workspace made every catalog
	// fork a 404. Order matters — own workspace first, so a caller's private
	// skill is never shadowed by a catalog skill sharing an id (it cannot, but
	// the cheap read is also the correct one). Anything outside both scopes
	// still answers ErrNotFound, so another user's private skill is unchanged.
	src, err := q.GetSkill(ctx, gen.GetSkillParams{ID: skillID, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		src, err = q.GetCatalogSkill(ctx, skillID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Skill{}, gen.SkillVersion{}, ErrNotFound
	}
	if err != nil {
		return gen.Skill{}, gen.SkillVersion{}, err
	}
	// A taken-down skill is not a fork source (INGEST-010): the takedown exists
	// because the content should stop spreading, and a fork is a new copy.
	// Existing forks are untouched — theirs are already separate rows.
	if src.TakedownAt.Valid {
		return gen.Skill{}, gen.SkillVersion{}, ErrNotFound
	}
	srcVer, err := q.GetLatestSkillVersion(ctx, gen.GetLatestSkillVersionParams{
		SkillID: src.ID, WorkspaceID: src.WorkspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Skill{}, gen.SkillVersion{}, ErrNotFound
	}
	if err != nil {
		return gen.Skill{}, gen.SkillVersion{}, err
	}

	fork, err := q.CreateSkill(ctx, gen.CreateSkillParams{
		WorkspaceID:         ws.ID,
		Name:                src.Name + "-fork",
		Summary:             src.Summary,
		ForkedFromSkillID:   src.ID,
		ForkedFromVersionID: srcVer.ID,
		// A licensing hold travels with the copy (0023). Forking is how the
		// materials reach a workspace where they could be read in full or run, so
		// a fork that dropped the hold would be the way around it.
		AccessRestriction: src.AccessRestriction,
		// So does the redistribution verdict (0027, ADR-027 decision 4), for the
		// same reason plus one of its own: a fork that started again at the
		// column's conservative default would silently lose an `allowed` somebody
		// established, and letting the default answer for a copy is how a hold
		// becomes a formality.
		Redistribution: &src.Redistribution,
	})
	if isUniqueViolation(err) {
		return gen.Skill{}, gen.SkillVersion{}, ErrNameTaken
	}
	if err != nil {
		return gen.Skill{}, gen.SkillVersion{}, err
	}

	// source_id stays NULL: skill_sources rows belong to the origin workspace;
	// fork provenance is carried by the lineage columns instead (INGEST-004).
	ver, err := q.CreateSkillVersion(ctx, gen.CreateSkillVersionParams{
		WorkspaceID:       ws.ID,
		SkillID:           fork.ID,
		ContentHash:       srcVer.ContentHash,
		PackageObjectKey:  srcVer.PackageObjectKey,
		Manifest:          srcVer.Manifest,
		LicenseExpression: srcVer.LicenseExpression,
		// WS-001 requires a fork to keep the License relationship, which is the
		// expression *and* the tier it was established at (ADR-021). Copying the
		// expression alone would silently upgrade a repo-level license into one
		// the fork appears to declare for itself.
		LicenseSource: srcVer.LicenseSource,
	})
	if err != nil {
		return gen.Skill{}, gen.SkillVersion{}, err
	}

	summary := ""
	if fork.Summary != nil {
		summary = *fork.Summary
	}
	if err := q.UpsertSearchDocument(ctx, gen.UpsertSearchDocumentParams{
		SkillID:     fork.ID,
		WorkspaceID: ws.ID,
		Name:        fork.Name,
		Summary:     summary,
	}); err != nil {
		return gen.Skill{}, gen.SkillVersion{}, err
	}
	if err := audit.Log(ctx, q, audit.Event{
		Actor:        ws.OwnerUserID,
		Workspace:    ws.ID,
		Action:       audit.ActionSkillFork,
		ResourceType: audit.ResourceSkill,
		ResourceID:   fork.ID,
		Metadata: map[string]any{
			"source_skill_id":   pgconv.UUIDString(src.ID),
			"source_version_id": pgconv.UUIDString(srcVer.ID),
		},
	}); err != nil {
		return gen.Skill{}, gen.SkillVersion{}, err
	}
	return fork, ver, tx.Commit(ctx)
}

// DeleteResult tells the user what the deletion covered (WS-005 狀態回饋).
type DeleteResult struct {
	VersionsRetained int64
}

// Delete soft-deletes a skill: it vanishes from lists, reads, and search now;
// version snapshots stay frozen (0005 trigger) until the 30-day grace purge
// (PDM-006). The content-addressed package objects are shared with forks, so
// they are never removed here.
// ponytail: the hard-purge background job lands with the retention work.
func (s *Service) Delete(ctx context.Context, ws gen.Workspace, skillID pgtype.UUID) (DeleteResult, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return DeleteResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	skill, err := q.SoftDeleteSkill(ctx, gen.SoftDeleteSkillParams{ID: skillID, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return DeleteResult{}, ErrNotFound
	}
	if err != nil {
		return DeleteResult{}, err
	}
	if err := q.DeleteSearchDocument(ctx, skill.ID); err != nil {
		return DeleteResult{}, err
	}
	n, err := q.CountSkillVersions(ctx, skill.ID)
	if err != nil {
		return DeleteResult{}, err
	}
	if err := audit.Log(ctx, q, audit.Event{
		Actor:        ws.OwnerUserID,
		Workspace:    ws.ID,
		Action:       audit.ActionSkillDelete,
		ResourceType: audit.ResourceSkill,
		ResourceID:   skill.ID,
		Metadata:     map[string]any{"versions_retained": n},
	}); err != nil {
		return DeleteResult{}, err
	}
	return DeleteResult{VersionsRetained: n}, tx.Commit(ctx)
}

// ErrAlreadyTakenDown: the skill is already off the catalog.
var ErrAlreadyTakenDown = errors.New("skill is already taken down")

// Takedown removes a skill from search and from the fork path while keeping
// every row it owns (INGEST-010 人工下架; PDM-006 §6 lists takedown as the only
// removal path for skills and versions, and it marks invisible rather than
// deletes). Existing forks, historical lineage and past runs are unaffected.
//
// Authorization is the ordinary workspace scope: the operator who curates the
// catalog owns the catalog workspace, so taking curated content down is the
// same statement as taking your own content down.
//
// ponytail: no operator role. A platform-wide takedown of content in *someone
// else's* workspace — abuse reports, DMCA — has no path here and needs the
// admin role SEC/CORE has not specified yet. Upgrade path is a second query
// without the workspace predicate behind that role, not a change to this one.
func (s *Service) Takedown(ctx context.Context, ws gen.Workspace, skillID pgtype.UUID, reason string) (gen.Skill, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.Skill{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	skill, err := q.TakedownSkill(ctx, gen.TakedownSkillParams{
		ID: skillID, WorkspaceID: ws.ID, Reason: &reason,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Either it is not visible here, or it is already down. Tell the two
		// apart with a scoped read; both answers are safe to give the owner.
		if _, getErr := q.GetSkill(ctx, gen.GetSkillParams{ID: skillID, WorkspaceID: ws.ID}); getErr == nil {
			return gen.Skill{}, ErrAlreadyTakenDown
		}
		return gen.Skill{}, ErrNotFound
	}
	if err != nil {
		return gen.Skill{}, err
	}
	// Same transaction as the flag, so the content can never be down in the
	// registry but still listed in search.
	if err := q.DeleteSearchDocument(ctx, skill.ID); err != nil {
		return gen.Skill{}, err
	}
	if err := audit.Log(ctx, q, audit.Event{
		Actor:        ws.OwnerUserID,
		Workspace:    ws.ID,
		Action:       audit.ActionSkillTakedown,
		ResourceType: audit.ResourceSkill,
		ResourceID:   skill.ID,
		// The reason text lives on the skills row; the audit event stays
		// identifiers-only (PDM-006 §6 "不含內容").
	}); err != nil {
		return gen.Skill{}, err
	}
	return skill, tx.Commit(ctx)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
