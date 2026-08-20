package apiserver

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/analytics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/catalog"
	"github.com/ArthurC02/skillhub/apps/platform/internal/eval"
	"github.com/ArthurC02/skillhub/apps/platform/internal/identity"
	"github.com/ArthurC02/skillhub/apps/platform/internal/ingest"
	"github.com/ArthurC02/skillhub/apps/platform/internal/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/packaging"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/queue"
	"github.com/ArthurC02/skillhub/apps/platform/internal/registry"
	"github.com/ArthurC02/skillhub/apps/platform/internal/run"
	"github.com/ArthurC02/skillhub/apps/platform/internal/testlab"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trace"
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

	OAuth    *identity.GitHubOAuth
	Secure   bool
	AppURL   string
	DevLogin bool
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
	Quota run.QuotaLimits
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

// NewApp builds the API's object graph. It is the only place the contexts are
// wired together (ADR-032 §5): cmd/api and the integration tests both call it,
// so a dependency that is missing here is missing in both.
func NewApp(cfg Config) (*App, error) {
	auth := &identity.Handler{
		Service:   &identity.Service{Pool: cfg.Pool, OAuth: cfg.OAuth},
		Secure:    cfg.Secure,
		AppURL:    cfg.AppURL,
		DevLogin:  cfg.DevLogin,
		Operators: cfg.Operators,
		Invited:   cfg.Invited,
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

	versions := &ingest.Service{
		Pool:    cfg.Pool,
		Store:   cfg.Store,
		Fetcher: cfg.Fetcher,
		LLM:     cfg.LLM,
	}

	traceSvc := &trace.Service{Pool: cfg.Pool, Signer: cfg.TraceSigner}

	// Store is read-only here: the pre-run permission summary scans the stored
	// package to answer "does this carry a script" (02:TEST-005).
	runSvc := &run.Service{
		Pool: cfg.Pool, Queue: jobs, Providers: cfg.Providers, Store: cfg.Store,
		Quota: cfg.Quota,
	}

	// No judge and no suggester: producing a verdict and asking for advice are the
	// worker's jobs (iron rule 7). The store and the version writer are wired,
	// because EVAL-002's diff and apply are user actions and both need package
	// bytes — and applying goes through the ordinary version-creation path rather
	// than around it.
	evalSvc := &eval.Service{
		Pool: cfg.Pool, Store: cfg.Store, Versions: versions, Trace: traceSvc,
	}

	// Packaging runs in the control plane and needs no sandbox: it reads stored
	// bytes, filters them and writes a zip, and executes nothing (iron rules 1
	// and 2).
	packagingSvc := &packaging.Service{
		Pool: cfg.Pool, Store: cfg.Store, Profiles: cfg.Profiles,
		Retention: cfg.DownloadRetention,
	}

	return &App{
		Deps: Deps{
			Auth:     auth,
			Importer: &ingest.Handler{Svc: versions, Identity: auth.Service},
			Search: &catalog.Handler{
				Svc: &catalog.Service{
					Pool: cfg.Pool, LLM: cfg.LLM, Store: cfg.Store, Analytics: funnel,
				},
				Identity: auth.Service,
			},
			Registry: &registry.Handler{
				Svc:      &registry.Service{Pool: cfg.Pool, Store: cfg.Store},
				Identity: auth.Service,
			},
			TestLab: &testlab.Handler{
				Svc:      &testlab.Service{Pool: cfg.Pool, Store: cfg.Store, LLM: suggesterOrNil(cfg.LLM)},
				Identity: auth.Service,
			},
			Runs:      &run.Handler{Svc: runSvc, Identity: auth.Service},
			Trace:     &trace.Handler{Svc: traceSvc, Identity: auth.Service},
			Eval:      &eval.Handler{Svc: evalSvc, Identity: auth.Service},
			Packaging: &packaging.Handler{Svc: packagingSvc, Identity: auth.Service},
			Analytics: &analytics.Handler{Svc: funnel, Identity: auth.Service},
		},
		Auth:         auth,
		RunSvc:       runSvc,
		EvalSvc:      evalSvc,
		PackagingSvc: packagingSvc,
		Versions:     versions,
		TraceSvc:     traceSvc,
	}, nil
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
