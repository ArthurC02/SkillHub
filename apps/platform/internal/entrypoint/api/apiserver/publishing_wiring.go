package apiserver

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/delivery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/publishing"
)

func newPublishingService(cfg Config, registrySvc *registry.Service, packagingSvc *packaging.Service) *publishing.Service {
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
	svc.PackageForRecipient = func(
		ctx context.Context, recipient identity.Workspace, ownerWorkspaceID, skillID, versionID pgtype.UUID,
	) (publishing.Acquisition, error) {
		result, err := packagingSvc.CreateForRecipient(ctx, recipient, ownerWorkspaceID, skillID, versionID)
		if errors.Is(err, packaging.ErrNotFound) {
			return publishing.Acquisition{}, publishing.ErrNotFound
		}
		if err != nil {
			return publishing.Acquisition{}, err
		}
		if result.Plan != nil && !result.Plan.Allowed {
			return publishing.Acquisition{}, &publishing.RefusedError{
				Reason: publishing.Refusal(result.Plan.BlockedReason), Message: result.Plan.BlockedMessage,
			}
		}
		return acquisitionOf(result.Artifact, result.Duplicate), nil
	}
	svc.PackagePluginForRecipient = func(
		ctx context.Context, recipient identity.Workspace, ownerWorkspaceID pgtype.UUID, plugin publishing.PluginRequest,
	) (publishing.Acquisition, error) {
		spec := packaging.PluginSpec{Name: plugin.Name, Version: plugin.Version, Description: plugin.Description}
		for _, m := range plugin.Members {
			spec.Members = append(spec.Members, packaging.PluginMember{SkillID: m.SkillID, VersionID: m.VersionID})
		}
		result, err := packagingSvc.CreatePluginForRecipient(ctx, recipient, ownerWorkspaceID, spec)
		if errors.Is(err, packaging.ErrNotFound) {
			return publishing.Acquisition{}, publishing.ErrNotFound
		}
		if err != nil {
			return publishing.Acquisition{}, err
		}
		if result.Plan != nil && !result.Plan.Allowed {
			return publishing.Acquisition{}, &publishing.RefusedError{
				Reason: publishing.Refusal(result.Plan.BlockedReason), Message: result.Plan.BlockedMessage,
			}
		}
		return acquisitionOf(result.Artifact, result.Duplicate), nil
	}
	return svc
}

func acquisitionOf(artifact packaging.Artifact, duplicate bool) publishing.Acquisition {
	return publishing.Acquisition{
		ArtifactID: artifact.ArtifactID, FileName: artifact.FileName,
		SizeBytes: artifact.SizeBytes, ContentHash: artifact.ContentHash,
		ExpiresAt: artifact.ExpiresAt, Duplicate: duplicate,
	}
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

func wireExposure(catalogSvc *catalog.Service, publishingSvc *publishing.Service) {
	publishingSvc.ReadSearchSnapshot = func(ctx context.Context, skillID pgtype.UUID) (publishing.SearchSnapshot, bool, error) {
		snapshot, found, err := catalogSvc.SearchSnapshotOf(ctx, skillID)
		return publishing.SearchSnapshot{
			VersionID: snapshot.VersionID, Name: snapshot.Name, Summary: snapshot.Summary,
			EnrichedSummary: snapshot.EnrichedSummary, TaskExamples: snapshot.TaskExamples, Tags: snapshot.Tags,
			Limitations: snapshot.Limitations, Enriched: snapshot.Enriched, Listable: snapshot.Listable,
			Digest: snapshot.Digest,
		}, found, err
	}
	catalogSvc.ExposedSkills = func(ctx context.Context) ([]catalog.ExposedSkill, error) {
		exposed, err := publishingSvc.ExposedSkills(ctx)
		if err != nil {
			return nil, err
		}
		out := make([]catalog.ExposedSkill, 0, len(exposed))
		for _, e := range exposed {
			out = append(out, catalog.ExposedSkill{
				SkillID: e.SkillID, VersionID: e.VersionID, SnapshotDigest: e.SnapshotDigest, OwnerWorkspaceID: e.OwnerWorkspaceID,
			})
		}
		return out, nil
	}
}
