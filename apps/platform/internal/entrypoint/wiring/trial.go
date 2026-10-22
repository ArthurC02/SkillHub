package wiring

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	registry "github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	trace "github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/readmodel"
	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/facts"
)

func WireRunRegistryReaders(runs *run.Service, registryService *registry.Service) {
	runs.Registry = runRegistryReader{registryService}
}

type runRegistryReader struct{ service *registry.Service }

func (r runRegistryReader) Skill(ctx context.Context, workspaceID, skillID pgtype.UUID) (run.SkillFacts, bool, error) {
	skill, found, err := r.service.WorkspaceSkill(ctx, workspaceID, skillID)
	return runSkillFacts(skill), found, err
}

func (r runRegistryReader) Version(ctx context.Context, workspaceID, versionID pgtype.UUID) (run.VersionFacts, bool, error) {
	version, found, err := r.service.WorkspaceVersion(ctx, workspaceID, versionID)
	return runVersionFacts(version), found, err
}

func runVersionFacts(version registry.Version) run.VersionFacts {
	return run.VersionFacts{ID: version.ID, SkillID: version.SkillID, ContentHash: version.ContentHash,
		PackageObjectKey: version.PackageObjectKey, SourcePath: version.SourcePath}
}

func (r runRegistryReader) VersionSummaries(
	ctx context.Context, workspaceID pgtype.UUID, versionIDs []pgtype.UUID,
) (map[pgtype.UUID]run.VersionSummary, error) {
	summaries, err := r.service.VersionSummaries(ctx, workspaceID, versionIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[pgtype.UUID]run.VersionSummary, len(summaries))
	for id, summary := range summaries {
		out[id] = run.VersionSummary{SkillID: summary.SkillID, SkillName: summary.SkillName}
	}
	return out, nil
}

func (r runRegistryReader) ContentSource(
	ctx context.Context, workspaceID, versionID pgtype.UUID,
) (run.ContentSource, bool, error) {
	version, found, err := r.service.WorkspaceVersion(ctx, workspaceID, versionID)
	if err != nil || !found {
		return run.ContentSource{}, found, err
	}
	skill, found, err := r.service.WorkspaceSkill(ctx, workspaceID, version.SkillID)
	if err != nil || !found {
		return run.ContentSource{}, found, err
	}
	_, inCatalogue, err := r.service.CatalogSkill(ctx, version.SkillID)
	if err != nil {
		return run.ContentSource{}, false, err
	}
	return run.ContentSource{WorkspaceIsCatalog: inCatalogue, CurationTier: skill.CurationTier, CuratedVersionIsThisOne: skill.CuratedVersionID == versionID}, true, nil
}

func runSkillFacts(skill registry.Skill) run.SkillFacts {
	restriction := skill.Restriction()
	return run.SkillFacts{AccessRestricted: restriction.InEffect(), AccessRestrictionReason: restriction.Reason()}
}

func NewTraceService(pool *pgxpool.Pool, signer *trace.Signer, runs *run.Service) *trace.Service {
	reads := readmodel.Reader{Pool: runs.Pool}
	return &trace.Service{
		Pool: pool, Signer: signer,
		ReadRunState: func(ctx context.Context, workspaceID, runID pgtype.UUID) (trace.RunState, bool, error) {
			state, found, err := reads.TraceRun(ctx, workspaceID, runID)
			return trace.RunState{
				Status: state.Status, StatusReason: state.StatusReason, Started: state.Started,
			}, found, err
		},
		ReadIngestRunState: func(ctx context.Context, runID pgtype.UUID) (trace.IngestRunState, bool, error) {
			state, found, err := reads.TraceIngestRun(ctx, runID)
			return trace.IngestRunState{
				ID: state.ID, WorkspaceID: state.WorkspaceID, Status: state.Status, FinishedAt: state.FinishedAt,
			}, found, err
		},
		ReadRunTransitions: func(ctx context.Context, workspaceID, runID pgtype.UUID) ([]trace.RunTransition, error) {
			rows, err := reads.TraceTransitions(ctx, workspaceID, runID)
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
	reads := readmodel.Reader{Pool: runs.Pool}
	service.ReadRunFacts = func(ctx context.Context, workspaceID, runID pgtype.UUID) (facts.Run, bool, error) {
		evaluated, found, err := reads.EvaluationRun(ctx, workspaceID, runID)
		return evalRunFacts(evaluated), found, err
	}
	service.ReadEvaluationInput = func(ctx context.Context, workspaceID, runID pgtype.UUID) (facts.EvaluationInput, bool, error) {
		input, found, err := reads.EvaluationInput(ctx, workspaceID, runID)
		artifacts := make([]facts.Artifact, len(input.Artifacts))
		for i, artifact := range input.Artifacts {
			artifacts[i] = facts.Artifact{
				FileName: artifact.FileName, ContentType: artifact.ContentType,
				SizeBytes: artifact.SizeBytes, ContentHash: artifact.ContentHash,
			}
		}
		return facts.EvaluationInput{
			Run: evalRunFacts(input.Run), Artifacts: artifacts, LatestAttempt: input.LatestAttempt,
			EarlierQuestions: input.EarlierQuestions,
			Absent: facts.ArtifactAbsence{
				Deleted: input.Absent.Deleted,
				Expired: input.Absent.Expired,
			},
		}, found, err
	}
}

func evalVersionFacts(version registry.Version) facts.Version {
	return facts.Version{ID: version.ID, SkillID: version.SkillID,
		PackageObjectKey: version.PackageObjectKey, SourcePath: version.SourcePath}
}

func WireEvaluationRegistryReaders(service *eval.Service, registryService *registry.Service) {
	service.ReadVersion = func(ctx context.Context, workspaceID, versionID pgtype.UUID) (facts.Version, bool, error) {
		version, found, err := registryService.WorkspaceVersion(ctx, workspaceID, versionID)
		return evalVersionFacts(version), found, err
	}
	service.ReadLatestVersion = func(ctx context.Context, workspaceID, skillID pgtype.UUID) (facts.Version, bool, error) {
		version, found, err := registryService.LatestVersion(ctx, workspaceID, skillID)
		return evalVersionFacts(version), found, err
	}
	service.ReadSkill = func(ctx context.Context, workspaceID, skillID pgtype.UUID) (facts.Skill, bool, error) {
		skill, found, err := registryService.WorkspaceSkill(ctx, workspaceID, skillID)
		return evalSkillFacts(skill), found, err
	}
	service.ReadRuntimeCompatibility = func(ctx context.Context, versionID pgtype.UUID) (facts.RuntimeCompatibility, bool, error) {
		compat, found, err := registryService.RuntimeCompatibility(ctx, versionID)
		return facts.RuntimeCompatibility{
			Capability: compat.Capability, Runtime: compat.Runtime, RuntimeImage: compat.RuntimeImage,
		}, found, err
	}
}

func evalSkillFacts(skill registry.Skill) facts.Skill {
	return facts.Skill{
		ID: skill.ID, Name: skill.Name, Summary: skill.Summary, AccessRestricted: skill.Restriction().InEffect(),
	}
}

func evalRunFacts(evaluated readmodel.EvaluationRun) facts.Run {
	return facts.Run{
		ID: evaluated.ID, WorkspaceID: evaluated.WorkspaceID,
		SkillVersionID: evaluated.SkillVersionID, TestCaseSnapshotID: evaluated.TestCaseSnapshotID,
		Status: evaluated.Status, StatusReason: evaluated.StatusReason, RuntimeSnapshot: evaluated.RuntimeSnapshot,
		StartedAt: evaluated.StartedAt, FinishedAt: evaluated.FinishedAt, FailureClass: evaluated.FailureClass,
		Terminal: evaluated.Terminal, Succeeded: evaluated.Succeeded,
	}
}

func NewRunEventConsumer(pool *pgxpool.Pool, evaluations *eval.Service, runs *run.Service) *eval.RunEventConsumer {
	reads := readmodel.Reader{Pool: runs.Pool}
	return &eval.RunEventConsumer{
		HasCurrentEvaluation: evaluations.HasCurrentEvaluation,
		Pool:                 pool,
		FinishedRunsAfter: func(ctx context.Context, after eval.FinishedRun, before time.Time, limit int32) ([]eval.FinishedRun, error) {
			rows, err := reads.EvaluableRunsFinishedAfter(ctx, readmodel.FinishedRun(after), before, limit)
			out := make([]eval.FinishedRun, len(rows))
			for i, row := range rows {
				out[i] = eval.FinishedRun(row)
			}
			return out, err
		},
	}
}
