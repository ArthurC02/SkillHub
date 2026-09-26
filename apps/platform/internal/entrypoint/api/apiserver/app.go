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

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/queue"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/envx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objreconcile"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/learning"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/delivery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/publishing"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/modelbudget"
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

	Providers     *run.Registry
	RunDeployment run.Deployment

	Quota policy.QuotaLimits

	GenerateQuota policy.QuotaLimits

	GenerateExposed   bool
	CreationExposed   bool
	CreationLimits    creation.Limits
	CreationTransient func(context.Context, creation.JobArgs, *creation.Diagram) error

	PublicationDownloadsOpen bool

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
	identitySvc := &identity.Service{Pool: cfg.Pool, OAuth: identity.ProviderOrNone(cfg.OAuth)}
	testlabSvc := &testlab.Service{
		Pool: cfg.Pool, MayStoreObjects: identitySvc.MayStoreObjects, ClearSightings: objreconcile.ClearDatasetSightings,
	}
	runPurgeSvc := &run.Service{Pool: cfg.Pool, ClearSightings: objreconcile.ClearArtifactSightings}
	packagingPurgeSvc := &packaging.Service{Pool: cfg.Pool, ClearSightings: objreconcile.ClearArtifactSightings}
	registryPurgeSvc := &registry.Service{
		Pool:                cfg.Pool,
		VersionsInRuns:      run.SkillVersionsInRuns,
		VersionsInDownloads: packaging.SkillVersionsInDownloads,
		VersionsInBundles:   publishing.SkillVersionsInBundles,
		SkillsWithTestCases: testlab.SkillsWithTestCases,
	}
	ingestPurgeSvc := &ingest.Service{Pool: cfg.Pool, SourcesInVersions: registry.SourcesInVersions}

	identitySvc.PurgeAnalytics = analytics.PurgeWorkspace
	identitySvc.PurgeTestData = testlabSvc.PurgeWorkspace
	identitySvc.PurgeRunArtifacts = runPurgeSvc.PurgeWorkspace
	identitySvc.PurgeDownloads = packagingPurgeSvc.PurgeWorkspace
	identitySvc.PurgeCreation = creation.PurgeWorkspace
	identitySvc.PurgePublications = publishing.PurgeWorkspace
	identitySvc.PurgeSkills = registryPurgeSvc.PurgeWorkspace
	identitySvc.PurgeImportSources = ingestPurgeSvc.PurgeWorkspace
	identitySvc.DatasetObjectKeys = testlab.WorkspaceObjectKeys
	identitySvc.RunArtifactObjectKeys = run.WorkspaceObjectKeys
	identitySvc.DownloadArtifactObjectKeys = packaging.WorkspaceObjectKeys
	identitySvc.WorkspaceQuiescent = run.PurgeQuiescent
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
		Fetcher:       ingest.FetcherOrNone(cfg.Fetcher),
		LLM:           ingest.ModelOrNone(cfg.LLM),
		GenerateQuota: cfg.GenerateQuota,
	}
	registrySvc := &registry.Service{
		Pool: cfg.Pool, Store: cfg.Store,
		CatalogWorkspaces: identitySvc.CatalogWorkspaceIDs,
	}

	versions.References = registrySvc
	testlabSvc.Store = cfg.Store
	testlabSvc.LLM = testlab.ModelOrNone(cfg.LLM)
	testlabSvc.ReadSkill = func(ctx context.Context, workspaceID, skillID pgtype.UUID) (testlab.SkillFacts, bool, error) {
		skill, found, err := registrySvc.WorkspaceSkill(ctx, workspaceID, skillID)
		return testlab.SkillFacts{Name: skill.Name, Summary: skill.Summary}, found, err
	}
	testlabSvc.LockLiveSkillForCreate = func(ctx context.Context, tx pgx.Tx, workspaceID, skillID pgtype.UUID) (bool, error) {
		_, found, err := registrySvc.LockLiveWorkspaceSkill(ctx, tx, workspaceID, skillID)
		return found, err
	}

	runSvc := &run.Service{
		Pool: cfg.Pool, TestLab: testlabSvc, Queue: wiring.NewRunQueue(jobs), Providers: cfg.Providers, Store: cfg.Store,
		ClearSightings:     objreconcile.ClearArtifactSightings,
		Quota:              cfg.Quota,
		WorkspaceCreatedAt: auth.Service.WorkspaceCreatedAt,
		LastOrphanScan:     wiring.LastOrphanScan(cfg.Pool),
		Deployment:         cfg.RunDeployment,
	}
	wiring.WireRunRegistryReaders(runSvc, registrySvc)
	funnel.RunBelongsToWorkspace = runSvc.BelongsToWorkspace
	traceSvc := wiring.NewTraceService(cfg.Pool, cfg.TraceSigner, runSvc)
	runSvc.Trace = traceSvc

	evalSvc := &eval.Service{
		Pool: cfg.Pool, TestLab: testlabSvc, Store: cfg.Store, Versions: versions, Trace: traceSvc,
	}
	wiring.WireEvaluationRunReaders(evalSvc, runSvc)
	wiring.WireEvaluationRegistryReaders(evalSvc, registrySvc)

	packagingSvc := &packaging.Service{
		Pool: cfg.Pool, TestLab: testlabSvc, Store: cfg.Store, Profiles: cfg.Profiles,
		ClearSightings:   objreconcile.ClearArtifactSightings,
		MayStoreObjects:  identitySvc.MayStoreObjects,
		ReadDisplayNames: identitySvc.DisplayNames,
		Retention:        policy.DownloadRetention(cfg.DownloadRetention),
		AppliedSuggestions: func(ctx context.Context, workspaceID, versionID pgtype.UUID) ([]packaging.AppliedSuggestion, error) {
			return packagingSuggestions(ctx, evalSvc, workspaceID, versionID)
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
	runSvc.ActiveArtifactReferences = packaging.ActiveArtifactReferences
	catalogSvc := wiring.NewCatalogService(cfg.Pool)
	catalogSvc.CatalogWorkspaces = identitySvc.CatalogWorkspaceIDs
	catalogSvc.LLM = catalog.ModelOrNone(cfg.LLM)
	catalogSvc.IntentAnalyzer = catalog.IntentAnalyzerOrNone(cfg.LLM)

	budgets := wiring.NewModelBudgets(cfg.Pool)
	versions.Budgets, testlabSvc.Budgets, catalogSvc.Budgets, evalSvc.Budgets = budgets, budgets, budgets, budgets
	catalogSvc.Store = cfg.Store
	catalogSvc.Analytics = funnel
	catalogSvc.SourceByID = func(ctx context.Context, workspaceID, sourceID pgtype.UUID) (catalog.SourceFacts, bool, error) {
		source, found, err := versions.ReadSource(ctx, workspaceID, sourceID)
		return catalog.SourceFacts{
			SourceType: source.SourceType, SourceURL: source.SourceURL, SourceRef: source.SourceRef,
			ContentHash: source.ContentHash, FetchedAt: source.FetchedAt,
			LastCheckedAt: source.LastCheckedAt, UnavailableSince: source.UnavailableSince,
			TaskDescription: source.TaskDescription, GeneratorModel: source.GeneratorModel,
			GeneratorPromptVersion: source.GeneratorPromptVersion,
			GenerationInputs:       source.GenerationInputs,
			PluginName:             source.PluginName,
			PluginVersion:          source.PluginVersion,
			PluginRepository:       source.PluginRepository,
		}, found, err
	}
	versions.IndexSkill = func(ctx context.Context, tx pgx.Tx, p ingest.SkillProjection) error {
		return catalogSvc.IndexSkillEnriched(ctx, tx, catalog.EnrichedSkillProjection{
			SkillID: p.SkillID, WorkspaceID: p.WorkspaceID, Name: p.Name, Summary: p.Summary,
			EnrichedSummary: p.EnrichedSummary, TaskExamples: p.TaskExamples, Tags: p.Tags,
			Limitations: p.Limitations, Scan: p.Scan, Embedding: p.Embedding,
			EnrichmentStatus: p.EnrichmentStatus, EnrichmentModel: p.EnrichmentModel,
			EnrichmentPromptVersion: p.EnrichmentPromptVersion,
		})
	}
	registrySvc.IndexSkill = func(ctx context.Context, tx pgx.Tx, p registry.SkillProjection) error {
		return catalogSvc.IndexSkill(ctx, tx, catalog.SkillProjection{
			SkillID: p.SkillID, WorkspaceID: p.WorkspaceID, Name: p.Name, Summary: p.Summary,
		})
	}
	registrySvc.RemoveFromIndex = catalog.RemoveSkillFromIndex
	registrySvc.RefreshListing = catalogSvc.RefreshListing
	wireCatalogRegistryReaders(catalogSvc, registrySvc)

	registrySvc.SkillRisks = catalogSvc.SkillRisks
	registrySvc.CatalogSkillRisks = catalogSvc.CatalogSkillRisks

	creationSvc := &creation.Service{Pool: cfg.Pool, Limits: cfg.CreationLimits}
	creationSvc.Insert = wiring.NewCreationQueue(jobs)
	wireCreationReads(creationSvc, versions, catalogSvc)
	wireCreationWrites(creationSvc, versions, runSvc, evalSvc)
	wireCreationTestCases(creationSvc, testlabSvc)
	wireCreationAdopt(creationSvc, registrySvc)

	creditSvc, err := wiring.NewCreditService(cfg.Pool)
	if err != nil {
		return nil, err
	}
	wiring.WireCreationCredit(creationSvc, creditSvc, cfg.Pool)

	wireCostRecording(creditSvc, catalogSvc, versions, testlabSvc, cfg.Pool)
	wireGenerateCredit(versions, creditSvc, identitySvc.WorkspaceOwner)
	wiring.WireCreditDisplay(creditSvc, runSvc, traceSvc, evalSvc)
	wiring.WireRunCredit(runSvc, creditSvc, cfg.Pool)

	publishingSvc := newPublishingService(cfg, registrySvc, packagingSvc)

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
			Publishing: &publishing.Handler{
				Svc: publishingSvc, Identity: auth.Service, DescribeRedistribution: describeRedistribution,
				DownloadsOpenToUninvited: cfg.PublicationDownloadsOpen,
				InviteRosterConfigured:   func() bool { return len(auth.Invited) > 0 },
			},
			Credits: &creditsHandler{
				Ledger:          &creditLedger{svc: creditSvc, owner: identitySvc.WorkspaceOwner, pool: cfg.Pool},
				Identity:        identitySvc,
				RunsInWorkspace: runSvc.RunsInWorkspace,
			},
			OperatorAudit: &operatorAuditHandler{DB: cfg.Pool},
			ModelBudgets:  &modelbudget.Handler{Svc: budgets, Actor: sessionActorID},
			Trends: &trendsHandler{
				Credits:            &creditLedger{svc: creditSvc, owner: identitySvc.WorkspaceOwner, pool: cfg.Pool},
				DailyRuns:          runSvc.DailyRuns,
				DailyRunWorkspaces: runSvc.DailyRunWorkspaces,
				DailyFunnelReach:   funnel.DailyFunnelReach,
				Audit:              cfg.Pool,
				Now:                time.Now,
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
	service.ReadSourceSiblings = func(
		ctx context.Context, workspaceID pgtype.UUID, packageObjectKey string, excludedSkillID pgtype.UUID,
	) ([]catalog.SourceSiblingFacts, error) {
		siblings, err := registryService.SkillsFromSameStoredPackage(ctx, workspaceID, packageObjectKey, excludedSkillID)
		if err != nil {
			return nil, err
		}
		out := make([]catalog.SourceSiblingFacts, 0, len(siblings))
		for _, s := range siblings {
			out = append(out, catalog.SourceSiblingFacts{
				SkillID: s.SkillID, Name: s.Name, SourcePath: s.SourcePath,
			})
		}
		return out, nil
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
		PackageObjectKey: version.PackageObjectKey, SourcePath: version.SourcePath,
		LicenseExpression: version.LicenseExpression,
		CreatedAt:         version.CreatedAt, LicenseSource: version.LicenseSource,
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
		return packagingSkillFacts(skill), found, err
	}
	versionFacts := func(version registry.Version) packaging.VersionFacts {
		return packaging.VersionFacts{
			ID: version.ID, SkillID: version.SkillID, SourceID: version.SourceID,
			VersionNumber: version.VersionNumber, ContentHash: version.ContentHash,
			PackageObjectKey: version.PackageObjectKey, SourcePath: version.SourcePath,
			LicenseExpression: version.LicenseExpression,
			LicenseSource:     version.LicenseSource, CreatedAt: version.CreatedAt,
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
	service.ReadVersionSummaries = func(
		ctx context.Context, versionIDs []pgtype.UUID,
	) (map[pgtype.UUID]packaging.VersionSummary, error) {
		summaries, err := registryService.VersionSummariesByID(ctx, versionIDs)
		if err != nil {
			return nil, err
		}
		out := make(map[pgtype.UUID]packaging.VersionSummary, len(summaries))
		for id, summary := range summaries {
			out[id] = packagingVersionSummary(summary)
		}
		return out, nil
	}
	service.ReadOldest = func(ctx context.Context, skillID pgtype.UUID) (packaging.OldestVersion, bool, error) {
		version, found, err := registryService.OldestVersion(ctx, skillID)
		return packaging.OldestVersion{SourceID: version.SourceID}, found, err
	}
}

func packagingSkillFacts(skill registry.Skill) packaging.SkillFacts {
	return packaging.SkillFacts{
		ID: skill.ID, Name: skill.Name, ForkedFromSkillID: skill.ForkedFromSkillID,
		ForkedFromVersionID: skill.ForkedFromVersionID,
		AccessRestricted:    skill.Restriction().InEffect(), Redistribution: skill.Redistribution,
	}
}

func packagingVersionSummary(summary registry.VersionSummary) packaging.VersionSummary {
	return packaging.VersionSummary{
		SkillID: summary.SkillID, SkillName: summary.SkillName, VersionNumber: summary.VersionNumber,
		LatestVersionNumber: summary.LatestVersionNumber,
		AccessRestricted:    summary.Restriction().InEffect(), Redistribution: summary.Redistribution,
	}
}

func packagingSuggestions(ctx context.Context, svc *eval.Service, workspaceID, versionID pgtype.UUID) ([]packaging.AppliedSuggestion, error) {
	rows, err := svc.AppliedSuggestions(ctx, workspaceID, versionID)
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
