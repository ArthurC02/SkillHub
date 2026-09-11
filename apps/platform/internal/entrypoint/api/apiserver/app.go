package apiserver

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/queue"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/envx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/learning"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/delivery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

type ObjectStore interface {
	ingest.ObjectStore
	packaging.ObjectStore
	run.ObjectStore
	testlab.ObjectStore
}

type Config struct {
	Pool  *pgxpool.Pool
	Store ObjectStore

	LLM     *llmclient.Client
	Fetcher *ingest.URLFetcher

	TraceSigner *trace.Signer

	Profiles packaging.Profiles

	DownloadRetention  time.Duration
	AnalyticsRetention time.Duration

	Readiness *envx.Registry
	OAuth     *identity.GitHubOAuth
	Secure    bool
	AppURL    string
	DevLogin  bool

	Operators map[string]bool
	Invited   map[string]bool

	Providers *run.Registry

	Quota policy.QuotaLimits

	GenerateQuota policy.QuotaLimits

	GenerateExposed   bool
	CreationExposed   bool
	CreationLimits    creation.Limits
	CreationTransient func(context.Context, creation.JobArgs, *llmclient.GenerateDiagram) error

	RateLimits *httpx.RateLimiter

	FeedbackRetention time.Duration

	CleanMode bool
}

type App struct {
	Deps Deps

	Auth         *identity.Handler
	RunSvc       *run.Service
	EvalSvc      *eval.Service
	PackagingSvc *packaging.Service
	Versions     *ingest.Service
	CreationSvc  *creation.Service
	TraceSvc     *trace.Service
}

func NewApp(cfg Config) (*App, error) {
	analyticsPurgeSvc := &analytics.Service{Pool: cfg.Pool}
	identitySvc := &identity.Service{Pool: cfg.Pool, OAuth: cfg.OAuth}
	testlabSvc := &testlab.Service{Pool: cfg.Pool, MayStoreObjects: identitySvc.MayStoreObjects}
	runPurgeSvc := &run.Service{Pool: cfg.Pool}
	packagingPurgeSvc := &packaging.Service{Pool: cfg.Pool}
	registryPurgeSvc := &registry.Service{Pool: cfg.Pool}
	ingestPurgeSvc := &ingest.Service{Pool: cfg.Pool}
	creationPurgeSvc := &creation.Service{Pool: cfg.Pool}

	identitySvc.PurgeAnalytics = analyticsPurgeSvc.PurgeWorkspace
	identitySvc.PurgeTestData = testlabSvc.PurgeWorkspace
	identitySvc.PurgeRunArtifacts = runPurgeSvc.PurgeWorkspace
	identitySvc.PurgeDownloads = packagingPurgeSvc.PurgeWorkspace
	identitySvc.PurgeCreation = creationPurgeSvc.PurgeWorkspace
	identitySvc.PurgeSkills = registryPurgeSvc.PurgeWorkspace
	identitySvc.PurgeImportSources = ingestPurgeSvc.PurgeWorkspace
	identitySvc.DatasetObjectKeys = testlabSvc.WorkspaceObjectKeys
	identitySvc.RunArtifactObjectKeys = runPurgeSvc.WorkspaceObjectKeys
	identitySvc.DownloadArtifactObjectKeys = packagingPurgeSvc.WorkspaceObjectKeys
	identitySvc.WorkspaceQuiescent = runPurgeSvc.PurgeQuiescent
	auth := &identity.Handler{
		Service:     identitySvc,
		Secure:      cfg.Secure,
		AppURL:      cfg.AppURL,
		DevLogin:    cfg.DevLogin,
		Operators:   cfg.Operators,
		Invited:     cfg.Invited,
		Features:    entryPointFeatures(cfg),
		Disclosures: disclosureFeatures(cfg),
	}

	jobs, err := queue.New(cfg.Pool, nil)
	if err != nil {
		return nil, err
	}

	funnel := &analytics.Service{
		Pool:      cfg.Pool,
		Retention: cfg.AnalyticsRetention,
		Secure:    cfg.Secure,
	}

	versions := &ingest.Service{
		Pool:          cfg.Pool,
		Store:         cfg.Store,
		Fetcher:       cfg.Fetcher,
		LLM:           cfg.LLM,
		GenerateQuota: cfg.GenerateQuota,
		IndexSkill: func(ctx context.Context, tx pgx.Tx, p ingest.SkillProjection) error {
			return catalog.IndexSkillEnriched(ctx, tx, catalog.EnrichedSkillProjection{
				SkillID: p.SkillID, WorkspaceID: p.WorkspaceID, Name: p.Name, Summary: p.Summary,
				EnrichedSummary: p.EnrichedSummary, TaskExamples: p.TaskExamples, Tags: p.Tags,
				Limitations: p.Limitations, Scan: p.Scan, Embedding: p.Embedding,
				EnrichmentStatus: p.EnrichmentStatus, EnrichmentModel: p.EnrichmentModel,
				EnrichmentPromptVersion: p.EnrichmentPromptVersion,
			})
		},
	}
	registrySvc := &registry.Service{
		Pool: cfg.Pool, Store: cfg.Store,
		IndexSkill: func(ctx context.Context, tx pgx.Tx, p registry.SkillProjection) error {
			return catalog.IndexSkill(ctx, tx, catalog.SkillProjection{
				SkillID: p.SkillID, WorkspaceID: p.WorkspaceID, Name: p.Name, Summary: p.Summary,
			})
		},
		RemoveFromIndex: catalog.RemoveSkillFromIndex,
	}

	versions.References = registrySvc
	testlabSvc.Store = cfg.Store
	testlabSvc.LLM = suggesterOrNil(cfg.LLM)
	testlabSvc.ReadSkill = func(ctx context.Context, workspaceID, skillID pgtype.UUID) (testlab.SkillFacts, bool, error) {
		skill, found, err := registrySvc.WorkspaceSkill(ctx, workspaceID, skillID)
		return testlab.SkillFacts{Name: skill.Name, Summary: skill.Summary}, found, err
	}

	runSvc := &run.Service{
		Pool: cfg.Pool, TestLab: testlabSvc, Queue: jobs, Providers: cfg.Providers, Store: cfg.Store,
		Quota:              cfg.Quota,
		WorkspaceCreatedAt: auth.Service.WorkspaceCreatedAt,
		ReadSkill: func(ctx context.Context, workspaceID, skillID pgtype.UUID) (run.SkillFacts, bool, error) {
			skill, found, err := registrySvc.WorkspaceSkill(ctx, workspaceID, skillID)
			return run.SkillFacts{AccessRestriction: skill.AccessRestriction}, found, err
		},
		ReadVersion: func(ctx context.Context, workspaceID, versionID pgtype.UUID) (run.VersionFacts, bool, error) {
			version, found, err := registrySvc.WorkspaceVersion(ctx, workspaceID, versionID)
			return run.VersionFacts{
				ID: version.ID, SkillID: version.SkillID, ContentHash: version.ContentHash,
				PackageObjectKey: version.PackageObjectKey,
			}, found, err
		},
		ReadContentSource: readContentSource(registrySvc),
	}
	funnel.RunBelongsToWorkspace = runSvc.BelongsToWorkspace
	traceSvc := newTraceService(cfg.Pool, cfg.TraceSigner, runSvc)
	runSvc.Trace = traceSvc

	evalSvc := &eval.Service{
		Pool: cfg.Pool, TestLab: testlabSvc, Store: cfg.Store, Versions: versions, Trace: traceSvc,
	}
	wireEvaluationRunReaders(evalSvc, runSvc)
	wireEvaluationRegistryReaders(evalSvc, registrySvc)

	packagingSvc := &packaging.Service{
		Pool: cfg.Pool, TestLab: testlabSvc, Store: cfg.Store, Profiles: cfg.Profiles,
		MayStoreObjects: identitySvc.MayStoreObjects,
		Retention:       policy.DownloadRetention(cfg.DownloadRetention),
		AppliedSuggestions: func(ctx context.Context, versionID, workspaceID pgtype.UUID) ([]packaging.AppliedSuggestion, error) {
			return packagingSuggestions(ctx, evalSvc, versionID, workspaceID)
		},
		SourceLineage: func(ctx context.Context, sourceID pgtype.UUID) (packaging.LineageSource, error) {
			source, err := versions.SourceLineage(ctx, sourceID)
			return packaging.LineageSource{
				SourceType: source.SourceType, SourceURL: source.SourceURL, SourceRef: source.SourceRef,
				ContentHash: source.ContentHash, FetchedAt: source.FetchedAt,
			}, err
		},
	}
	wirePackagingRegistryReaders(packagingSvc, registrySvc)
	runSvc.ActiveArtifactReferences = packagingSvc.ActiveArtifactReferences
	catalogSvc := &catalog.Service{
		Pool: cfg.Pool, LLM: cfg.LLM, Store: cfg.Store, Analytics: funnel,
		SourceByID: func(ctx context.Context, workspaceID, sourceID pgtype.UUID) (catalog.SourceFacts, bool, error) {
			source, found, err := versions.ReadSource(ctx, workspaceID, sourceID)
			return catalog.SourceFacts{
				SourceType: source.SourceType, SourceURL: source.SourceURL, SourceRef: source.SourceRef,
				ContentHash: source.ContentHash, FetchedAt: source.FetchedAt,
				LastCheckedAt: source.LastCheckedAt, UnavailableSince: source.UnavailableSince,
				TaskDescription: source.TaskDescription, GeneratorModel: source.GeneratorModel,
				GeneratorPromptVersion: source.GeneratorPromptVersion,
				GenerationInputs:       source.GenerationInputs,
			}, found, err
		},
	}
	wireCatalogRegistryReaders(catalogSvc, registrySvc)

	registrySvc.SkillRisks = catalogSvc.SkillRisks
	registrySvc.CatalogSkillRisks = catalogSvc.CatalogSkillRisks

	creationSvc := &creation.Service{Pool: cfg.Pool, Limits: cfg.CreationLimits}
	creationSvc.Insert = func(ctx context.Context, tx pgx.Tx, a creation.JobArgs) error {
		_, err := jobs.InsertTx(ctx, tx, a, &river.InsertOpts{MaxAttempts: 1})
		return err
	}
	wireCreationReads(creationSvc, versions, catalogSvc)
	wireCreationWrites(creationSvc, versions, runSvc, evalSvc)
	wireCreationTestCases(creationSvc, testlabSvc)
	wireCreationAdopt(creationSvc, registrySvc)

	creditSvc, err := newCreditService(cfg.Pool, identitySvc)
	if err != nil {
		return nil, err
	}
	wireCreationCredit(creationSvc, creditSvc, identitySvc)

	wireCostRecording(creditSvc, catalogSvc, versions)
	wireGenerateCredit(versions, creditSvc, identitySvc.WorkspaceOwner)
	wireCreditDisplay(creditSvc, runSvc, traceSvc, evalSvc)
	wireRunCredit(runSvc, creditSvc, cfg.Pool)

	identitySvc.PurgeCredit = func(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {

		userID, err := identitySvc.WorkspaceOwnerIn(ctx, tx, workspaceID)
		if err != nil {
			return err
		}
		return creditSvc.PurgeUser(ctx, tx, userID)
	}
	return &App{
		Deps: Deps{
			Auth:            auth,
			Creation:        &creationHandler{Svc: creationSvc, Identity: identitySvc, Transient: cfg.CreationTransient, Credit: creditSvc},
			CreationExposed: creationEnabled(cfg),
			Readiness:       cfg.Readiness,
			CleanMode:       cfg.CleanMode,
			Importer:        &ingest.Handler{Svc: versions, Identity: auth.Service},
			Search: &catalog.Handler{
				Svc:      catalogSvc,
				Identity: auth.Service,
			},
			Registry: &registry.Handler{

				Svc:      registrySvc,
				Identity: auth.Service,
			},
			TestLab: &testlab.Handler{
				Svc:      testlabSvc,
				Identity: auth.Service,
			},
			Runs:      &run.Handler{Svc: runSvc, Identity: auth.Service, RunVerdicts: evalSvc.RunVerdicts},
			Trace:     &trace.Handler{Svc: traceSvc, Identity: auth.Service},
			Eval:      &eval.Handler{Svc: evalSvc, Identity: auth.Service},
			Packaging: &packaging.Handler{Svc: packagingSvc, Identity: auth.Service},
			Credits: &creditsHandler{
				Ledger:   &creditLedger{svc: creditSvc, owner: identitySvc.WorkspaceOwner, pool: cfg.Pool},
				Identity: identitySvc,
			},
			Analytics: &analytics.Handler{
				Svc: funnel, Identity: auth.Service, FeedbackRetention: cfg.FeedbackRetention,
			},
			GenerateExposed: cfg.GenerateExposed,
			Limits:          cfg.RateLimits,
			AppURL:          cfg.AppURL,
		},
		Auth:         auth,
		RunSvc:       runSvc,
		EvalSvc:      evalSvc,
		PackagingSvc: packagingSvc,
		Versions:     versions,
		CreationSvc:  creationSvc,
		TraceSvc:     traceSvc,
	}, nil
}

func newTraceService(pool *pgxpool.Pool, signer *trace.Signer, runs *run.Service) *trace.Service {
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

func wireEvaluationRunReaders(service *eval.Service, runs *run.Service) {
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

func wireEvaluationRegistryReaders(service *eval.Service, registryService *registry.Service) {
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

func wireCatalogRegistryReaders(service *catalog.Service, registryService *registry.Service) {
	service.ReadCatalogSkill = func(ctx context.Context, skillID pgtype.UUID) (catalog.SkillFacts, bool, error) {
		skill, found, err := registryService.CatalogSkill(ctx, skillID)
		return catalogSkillFacts(skill), found, err
	}
	service.ReadWorkspaceSkill = func(ctx context.Context, workspaceID, skillID pgtype.UUID) (catalog.SkillFacts, bool, error) {
		skill, found, err := registryService.WorkspaceSkill(ctx, workspaceID, skillID)
		return catalogSkillFacts(skill), found, err
	}
	service.ReadLatestVersion = func(ctx context.Context, workspaceID, skillID pgtype.UUID) (catalog.VersionFacts, bool, error) {
		version, found, err := registryService.LatestVersion(ctx, workspaceID, skillID)
		return catalogVersionFacts(version), found, err
	}
	service.ReadRuntimeCompatibility = func(ctx context.Context, versionID pgtype.UUID) (catalog.RuntimeCompatibilityFacts, bool, error) {
		compat, found, err := registryService.RuntimeCompatibility(ctx, versionID)
		return catalogRuntimeCompatibilityFacts(compat), found, err
	}
}

func catalogSkillFacts(skill registry.Skill) catalog.SkillFacts {
	return catalog.SkillFacts{
		ID: skill.ID, WorkspaceID: skill.WorkspaceID, Name: skill.Name, Summary: skill.Summary,
		ForkedFromSkillID: skill.ForkedFromSkillID, ForkedFromVersionID: skill.ForkedFromVersionID,
		TakedownAt: skill.TakedownAt, AccessRestriction: skill.AccessRestriction,
		Redistribution: skill.Redistribution, CurationTier: skill.CurationTier,
		CuratedVersionID: skill.CuratedVersionID, Category: skill.Category,
		CategorySource: skill.CategorySource,
	}
}

func catalogVersionFacts(version registry.Version) catalog.VersionFacts {
	return catalog.VersionFacts{
		ID: version.ID, WorkspaceID: version.WorkspaceID, SourceID: version.SourceID,
		VersionNumber: version.VersionNumber, ContentHash: version.ContentHash,
		PackageObjectKey: version.PackageObjectKey, LicenseExpression: version.LicenseExpression,
		CreatedAt: version.CreatedAt, LicenseSource: version.LicenseSource,
	}
}

func catalogRuntimeCompatibilityFacts(compat registry.RuntimeCompatibility) catalog.RuntimeCompatibilityFacts {
	return catalog.RuntimeCompatibilityFacts{
		Capability: compat.Capability, Runtime: compat.Runtime,
		RuntimeImage: compat.RuntimeImage, MeasuredAt: compat.MeasuredAt,
	}
}

func wirePackagingRegistryReaders(service *packaging.Service, registryService *registry.Service) {
	service.ReadSkill = func(ctx context.Context, workspaceID, skillID pgtype.UUID) (packaging.SkillFacts, bool, error) {
		skill, found, err := registryService.WorkspaceSkill(ctx, workspaceID, skillID)
		return packaging.SkillFacts{
			ID: skill.ID, Name: skill.Name, ForkedFromSkillID: skill.ForkedFromSkillID,
			ForkedFromVersionID: skill.ForkedFromVersionID,
			AccessRestriction:   skill.AccessRestriction, Redistribution: skill.Redistribution,
		}, found, err
	}
	versionFacts := func(version registry.Version) packaging.VersionFacts {
		return packaging.VersionFacts{
			ID: version.ID, SkillID: version.SkillID, SourceID: version.SourceID,
			VersionNumber: version.VersionNumber, ContentHash: version.ContentHash,
			PackageObjectKey: version.PackageObjectKey, LicenseExpression: version.LicenseExpression,
			LicenseSource: version.LicenseSource, CreatedAt: version.CreatedAt,
		}
	}
	service.ReadVersion = func(ctx context.Context, workspaceID, versionID pgtype.UUID) (packaging.VersionFacts, bool, error) {
		version, found, err := registryService.WorkspaceVersion(ctx, workspaceID, versionID)
		return versionFacts(version), found, err
	}
	service.ReadCompatibility = func(ctx context.Context, versionID pgtype.UUID) (packaging.RuntimeCompatibility, bool, error) {
		compat, found, err := registryService.RuntimeCompatibility(ctx, versionID)
		return packaging.RuntimeCompatibility{
			Capability: compat.Capability, Runtime: compat.Runtime,
			RuntimeImage: compat.RuntimeImage, MeasuredAt: compat.MeasuredAt,
		}, found, err
	}
	service.ReadPrevious = func(ctx context.Context, workspaceID, skillID pgtype.UUID, versionNumber int32) (packaging.PreviousVersion, bool, error) {
		version, found, err := registryService.PreviousVersion(ctx, workspaceID, skillID, versionNumber)
		return packaging.PreviousVersion{ID: version.ID, SkillID: version.SkillID, VersionNumber: version.VersionNumber}, found, err
	}
	service.ReadLineage = func(ctx context.Context, versionID pgtype.UUID) (packaging.LineageStep, bool, error) {
		step, found, err := registryService.VersionLineage(ctx, versionID)
		return packaging.LineageStep{
			ID: step.ID, SkillID: step.SkillID, VersionNumber: step.VersionNumber,
			ForkedFromVersionID: step.ForkedFromVersionID,
		}, found, err
	}

	service.CuratedSource = func(ctx context.Context, skillID pgtype.UUID) (packaging.CuratedSource, bool, error) {
		skill, found, err := registryService.CatalogSkill(ctx, skillID)
		return packaging.CuratedSource{SkillID: skill.ID, WorkspaceID: skill.WorkspaceID}, found, err
	}
	service.ReadOldest = func(ctx context.Context, skillID pgtype.UUID) (packaging.OldestVersion, bool, error) {
		version, found, err := registryService.OldestVersion(ctx, skillID)
		return packaging.OldestVersion{SourceID: version.SourceID}, found, err
	}
}

func evalRunFacts(facts run.EvaluationRun) eval.RunFacts {
	return eval.RunFacts{
		ID: facts.ID, WorkspaceID: facts.WorkspaceID,
		SkillVersionID: facts.SkillVersionID, TestCaseSnapshotID: facts.TestCaseSnapshotID,
		Status: facts.Status, StatusReason: facts.StatusReason, RuntimeSnapshot: facts.RuntimeSnapshot,
		StartedAt: facts.StartedAt, FinishedAt: facts.FinishedAt, FailureClass: facts.FailureClass,
	}
}

func packagingSuggestions(ctx context.Context, svc *eval.Service, versionID, workspaceID pgtype.UUID) ([]packaging.AppliedSuggestion, error) {
	rows, err := svc.AppliedSuggestions(ctx, versionID, workspaceID)
	if err != nil {
		return nil, err
	}
	result := make([]packaging.AppliedSuggestion, len(rows))
	for i, row := range rows {
		result[i] = packaging.AppliedSuggestion{
			EvaluationID: row.EvaluationID,
			Category:     row.Category,
			TargetPath:   row.TargetPath,
		}
	}
	return result, nil
}

func (a *App) Handler() http.Handler { return NewRouter(a.Deps) }

func (a *App) AuditRosters(ctx context.Context) {
	if err := a.Auth.LogOperatorRoster(ctx); err != nil {
		slog.Error("operator roster not audited; no operator will be recognised", "error", err)
		a.Auth.Operators = nil
	}
	if err := a.Auth.LogInviteRoster(ctx); err != nil {
		slog.Error("beta roster not audited; the closed beta gate admits nobody", "error", err)
		a.Auth.Invited = BetaGateClosed()
	}
	if err := a.logFeatureFlags(ctx); err != nil {

		slog.Error("feature flags not audited; a mis-opened flag will leave no trace", "error", err)
	}
}

func (a *App) logFeatureFlags(ctx context.Context) error {

	on := make([]string, 0, len(a.Auth.Features)+len(a.Auth.Disclosures))
	for _, m := range []map[string]bool{a.Auth.Features, a.Auth.Disclosures} {
		for name, enabled := range m {
			if enabled {
				on = append(on, name)
			}
		}
	}
	sort.Strings(on)
	return audit.Log(ctx, a.Auth.Service.Pool, audit.Event{
		Action:       audit.ActionFeatureFlags,
		ResourceType: audit.ResourceFeatureFlags,
		Metadata: map[string]any{
			"enabled": on,
			"count":   len(on),
			"source":  "environment",
		},
	})
}

func BetaGateClosed() map[string]bool {
	return map[string]bool{"\x00 roster was not recorded": true}
}

func suggesterOrNil(c *llmclient.Client) testlab.CriteriaSuggester {
	if c == nil {
		return nil
	}
	return c
}

func creationEnabled(cfg Config) bool {
	return cfg.GenerateExposed && cfg.CreationExposed && cfg.CreationLimits.Valid() && cfg.CreationTransient != nil
}

func entryPointFeatures(cfg Config) map[string]bool {
	f := map[string]bool{}
	if cfg.GenerateExposed {
		f["generate_skill"] = true
		if creationEnabled(cfg) {
			f["creation_skill"] = true
		}
	}
	return f
}

func disclosureFeatures(cfg Config) map[string]bool {
	f := map[string]bool{}
	if cfg.CleanMode {
		f["clean_mode"] = true
	}
	return f
}
