package wiring

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	registry "github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/publishing"
)

func PublishingSkillFacts(skill registry.Skill) publishing.SkillFacts {
	facts := publishing.SkillFacts{
		ID: skill.ID, Name: skill.Name, TakenDown: skill.TakenDown(),
		AccessRestricted: skill.Restriction().InEffect(), Redistribution: skill.Redistribution,
	}
	if skill.Summary != nil {
		facts.Summary = *skill.Summary
	}
	return facts
}

func SearchSnapshotReader(catalogSvc *catalog.Service) func(context.Context, pgtype.UUID) (publishing.SearchSnapshot, bool, error) {
	return func(ctx context.Context, skillID pgtype.UUID) (publishing.SearchSnapshot, bool, error) {
		snapshot, found, err := catalogSvc.SearchSnapshotOf(ctx, skillID)
		return publishing.SearchSnapshot{
			VersionID: snapshot.VersionID, Name: snapshot.Name, Summary: snapshot.Summary,
			EnrichedSummary: snapshot.EnrichedSummary, TaskExamples: snapshot.TaskExamples, Tags: snapshot.Tags,
			Limitations: snapshot.Limitations, Enriched: snapshot.Enriched, Listable: snapshot.Listable,
			Digest: snapshot.Digest,
		}, found, err
	}
}

func NewExposureDocket(
	pool *pgxpool.Pool, registrySvc *registry.Service, catalogSvc *catalog.Service,
) func(context.Context) ([]publishing.DocketEntry, error) {
	svc := &publishing.Service{Pool: pool, ReadSearchSnapshot: SearchSnapshotReader(catalogSvc)}
	svc.ReadSkill = func(ctx context.Context, workspaceID, skillID pgtype.UUID) (publishing.SkillFacts, bool, error) {
		skill, found, err := registrySvc.WorkspaceSkill(ctx, workspaceID, skillID)
		return PublishingSkillFacts(skill), found, err
	}
	return svc.ExposureDocket
}
