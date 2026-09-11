package wiring

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	registry "github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

func WireRunRegistryReaders(runs *run.Service, registryService *registry.Service) {
	runs.ReadSkill = func(ctx context.Context, workspaceID, skillID pgtype.UUID) (run.SkillFacts, bool, error) {
		skill, found, err := registryService.WorkspaceSkill(ctx, workspaceID, skillID)
		return run.SkillFacts{AccessRestriction: skill.AccessRestriction}, found, err
	}
	runs.ReadVersion = func(ctx context.Context, workspaceID, versionID pgtype.UUID) (run.VersionFacts, bool, error) {
		version, found, err := registryService.WorkspaceVersion(ctx, workspaceID, versionID)
		return run.VersionFacts{
			ID: version.ID, SkillID: version.SkillID, ContentHash: version.ContentHash,
			PackageObjectKey: version.PackageObjectKey,
		}, found, err
	}
	runs.ReadVersionSummaries = func(
		ctx context.Context, workspaceID pgtype.UUID, versionIDs []pgtype.UUID,
	) (map[pgtype.UUID]run.VersionSummary, error) {
		summaries, err := registryService.VersionSummaries(ctx, workspaceID, versionIDs)
		if err != nil {
			return nil, err
		}
		out := make(map[pgtype.UUID]run.VersionSummary, len(summaries))
		for id, summary := range summaries {
			out[id] = run.VersionSummary{SkillID: summary.SkillID, SkillName: summary.SkillName}
		}
		return out, nil
	}
	runs.ReadContentSource = readContentSource(registryService)
}

func readContentSource(registryService *registry.Service) func(context.Context, pgtype.UUID, pgtype.UUID) (run.ContentSource, bool, error) {
	return func(ctx context.Context, workspaceID, versionID pgtype.UUID) (run.ContentSource, bool, error) {
		version, found, err := registryService.WorkspaceVersion(ctx, workspaceID, versionID)
		if err != nil || !found {
			return run.ContentSource{}, found, err
		}
		skill, found, err := registryService.WorkspaceSkill(ctx, workspaceID, version.SkillID)
		if err != nil || !found {
			return run.ContentSource{}, found, err
		}
		_, inCatalogue, err := registryService.CatalogSkill(ctx, version.SkillID)
		if err != nil {
			return run.ContentSource{}, false, err
		}
		return run.ContentSource{
			WorkspaceIsCatalog:      inCatalogue,
			CurationTier:            skill.CurationTier,
			CuratedVersionIsThisOne: skill.CuratedVersionID == versionID,
		}, true, nil
	}
}

func NewTraceService(pool *pgxpool.Pool, signer *trace.Signer, runs *run.Service) *trace.Service {
	return &trace.Service{
		Pool: pool, Signer: signer,
		ReadRunState: func(ctx context.Context, workspaceID, runID pgtype.UUID) (trace.RunState, bool, error) {
			state, found, err := runs.TraceRun(ctx, workspaceID, runID)
			return trace.RunState{Status: state.Status, StatusReason: state.StatusReason}, found, err
		},
		ReadIngestRunState: func(ctx context.Context, runID pgtype.UUID) (trace.IngestRunState, bool, error) {
			state, found, err := runs.TraceIngestRun(ctx, runID)
			return trace.IngestRunState{
				ID: state.ID, WorkspaceID: state.WorkspaceID, Status: state.Status, FinishedAt: state.FinishedAt,
			}, found, err
		},
		ReadRunTransitions: func(ctx context.Context, workspaceID, runID pgtype.UUID) ([]trace.RunTransition, error) {
			rows, err := runs.TraceTransitions(ctx, workspaceID, runID)
			if err != nil {
				return nil, err
			}
			out := make([]trace.RunTransition, len(rows))
			for i, row := range rows {
				out[i] = trace.RunTransition{ToStatus: row.ToStatus, Reason: row.Reason}
			}
			return out, nil
		},
	}
}

func WireEvaluationRunReaders(service *eval.Service, runs *run.Service) {
	service.ReadRunFacts = func(ctx context.Context, workspaceID, runID pgtype.UUID) (eval.RunFacts, bool, error) {
		facts, found, err := runs.EvaluationRun(ctx, workspaceID, runID)
		return evalRunFacts(facts), found, err
	}
	service.ReadEvaluationInput = func(ctx context.Context, workspaceID, runID pgtype.UUID) (eval.EvaluationInput, bool, error) {
		input, found, err := runs.EvaluationInput(ctx, workspaceID, runID)
		artifacts := make([]eval.ArtifactFacts, len(input.Artifacts))
		for i, artifact := range input.Artifacts {
			artifacts[i] = eval.ArtifactFacts{
				FileName: artifact.FileName, ContentType: artifact.ContentType,
				SizeBytes: artifact.SizeBytes, ContentHash: artifact.ContentHash,
			}
		}
		return eval.EvaluationInput{
			Run: evalRunFacts(input.Run), Artifacts: artifacts, LatestAttempt: input.LatestAttempt,
			Absent: eval.ArtifactAbsence{
				Deleted: input.Absent.Deleted,
				Expired: input.Absent.Expired,
			},
		}, found, err
	}
}

func WireEvaluationRegistryReaders(service *eval.Service, registryService *registry.Service) {
	service.ReadVersion = func(ctx context.Context, workspaceID, versionID pgtype.UUID) (eval.VersionFacts, bool, error) {
		version, found, err := registryService.WorkspaceVersion(ctx, workspaceID, versionID)
		return eval.VersionFacts{ID: version.ID, SkillID: version.SkillID, PackageObjectKey: version.PackageObjectKey}, found, err
	}
	service.ReadLatestVersion = func(ctx context.Context, workspaceID, skillID pgtype.UUID) (eval.VersionFacts, bool, error) {
		version, found, err := registryService.LatestVersion(ctx, workspaceID, skillID)
		return eval.VersionFacts{ID: version.ID, SkillID: version.SkillID, PackageObjectKey: version.PackageObjectKey}, found, err
	}
	service.ReadSkill = func(ctx context.Context, workspaceID, skillID pgtype.UUID) (eval.SkillFacts, bool, error) {
		skill, found, err := registryService.WorkspaceSkill(ctx, workspaceID, skillID)
		return eval.SkillFacts{
			ID: skill.ID, Name: skill.Name, Summary: skill.Summary, AccessRestriction: skill.AccessRestriction,
		}, found, err
	}
	service.ReadRuntimeCompatibility = func(ctx context.Context, versionID pgtype.UUID) (eval.RuntimeCompatibility, bool, error) {
		compat, found, err := registryService.RuntimeCompatibility(ctx, versionID)
		return eval.RuntimeCompatibility{
			Capability: compat.Capability, Runtime: compat.Runtime, RuntimeImage: compat.RuntimeImage,
		}, found, err
	}
}

func evalRunFacts(facts run.EvaluationRun) eval.RunFacts {
	return eval.RunFacts{
		ID: facts.ID, WorkspaceID: facts.WorkspaceID,
		SkillVersionID: facts.SkillVersionID, TestCaseSnapshotID: facts.TestCaseSnapshotID,
		Status: facts.Status, StatusReason: facts.StatusReason, RuntimeSnapshot: facts.RuntimeSnapshot,
		StartedAt: facts.StartedAt, FinishedAt: facts.FinishedAt, FailureClass: facts.FailureClass,
		Terminal: run.IsTerminal(gen.RunStatus(facts.Status)),
	}
}
