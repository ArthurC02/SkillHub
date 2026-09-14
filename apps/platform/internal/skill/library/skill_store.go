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

func LoadSkill(ctx context.Context, tx pgx.Tx, workspaceID, skillID pgtype.UUID) (*SkillRoot, error) {
	return loadSkill(ctx, gen.New(tx), workspaceID, skillID)
}

func LoadSkillNamed(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID, name string) (*SkillRoot, bool, error) {
	q := gen.New(tx)
	row, err := q.GetSkillByName(ctx, gen.GetSkillByNameParams{WorkspaceID: workspaceID, Name: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	root, err := loadSkill(ctx, q, workspaceID, row.ID)
	return root, err == nil, err
}

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

func SaveSkill(ctx context.Context, tx pgx.Tx, root *SkillRoot) error {
	if refused, ok := root.Refusal(); ok {
		return refused.err()
	}
	return saveSkill(ctx, tx, root)
}

func saveSkill(ctx context.Context, tx pgx.Tx, root *SkillRoot) error {
	q := gen.New(tx)
	for i := root.saved; i < len(root.events); i++ {
		event, err := writeSkillEvent(ctx, q, root, root.events[i])
		if err != nil {
			return err
		}
		root.events[i] = event
		if err := outbox.Insert(ctx, tx, outbox.NewEvent{
			EventType: event.eventType(), EventVersion: outbox.EventVersion1,
			CorrelationID: root.row.ID, WorkspaceID: root.row.WorkspaceID,
			AggregateType: outbox.AggregateSkill, AggregateID: root.row.ID, Payload: event,
		}); err != nil {
			return err
		}
	}
	root.saved = len(root.events)
	return nil
}

func writeSkillEvent(ctx context.Context, q *gen.Queries, root *SkillRoot, event Event) (Event, error) {
	var err error
	switch e := event.(type) {
	case SkillCreated:
		root.row, err = q.CreateSkill(ctx, gen.CreateSkillParams{
			WorkspaceID: root.row.WorkspaceID, Name: root.row.Name, Summary: root.row.Summary,
			ForkedFromSkillID: root.row.ForkedFromSkillID, ForkedFromVersionID: root.row.ForkedFromVersionID,
			AccessRestriction: root.row.AccessRestriction, Redistribution: &root.row.Redistribution,
			Category: root.row.Category, CategorySource: root.row.CategorySource,
		})
	case SkillVersionAdded:
		content := root.pending
		root.added, err = q.CreateSkillVersion(ctx, gen.CreateSkillVersionParams{
			WorkspaceID: root.row.WorkspaceID, SkillID: root.row.ID, SourceID: content.sourceID,
			ContentHash: content.contentHash, PackageObjectKey: content.packageObjectKey,
			Manifest: content.manifest, LicenseExpression: content.license, LicenseSource: content.licenseSource,
		})
		e.VersionID, e.VersionNumber = root.added.ID, root.added.VersionNumber
		event = e
	case SkillDescribed:
		err = q.UpdateSkillSummary(ctx, gen.UpdateSkillSummaryParams{
			ID: root.row.ID, WorkspaceID: root.row.WorkspaceID, Summary: root.row.Summary,
		})
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
	return event, err
}
