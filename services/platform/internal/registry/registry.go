// Package registry owns skills and their immutable versions (ADR-002 Skill
// Registry & Versioning). Fork copies nothing: packages are content-addressed
// in object storage, so a fork is two rows plus lineage (WS-001, iron rule 4).
package registry

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
)

var (
	// ErrNotFound: skill not visible in the caller's scope. Deliberately the
	// same answer for "does not exist" and "not yours" (WS-006).
	ErrNotFound = errors.New("skill not found")
	// ErrNameTaken: the caller already has a skill with this name.
	ErrNameTaken = errors.New("a skill with this name already exists in your workspace")
)

type Service struct {
	Pool *pgxpool.Pool
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

	// Readable scope today is the caller's own workspace; widens to curated
	// public content with the CONTENT milestone.
	src, err := q.GetSkill(ctx, gen.GetSkillParams{ID: skillID, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Skill{}, gen.SkillVersion{}, ErrNotFound
	}
	if err != nil {
		return gen.Skill{}, gen.SkillVersion{}, err
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
	return fork, ver, tx.Commit(ctx)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
