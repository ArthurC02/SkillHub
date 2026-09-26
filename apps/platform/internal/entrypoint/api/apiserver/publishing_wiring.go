package apiserver

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/publishing"
)

func newPublishingService(cfg Config, registrySvc *registry.Service) *publishing.Service {
	svc := &publishing.Service{Pool: cfg.Pool}
	if cfg.Store != nil {
		svc.Store = cfg.Store
	}
	svc.LockSkillForRelease = func(ctx context.Context, tx pgx.Tx, workspaceID, skillID pgtype.UUID) (publishing.SkillFacts, bool, error) {
		skill, found, err := registrySvc.LockLiveWorkspaceSkill(ctx, tx, workspaceID, skillID)
		return publishingSkillFacts(skill), found, err
	}
	svc.ReadSkill = func(ctx context.Context, workspaceID, skillID pgtype.UUID) (publishing.SkillFacts, bool, error) {
		skill, found, err := registrySvc.WorkspaceSkill(ctx, workspaceID, skillID)
		return publishingSkillFacts(skill), found, err
	}
	svc.ReadVersion = func(ctx context.Context, workspaceID, versionID pgtype.UUID) (publishing.VersionFacts, bool, error) {
		version, found, err := registrySvc.WorkspaceVersion(ctx, workspaceID, versionID)
		return publishingVersionFacts(version), found, err
	}
	svc.LatestVersion = func(ctx context.Context, workspaceID, skillID pgtype.UUID) (publishing.VersionFacts, bool, error) {
		version, found, err := registrySvc.LatestVersion(ctx, workspaceID, skillID)
		return publishingVersionFacts(version), found, err
	}
	return svc
}

func publishingSkillFacts(skill registry.Skill) publishing.SkillFacts {
	facts := publishing.SkillFacts{
		ID: skill.ID, Name: skill.Name, TakenDown: skill.TakenDown(),
		AccessRestricted: skill.Restriction().InEffect(), Redistribution: skill.Redistribution,
	}
	if skill.Summary != nil {
		facts.Summary = *skill.Summary
	}
	return facts
}

func publishingVersionFacts(version registry.Version) publishing.VersionFacts {
	facts := publishing.VersionFacts{
		ID: version.ID, SkillID: version.SkillID, VersionNumber: version.VersionNumber,
		ContentHash: version.ContentHash, PackageObjectKey: version.PackageObjectKey, SourcePath: version.SourcePath,
	}
	if version.LicenseExpression != nil {
		facts.LicenseExpression = *version.LicenseExpression
	}
	if version.LicenseSource != nil {
		facts.LicenseSource = *version.LicenseSource
	}
	return facts
}

func describeRedistribution(value string) (label, note string) {
	display := catalog.Redistribution(value).Display()
	return display.Label, display.Note
}
