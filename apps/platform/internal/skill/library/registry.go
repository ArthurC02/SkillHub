package registry

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

var (
	ErrNotFound = errors.New("skill not found")

	ErrNameTaken = errors.New("你的工作區已經有同名的 Skill")
)

type ObjectStore interface {
	Get(ctx context.Context, key string) ([]byte, error)
}

type SkillProjection struct {
	SkillID     pgtype.UUID
	WorkspaceID pgtype.UUID
	Name        string
	Summary     string
}

type Service struct {
	Pool  *pgxpool.Pool
	Store ObjectStore

	IndexSkill      func(ctx context.Context, tx pgx.Tx, projection SkillProjection) error
	RemoveFromIndex func(ctx context.Context, tx pgx.Tx, workspaceID, skillID pgtype.UUID) error
	RefreshListing  func(ctx context.Context, db gen.DBTX, skillID pgtype.UUID) error

	SkillRisks func(ctx context.Context, workspaceID pgtype.UUID, skillIDs []pgtype.UUID) (map[string]json.RawMessage, error)

	CatalogSkillRisks func(ctx context.Context, skillIDs []pgtype.UUID) (map[string]json.RawMessage, error)

	CatalogWorkspaces func(ctx context.Context, db gen.DBTX) ([]pgtype.UUID, error)

	VersionsInRuns      ReferenceRead
	VersionsInDownloads ReferenceRead
	SkillsWithTestCases ReferenceRead
}

func (s *Service) requireProjection() error {
	if s.IndexSkill == nil || s.RemoveFromIndex == nil {
		return errors.New("registry: search projection writes not injected; refusing to write")
	}
	return nil
}

func (s *Service) catalogWorkspaceIDs(ctx context.Context, db gen.DBTX) ([]pgtype.UUID, error) {
	if s.CatalogWorkspaces == nil {
		return nil, errors.New("registry: catalog workspace read not injected")
	}
	return s.CatalogWorkspaces(ctx, db)
}

func (s *Service) catalogSkillIn(ctx context.Context, db gen.DBTX, skillID pgtype.UUID) (gen.Skill, error) {
	catalogs, err := s.catalogWorkspaceIDs(ctx, db)
	if err != nil {
		return gen.Skill{}, err
	}
	return gen.New(db).GetCatalogSkill(ctx, gen.GetCatalogSkillParams{ID: skillID, CatalogWorkspaceIds: catalogs})
}

func (s *Service) Fork(ctx context.Context, ws identity.Workspace, skillID pgtype.UUID) (Skill, Version, error) {
	if err := s.requireProjection(); err != nil {
		return Skill{}, Version{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Skill{}, Version{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	src, err := q.GetSkill(ctx, gen.GetSkillParams{ID: skillID, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		src, err = s.catalogSkillIn(ctx, tx, skillID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Skill{}, Version{}, ErrNotFound
	}
	if err != nil {
		return Skill{}, Version{}, err
	}

	if src.TakedownAt.Valid {
		return Skill{}, Version{}, ErrNotFound
	}
	srcVer, err := q.GetLatestSkillVersion(ctx, gen.GetLatestSkillVersionParams{
		SkillID: src.ID, WorkspaceID: src.WorkspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Skill{}, Version{}, ErrNotFound
	}
	if err != nil {
		return Skill{}, Version{}, err
	}

	name, err := s.forkName(ctx, tx, ws.ID, src.Name)
	if err != nil {
		return Skill{}, Version{}, err
	}
	root := forkOf(ws.ID, name, src, srcVer)
	if err := SaveSkill(ctx, tx, root); isUniqueViolation(err) {
		return Skill{}, Version{}, ErrNameTaken
	} else if err != nil {
		return Skill{}, Version{}, err
	}
	fork := root.row

	summary := ""
	if fork.Summary != nil {
		summary = *fork.Summary
	}
	if err := s.IndexSkill(ctx, tx, SkillProjection{
		SkillID:     fork.ID,
		WorkspaceID: ws.ID,
		Name:        fork.Name,
		Summary:     summary,
	}); err != nil {
		return Skill{}, Version{}, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
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
		return Skill{}, Version{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Skill{}, Version{}, err
	}
	return root.Skill(), root.AddedVersion(), nil
}

type DeleteResult struct {
	VersionsRetained int64
}

func (s *Service) Delete(ctx context.Context, ws identity.Workspace, skillID pgtype.UUID) (DeleteResult, error) {
	if err := s.requireProjection(); err != nil {
		return DeleteResult{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return DeleteResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	root, err := loadSkill(ctx, q, ws.ID, skillID)
	if err != nil {
		return DeleteResult{}, err
	}
	root.Delete()
	if err := SaveSkill(ctx, tx, root); err != nil {
		return DeleteResult{}, err
	}
	skill := root.row
	if err := s.RemoveFromIndex(ctx, tx, skill.WorkspaceID, skill.ID); err != nil {
		return DeleteResult{}, err
	}
	n, err := q.CountSkillVersions(ctx, skill.ID)
	if err != nil {
		return DeleteResult{}, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
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

var (
	ErrAlreadyTakenDown       = errors.New("skill is already taken down")
	ErrTakedownReasonRequired = errors.New("reason is required")
)

func (s *Service) Takedown(ctx context.Context, ws identity.Workspace, skillID pgtype.UUID, reason string) (Skill, error) {
	if err := s.requireProjection(); err != nil {
		return Skill{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Skill{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	root, err := loadSkill(ctx, gen.New(tx), ws.ID, skillID)
	if err != nil {
		return Skill{}, err
	}
	root.TakeDown(reason)
	if err := SaveSkill(ctx, tx, root); err != nil {
		return Skill{}, err
	}
	skill := root.row

	if err := s.RemoveFromIndex(ctx, tx, skill.WorkspaceID, skill.ID); err != nil {
		return Skill{}, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        ws.OwnerUserID,
		Workspace:    ws.ID,
		Action:       audit.ActionSkillTakedown,
		ResourceType: audit.ResourceSkill,
		ResourceID:   skill.ID,
	}); err != nil {
		return Skill{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Skill{}, err
	}
	return root.Skill(), nil
}

func isUniqueViolation(err error) bool {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	return ok && pgErr.Code == "23505"
}

const maxForkAttempts = 10

func (s *Service) forkName(
	ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID, sourceName string,
) (string, error) {
	base := strings.TrimSuffix(sourceName, "-fork")
	if trimmed, _, ok := cutForkOrdinal(base); ok {
		base = trimmed
	}
	for i := 1; i <= maxForkAttempts; i++ {
		name := base + "-fork"
		if i > 1 {
			name += "-" + strconv.Itoa(i)
		}
		if _, found, err := SkillByName(ctx, tx, workspaceID, name); err != nil {
			return "", err
		} else if !found {
			return name, nil
		}
	}
	return "", ErrNameTaken
}

func cutForkOrdinal(name string) (string, int, bool) {
	base, ordinal, found := strings.Cut(name, "-fork-")
	if !found || base == "" {
		return name, 0, false
	}
	n, err := strconv.Atoi(ordinal)
	if err != nil || n < 2 {
		return name, 0, false
	}
	return base, n, true
}
