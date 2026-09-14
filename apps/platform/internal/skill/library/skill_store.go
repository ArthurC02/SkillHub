package registry

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func loadSkill(ctx context.Context, q *gen.Queries, workspaceID, skillID pgtype.UUID) (*SkillRoot, error) {
	return skillRootOf(q.LockSkill(ctx, gen.LockSkillParams{ID: skillID, WorkspaceID: workspaceID}))
}

func loadSkillForOperator(ctx context.Context, q *gen.Queries, skillID pgtype.UUID) (*SkillRoot, error) {
	return skillRootOf(q.LockSkillForOperatorWrite(ctx, skillID))
}

func skillRootOf(row gen.Skill, err error) (*SkillRoot, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &SkillRoot{row: row}, nil
}

func loadNewestLicense(ctx context.Context, q *gen.Queries, root *SkillRoot) error {
	row, err := q.GetLatestVersionLicense(ctx, root.row.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	root.newest.exists = true
	if row.LicenseExpression != nil {
		root.newest.license.Expression = *row.LicenseExpression
	}
	if row.LicenseSource != nil {
		root.newest.license.Source = *row.LicenseSource
	}
	return nil
}

func saveUnlessRefused(ctx context.Context, tx pgx.Tx, root *SkillRoot) error {
	if refused, ok := root.Refusal(); ok {
		return refused.err()
	}
	return saveSkill(ctx, tx, root)
}

func saveSkill(ctx context.Context, tx pgx.Tx, root *SkillRoot) error {
	q := gen.New(tx)
	for _, event := range root.events {
		if err := writeSkillEvent(ctx, q, root, event); err != nil {
			return err
		}
		if err := outbox.Insert(ctx, tx, outbox.NewEvent{
			EventType: event.eventType(), EventVersion: outbox.EventVersion1,
			CorrelationID: root.row.ID, WorkspaceID: root.row.WorkspaceID,
			AggregateType: outbox.AggregateSkill, AggregateID: root.row.ID, Payload: event,
		}); err != nil {
			return err
		}
	}
	return nil
}

func writeSkillEvent(ctx context.Context, q *gen.Queries, root *SkillRoot, event Event) error {
	var err error
	switch event.(type) {
	case SkillTakenDown:
		root.row, err = q.SetSkillTakedown(ctx, gen.SetSkillTakedownParams{
			ID: root.row.ID, TakedownReason: &root.takedownReason,
		})
	case AccessRestricted, AccessRestrictionLifted:
		err = q.SetSkillAccessRestriction(ctx, gen.SetSkillAccessRestrictionParams{
			ID: root.row.ID, AccessRestriction: root.row.AccessRestriction,
		})
	case RedistributionSet:
		err = q.SetSkillRedistribution(ctx, gen.SetSkillRedistributionParams{
			ID: root.row.ID, Redistribution: root.row.Redistribution,
		})
	case SkillCategorized:
		root.row, err = q.SetSkillCategory(ctx, gen.SetSkillCategoryParams{
			ID: root.row.ID, WorkspaceID: root.row.WorkspaceID,
			Category: root.row.Category, CategorySource: root.row.CategorySource,
		})
	case SkillDeleted:
		root.row, err = q.SoftDeleteSkill(ctx, gen.SoftDeleteSkillParams{
			ID: root.row.ID, WorkspaceID: root.row.WorkspaceID,
		})
	default:
		err = fmt.Errorf("skill event %T has nothing to write", event)
	}
	return err
}
