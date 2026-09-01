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

// ObjectStore is the whole object-storage surface the API's contexts need,
// assembled from the slices each of them declares. Embedding rather than
// re-listing the methods: the union then cannot drift from the parts, and a
// context that widens its own slice widens this one with it.
//
// *objstore.Client satisfies it, and so does the integration tests' in-memory
// store — which is the point: one Store field on Config, not seven.
type ObjectStore interface {
	ingest.ObjectStore
	packaging.ObjectStore
	run.ObjectStore
	testlab.ObjectStore
}

// Config is everything the API needs from outside itself. Its fields are
// deployment inputs, not wiring: cmd/api reads them from the environment and the
// integration tests set them directly, and neither builds a domain Service of
// its own (ADR-032 §5).
type Config struct {
	Pool  *pgxpool.Pool
	Store ObjectStore
	// LLM is the internal Python service (ADR-016). Nil is a working deployment:
	// search degrades to FTS-only, imports land with enrichment pending, and
	// TEST-002's suggestions report themselves unavailable.
	LLM     *llmclient.Client
	Fetcher *ingest.URLFetcher
	// TraceSigner mints the TRACE-002 ingestion credential. One with no secret
	// means no ingestion URL is minted and no events are collected.
	TraceSigner *trace.Signer
	// Profiles is the PACK-002 target configuration. Empty means "no targets" and
	// never "use defaults" — the packaging routes answer 503.
	Profiles packaging.Profiles
	// DownloadRetention and AnalyticsRetention are both "zero = do not collect",
	// because PDM-006 has not ratified either period (ADR-027, ADR-029 決策 5).
	DownloadRetention  time.Duration
	AnalyticsRetention time.Duration

	// Readiness is the deployment capability table (05 R-36 第二段), built by the
	// entrypoint because that is where the probes' dependencies live. Nil is a
	// working deployment whose /readyz says it measured nothing, which is the one
	// answer that must never be rounded up to "ready" (04 丙-118).
	Readiness *envx.Registry
	OAuth     *identity.GitHubOAuth
	Secure    bool
	AppURL    string
	DevLogin  bool
	// Operators is the 02:SEC-011 roster; Invited is the BETA-001 admission list
	// (ADR-028 決策 1). Empty Operators grants nothing; empty Invited admits
	// everybody, which is why AuditRosters fails them closed in opposite
	// directions.
	Operators map[string]bool
	Invited   map[string]bool

	// Providers is the sandbox fleet the API checks a run against before queueing
	// (RUN-005). It never dispatches — that is the worker's job (iron rule 7) — so
	// nil is fine and means every run is accepted and then fails saying so.
	Providers *run.Registry
	// Quota is the PDM-010 free allowance. The zero value is "not enforced", and
	// it also unmounts GET /me/quota (ADR-028 決策 3).
	Quota policy.QuotaLimits
	// GenerateQuota is the M5 generation allowance, deliberately a second value
	// rather than a second draw on Quota (ADR-047 決策 5).
	GenerateQuota policy.QuotaLimits
	// GenerateExposed is ADR-052's hard boundary between 開工 and 曝光. False
	// means POST /skills/generate is not mounted and /me does not mention it, so
	// a beta participant cannot meet "搜不到 → 生成一個" — the funnel's first
	// segment measures whether search works, and that number has one chance with
	// twelve people.
	GenerateExposed bool
	// RateLimits is NFR-001 clause 5's limiter for anonymous search and the two
	// import endpoints. Nil = no limiting; cmd/api sets it unless RATE_LIMIT=off
	// (unset = enforced with defaults, the RUN_QUOTA convention — a protection
	// left unconfigured must not silently be absent).
	RateLimits *httpx.RateLimiter
	// FeedbackRetention is how long BETA-003/004/005's free-text reports live,
	// for GET /policy/data-retention to disclose (FEEDBACK_RETENTION; the same
	// value `maintenance purge-feedback` refuses to run without). Zero means no
	// window has been set, which the endpoint says out loud rather than omitting.
	FeedbackRetention time.Duration
	// CleanMode is ADR-060 決策 6's deployment declaration, injected rather than
	// read here. features() used to call os.Getenv("SKILLHUB_CLEAN_MODE") itself,
	// which made this Config blind to the one axis that decides whether the user
	// is shown 02:PORT-003's disclosure at all — a deployment input like every
	// other field of this struct (GenerateExposed is the exact shape to copy).
	// cmd/api reads the variable once and hands the bool down; a test sets the
	// field instead of the process environment.
	CleanMode bool
}

// App is the wired object graph. Deps is exposed so a test can adjust the
// deployment knobs that change which routes exist before Handler builds the
// table; the service handles beside it are exposed so a test can drive a domain
// service directly (produce a verdict with a fake judge, point the run service
// at a fake provider) without re-assembling any of this.
type App struct {
	Deps Deps

	Auth         *identity.Handler
	RunSvc       *run.Service
	EvalSvc      *eval.Service
	PackagingSvc *packaging.Service
	Versions     *ingest.Service
	TraceSvc     *trace.Service
}

// NewApp is the API's composition root: the one place this process wires its
// contexts together (ADR-032 §5). cmd/api and the integration tests both call
// it, so a dependency missing here is missing in both — which is the point, and
// why app_test.go checks the graph without a database.
//
// It is the API's root, not the platform's. cmd/worker, cmd/maintenance and
// cmd/reindex build the services their own deployment unit runs, and none of
// those objects is this one: the worker's run.Service holds a model gateway and
// a working queue client, this one holds an insert-only client and no gateway,
// because the API enqueues and never dispatches (iron rule 7).
func NewApp(cfg Config) (*App, error) {
	analyticsPurgeSvc := &analytics.Service{Pool: cfg.Pool}
	testlabSvc := &testlab.Service{Pool: cfg.Pool}
	runPurgeSvc := &run.Service{Pool: cfg.Pool}
	packagingPurgeSvc := &packaging.Service{Pool: cfg.Pool}
	registryPurgeSvc := &registry.Service{Pool: cfg.Pool}
	ingestPurgeSvc := &ingest.Service{Pool: cfg.Pool}
	auth := &identity.Handler{
		Service: &identity.Service{
			Pool:  cfg.Pool,
			OAuth: cfg.OAuth,
			// CORE-007's account purge clears six other contexts' rows in one
			// transaction. Each step is the owning context's own — every one of
			// them imports identity for its workspace scope, so identity can
			// import none of them and only a composition root sees all six
			// (ADR-034). Unwired, the purge refuses the batch rather than
			// silently leaving a context behind; the order is identity's, in
			// purge.go.
			PurgeAnalytics:     analyticsPurgeSvc.PurgeWorkspace,
			PurgeTestData:      testlabSvc.PurgeWorkspace,
			PurgeRunArtifacts:  runPurgeSvc.PurgeWorkspace,
			PurgeDownloads:     packagingPurgeSvc.PurgeWorkspace,
			PurgeSkills:        registryPurgeSvc.PurgeWorkspace,
			PurgeImportSources: ingestPurgeSvc.PurgeWorkspace,
			// Not purge steps: all three run before the transaction, because
			// object bytes are deleted before their rows (DDD-033).
			DatasetObjectKeys:          testlabSvc.WorkspaceObjectKeys,
			RunArtifactObjectKeys:      runPurgeSvc.WorkspaceObjectKeys,
			DownloadArtifactObjectKeys: packagingPurgeSvc.WorkspaceObjectKeys,
		},
		Secure:      cfg.Secure,
		AppURL:      cfg.AppURL,
		DevLogin:    cfg.DevLogin,
		Operators:   cfg.Operators,
		Invited:     cfg.Invited,
		Features:    entryPointFeatures(cfg),
		Disclosures: disclosureFeatures(cfg),
	}

	// Insert-only queue client: the API enqueues run jobs in the same transaction
	// as the run row, and never works one (iron rule 7). Schema migration belongs
	// to cmd/worker, so an API rollout does not touch the queue's tables.
	jobs, err := queue.New(cfg.Pool, nil)
	if err != nil {
		return nil, err
	}

	funnel := &analytics.Service{
		Pool:      cfg.Pool,
		Retention: cfg.AnalyticsRetention,
		Secure:    cfg.Secure,
	}

	// IndexSkill is catalog's write on catalog's table, handed to ingest here
	// because neither may import the other's package for it (ADR-034). It runs
	// inside the import transaction, which is what INGEST-009 asks for: a version
	// is searchable the moment it exists. Left unset, every import path refuses
	// rather than committing a version nobody can find — see app_test.go.
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
	testlabSvc.Store = cfg.Store
	testlabSvc.LLM = suggesterOrNil(cfg.LLM)
	testlabSvc.ReadSkill = func(ctx context.Context, workspaceID, skillID pgtype.UUID) (testlab.SkillFacts, bool, error) {
		skill, found, err := registrySvc.WorkspaceSkill(ctx, workspaceID, skillID)
		return testlab.SkillFacts{Name: skill.Name, Summary: skill.Summary}, found, err
	}

	// Store is read-only here: the pre-run permission summary scans the stored
	// package to answer "does this carry a script" (02:TEST-005).
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

	// No judge and no suggester: producing a verdict and asking for advice are the
	// worker's jobs (iron rule 7). The store and the version writer are wired,
	// because EVAL-002's diff and apply are user actions and both need package
	// bytes — and applying goes through the ordinary version-creation path rather
	// than around it.
	evalSvc := &eval.Service{
		Pool: cfg.Pool, TestLab: testlabSvc, Store: cfg.Store, Versions: versions, Trace: traceSvc,
	}
	wireEvaluationRunReaders(evalSvc, runSvc)
	wireEvaluationRegistryReaders(evalSvc, registrySvc)

	// Packaging runs in the control plane and needs no sandbox: it reads stored
	// bytes, filters them and writes a zip, and executes nothing (iron rules 1
	// and 2).
	packagingSvc := &packaging.Service{
		Pool: cfg.Pool, TestLab: testlabSvc, Store: cfg.Store, Profiles: cfg.Profiles,
		Retention: policy.DownloadRetention(cfg.DownloadRetention),
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
		Pool: cfg.Pool, Registry: registrySvc, LLM: cfg.LLM, Store: cfg.Store, Analytics: funnel,
		SourceByID: func(ctx context.Context, workspaceID, sourceID pgtype.UUID) (catalog.SourceFacts, bool, error) {
			source, found, err := versions.ReadSource(ctx, workspaceID, sourceID)
			return catalog.SourceFacts{
				SourceType: source.SourceType, SourceURL: source.SourceURL, SourceRef: source.SourceRef,
				ContentHash: source.ContentHash, FetchedAt: source.FetchedAt,
				LastCheckedAt: source.LastCheckedAt, UnavailableSince: source.UnavailableSince,
				TaskDescription: source.TaskDescription, GeneratorModel: source.GeneratorModel,
				GeneratorPromptVersion: source.GeneratorPromptVersion,
			}, found, err
		},
	}
	// The read half of the same inversion as registrySvc's two projection writes
	// (ADR-034): catalog's column, catalog's wording, handed to the owner's list
	// rather than imported by it. Assigned here rather than inside the literal
	// above because catalogSvc does not exist yet at that point — the same reason
	// testlabSvc.ReadSkill and runSvc.ActiveArtifactReferences are assigned late.
	//
	// No adapter, unlike SourceByID directly above: the block crosses as bytes
	// precisely so that neither side can re-declare its shape.
	registrySvc.SkillRisks = catalogSvc.SkillRisks
	registrySvc.CatalogSkillRisks = catalogSvc.CatalogSkillRisks
	// The run history's second axis (04 丙-32). Same shape and same reason as the
	// two above: eval owns `evaluations`, and a JOIN to it from a run-owned query
	// would slip past the ownership checker rather than satisfy it.
	runSvc.RunVerdicts = evalSvc.RunVerdicts

	return &App{
		Deps: Deps{
			Auth:      auth,
			Readiness: cfg.Readiness,
			CleanMode: cfg.CleanMode,
			Importer:  &ingest.Handler{Svc: versions, Identity: auth.Service},
			Search: &catalog.Handler{
				Svc:      catalogSvc,
				Identity: auth.Service,
			},
			Registry: &registry.Handler{
				// Fork indexes, delete and takedown de-index; both writes are
				// catalog's and both stay inside registry's own transaction
				// (ADR-034).
				Svc:      registrySvc,
				Identity: auth.Service,
			},
			TestLab: &testlab.Handler{
				Svc:      testlabSvc,
				Identity: auth.Service,
			},
			Runs:      &run.Handler{Svc: runSvc, Identity: auth.Service},
			Trace:     &trace.Handler{Svc: traceSvc, Identity: auth.Service},
			Eval:      &eval.Handler{Svc: evalSvc, Identity: auth.Service},
			Packaging: &packaging.Handler{Svc: packagingSvc, Identity: auth.Service},
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

// readContentSource adapts Registry's reads into the two facts 02:PORT-010's
// content gate needs (04 丙-85). Named rather than written inline because
// entrypoint/worker carries a line-for-line twin of it, and the two roots must
// not answer this differently — the worker is the one that dispatches, so this
// copy is the API's half and that one is the gate's.
//
// Three reads and not one query: `curation_tier` and `is_catalog` sit in two
// tables owned by two contexts, and the composition root is not allowed to
// JOIN past either owner (ADR-033). CatalogSkill is Registry's existing
// catalogue-scoped read; `found` there is exactly `workspaces.is_catalog`.
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

// Handler builds the route table. Separate from NewApp so a caller can adjust
// the deployment knobs that decide which routes exist at all (the run allowance,
// the invite list, the analytics retention) before the table reads them.
func (a *App) Handler() http.Handler { return NewRouter(a.Deps) }

// AuditRosters records who this deployment recognises, and revokes what it could
// not record.
//
// 02:SEC-011 「授予或撤銷 operator 角色本身也是 audit event」. The roster is
// deployment configuration, so the grant happens outside the application and
// this row at start-up is the only durable record that it happened.
//
// Failing to write it revokes the roster rather than stopping the process: an
// unaudited operator is exactly what the requirement forbids, while an API that
// will not boot because of a database hiccup takes the whole platform with it.
// Nobody is an operator until a start-up manages to record who is.
//
// ADR-028 決策 1 asks for the same record, and fails closed in the other
// direction. An unaudited operator list has to become empty, because an empty one
// grants nothing; an unaudited invite list must not become empty, because an
// empty one admits everybody. So a cohort that could not be recorded shuts the
// gate on everyone until a start-up manages to record it.
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
		// Not fail-closed, unlike the two rosters: this row is a record of a
		// decision somebody already made in the deployment's configuration, not
		// the thing that enforces it. Refusing to serve because the record failed
		// would take the platform down over its own bookkeeping.
		slog.Error("feature flags not audited; a mis-opened flag will leave no trace", "error", err)
	}
}

// logFeatureFlags records which optional entry points this process came up with,
// once per start (ADR-052's own 待決策, answered here).
//
// Written even when everything is off, which is the difference from the two
// rosters above: "nothing was exposed on this deployment, from this time" is the
// statement somebody will need, and an absent row cannot make it — it is equally
// consistent with a build that predates the flag.
func (a *App) logFeatureFlags(ctx context.Context) error {
	// Both maps: the audit answer to "what was on in this process" does not care
	// which of them a flag is served from, and a record that listed only the
	// entry points would have nothing to say about the deployment shape a
	// participant was actually handed.
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

// BetaGateClosed is the invite list of a start-up that could not audit its own
// cohort (ADR-028 決策 1 fail-closed). It has to be non-empty, because an empty
// list means "no closed beta is running" and admits everyone — the opposite of
// what failing closed means here. One entry nobody can hold: a GitHub
// provider_user_id is a decimal string.
func BetaGateClosed() map[string]bool {
	return map[string]bool{"\x00 roster was not recorded": true}
}

// suggesterOrNil keeps a nil *llmclient.Client from becoming a non-nil interface
// value. Without it, "no LLM service configured" would reach the test lab as a
// configured suggester and every suggestion would fail with a nil-pointer panic
// instead of the honest "unavailable".
func suggesterOrNil(c *llmclient.Client) testlab.CriteriaSuggester {
	if c == nil {
		return nil
	}
	return c
}

// The two halves of /me's `features` map, split because they answer different
// questions and public.yaml says so in as many words: "generate_skill (ADR-052)
// is an entry point: it says a route exists. clean_mode (ADR-060) is not — it
// says this deployment... A client that treats clean_mode as something to unlock
// has read it backwards."
//
// The server was doing exactly what that sentence warns against. /me handed the
// whole map out only to callers past the BETA-001 invite check, so a signed-in
// visitor outside the cohort, on a clean-mode deployment, was not told that the
// environment has no sandbox isolation and verifies no signature. A disclosure
// gated on an entitlement is not a disclosure.
//
// Only true ones are listed, and the merged map is absent entirely when there
// are none — a client that has to distinguish "off" from "this build predates
// the flag" is a client that will get one of them wrong.

// entryPointFeatures are the optional routes this deployment mounts. Gated per
// caller at /me, because drawing an entry point for somebody the route will
// refuse is the failure ADR-052's flag exists to prevent, one scope down.
func entryPointFeatures(cfg Config) map[string]bool {
	f := map[string]bool{}
	if cfg.GenerateExposed {
		f["generate_skill"] = true
	}
	return f
}

// disclosureFeatures are the facts about this deployment that every signed-in
// caller is owed regardless of what they may use. Ungated at /me for that
// reason.
//
// clean_mode — PORT-003. A Config field and not an os.Getenv here: the flag is
// a deployment declaration, and reading it in this process's presentation layer
// put a fourth independent reader on a variable whose own comment in
// cmd/api/main.go promises "read exactly once, right here".
func disclosureFeatures(cfg Config) map[string]bool {
	f := map[string]bool{}
	if cfg.CleanMode {
		f["clean_mode"] = true
	}
	return f
}
