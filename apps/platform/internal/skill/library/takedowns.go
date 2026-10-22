package registry

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type Takedowns struct {
	Registry *Service
}

func (t *Takedowns) Takedown(ctx context.Context, ws identity.Workspace, skillID pgtype.UUID, reason string) (Skill, error) {
	if err := t.Registry.requireProjection(); err != nil {
		return Skill{}, err
	}
	tx, err := t.Registry.Pool.Begin(ctx)
	if err != nil {
		return Skill{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	root, err := loadSkill(ctx, gen.New(tx), ws.ID, skillID)
	if err != nil {
		return Skill{}, err
	}
	root.TakeDown(reason, TakedownByOwner)
	if err := SaveSkill(ctx, tx, root); err != nil {
		return Skill{}, err
	}
	skill := root.row

	if err := t.Registry.RemoveFromIndex(ctx, tx, skill.WorkspaceID, skill.ID); err != nil {
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

func (t *Takedowns) Restore(ctx context.Context, ws identity.Workspace, skillID pgtype.UUID) (Skill, error) {
	if err := t.Registry.requireProjection(); err != nil {
		return Skill{}, err
	}
	tx, err := t.Registry.Pool.Begin(ctx)
	if err != nil {
		return Skill{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	root, err := loadSkill(ctx, gen.New(tx), ws.ID, skillID)
	if err != nil {
		return Skill{}, err
	}
	if err := restoreSkill(ctx, tx, t.Registry, root, TakedownByOwner); err != nil {
		return Skill{}, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        ws.OwnerUserID,
		Workspace:    ws.ID,
		Action:       audit.ActionSkillRestore,
		ResourceType: audit.ResourceSkill,
		ResourceID:   skillID,
	}); err != nil {
		return Skill{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Skill{}, err
	}
	return root.Skill(), nil
}

func LiftTakedown(ctx context.Context, tx pgx.Tx, registry *Service, skillID pgtype.UUID) (TakedownBefore, error) {
	root, err := loadSkillForOperatorHold(ctx, gen.New(tx), skillID)
	if err != nil {
		return TakedownBefore{}, err
	}
	if err := restoreSkill(ctx, tx, registry, root, TakedownByOperator); err != nil {
		return TakedownBefore{}, err
	}
	return TakedownBefore{WorkspaceID: root.row.WorkspaceID}, nil
}

func restoreSkill(ctx context.Context, tx pgx.Tx, registry *Service, root *SkillRoot, by TakedownBy) error {
	root.Restore(by, time.Now())
	if err := SaveSkill(ctx, tx, root); err != nil {
		return err
	}
	skill := root.row
	summary := ""
	if skill.Summary != nil {
		summary = *skill.Summary
	}
	return registry.IndexSkill(ctx, tx, SkillProjection{
		SkillID: skill.ID, WorkspaceID: skill.WorkspaceID, Name: skill.Name, Summary: summary,
	})
}
