// Command api serves the Skill Hub public HTTP API (ADR-010 deployment unit 2).
//
// Everything below the environment is wired by apiserver.NewApp, this process's
// composition root — the API's, and only the API's. The platform has four
// processes and each wires the graph it runs (ADR-032 §5): cmd/worker's root is
// its buildWorkers, cmd/maintenance wires one service per subcommand, cmd/reindex
// wires its backfill in main. They share this file's environment variables, not
// its objects.
//
// What stays here is what is genuinely this process's own: reading the
// environment, the HTTP server, the metrics listener and shutdown.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/apiserver"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/worker"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/queue"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/envx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/delivery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

// SKILLHUB_CLEAN_MODE is the one flag ADR-060 決策 6 grants this process, read
// exactly once, right here — everything it changes is downstream of this
// single bool, and nothing downstream reads the variable itself (⛔ single
// choice point). Unset (the shipped default) must leave every line below
// behaving exactly as it did before this flag existed; that is 02:PORT-005's
// literal acceptance test, in main_test.go.
func cleanModeFromEnv() bool {
	return os.Getenv("SKILLHUB_CLEAN_MODE") == "1"
}

// applyCleanModePool is clean mode's first consequence: a single database
// connection, because the PGlite socket behind it serves one client at a time
// and this same process is about to also run the worker (see the package doc
// on cmd/api and ADR-060 決策 6 for why the two share a pool instead of two).
// Left alone, cfg keeps whatever pgxpool.ParseConfig derived from
// DATABASE_URL on its own — the same value pgxpool.New would have produced,
// which is what main_test.go pins for the flag-unset case.
func applyCleanModePool(cfg *pgxpool.Config, clean bool) {
	if clean {
		cfg.MaxConns = 1
	}
}

// newStore is clean mode's second consequence: production talks to whatever
// OBJSTORE_* points at (objstore.FromEnv), clean mode talks to an in-process
// stand-in that speaks the same S3 wire protocol so every caller downstream
// gets the same *objstore.Client either way (objstore.NewInProcess). The stop
// func is non-nil only for the path that started a server to shut down, which
// is what main_test.go checks for the flag-unset case — the two *Client
// values have no exported field a test could otherwise compare.
func newStore(clean bool) (*objstore.Client, func(), error) {
	if !clean {
		store, err := objstore.FromEnv()
		return store, nil, err
	}
	store, stop, err := objstore.NewInProcess(envx.Or("OBJSTORE_BUCKET", "skillhub"))
	return store, stop, err
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	clean := cleanModeFromEnv()

	poolCfg, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("database pool: DATABASE_URL is not a valid connection string", "error", err)
		os.Exit(1)
	}
	applyCleanModePool(poolCfg, clean)
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		slog.Error("database pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	store, stopStore, err := newStore(clean)
	if err != nil {
		slog.Error("object store", "error", err)
		os.Exit(1)
	}
	if stopStore != nil {
		defer stopStore()
	}
	if err := store.EnsureBucket(ctx); err != nil {
		slog.Error("object store bucket", "error", err)
		os.Exit(1)
	}

	// LLM service client (ADR-016: Python is capability provider).
	// LLM_SERVICE_URL empty = no embeddings: search degrades to FTS-only and
	// imported skills land with enrichment_status = 'pending'.
	var llm *llmclient.Client
	if llmURL := os.Getenv("LLM_SERVICE_URL"); llmURL != "" {
		token := os.Getenv("LLM_SERVICE_TOKEN")
		if token == "" {
			slog.Error("LLM_SERVICE_TOKEN is required when LLM_SERVICE_URL is set")
			os.Exit(1)
		}
		llm = &llmclient.Client{BaseURL: llmURL, Token: token}
		slog.Info("llm service configured", "url", llmURL)
	} else {
		slog.Warn("LLM_SERVICE_URL not set; search will use FTS-only fallback and imports will not be enriched")
	}

	// TRACE-002: the ingestion credential. Without a secret no ingestion URL is
	// minted, the provider is handed no destination and no events are collected -
	// the honest state for a deployment that has not configured one, and safer
	// than an endpoint anybody could post to.
	traceSigner := &trace.Signer{Secret: []byte(os.Getenv("SKILLHUB_TRACE_INGEST_SECRET"))}
	if !traceSigner.Enabled() {
		slog.Warn("SKILLHUB_TRACE_INGEST_SECRET not set; run traces will not be collected")
	}

	// PACK-002: the packaging targets are versioned configuration, read from
	// contracts/packaging/profiles at start-up rather than compiled in, so
	// changing an install path or a support status is a reviewed file and not a
	// release. A deployment with no directory gets no targets and says so on every
	// packaging route — never a hard-coded fallback, which would be the second
	// truth the endpoint exists to avoid.
	profiles, err := packaging.LoadProfiles(envx.Or("PACKAGING_PROFILES_DIR", "contracts/packaging/profiles"))
	if err != nil {
		slog.Error("packaging profiles unreadable; packaging is unavailable", "error", err)
		profiles = nil
	}
	if len(profiles) == 0 {
		slog.Warn("no packaging profiles configured; PACK-001 routes will answer 503")
	}

	// 02:O11Y-004 / ADR-029. ANALYTICS_RETENTION unset means this deployment
	// collects no funnel events at all — no cookie, no rows — which is the correct
	// state until PDM-006 ratifies a retention period (ADR-029 決策 5 proposes 180
	// days). NFR-002 requires the period to exist before the data class starts
	// accumulating, and this is the one class still early enough to obey that in
	// order rather than retrofit it.
	analyticsRetention := analyticsRetentionFromEnv()
	if analyticsRetention < time.Second { // the same threshold analytics.Service.Enabled applies
		slog.Warn("ANALYTICS_RETENTION not set; the BETA-002 funnel is not being measured")
	}

	// Shared with the clean-mode worker below when clean is set — in every other
	// deployment this registry is read here and nowhere else in this process
	// (iron rule 7: the API refuses a run no configured provider can carry, it
	// never dispatches one).
	providers := run.NewRegistryFromEnv()

	app, err := apiserver.NewApp(apiserver.Config{
		Pool:               pool,
		Store:              store,
		LLM:                llm,
		Fetcher:            importFetcherFromEnv(),
		TraceSigner:        traceSigner,
		Profiles:           profiles,
		DownloadRetention:  retentionFromEnv(),
		AnalyticsRetention: analyticsRetention,
		OAuth: &identity.GitHubOAuth{
			ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
			ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
			RedirectURL:  os.Getenv("OAUTH_REDIRECT_URL"),
		},
		Secure:    os.Getenv("COOKIE_INSECURE") != "1", // 1 only for plain-http local dev
		AppURL:    os.Getenv("APP_URL"),
		DevLogin:  os.Getenv("DEV_LOGIN") == "1", // offline dev provider; never in production
		Operators: operatorIDs(os.Getenv("OPERATOR_USER_IDS")),
		// BETA-001's admission list (ADR-028 決策 1), read exactly like the operator
		// roster above and keyed by provider_user_id. Unset — the shipped default —
		// means no closed beta is running and every signed-in user is admitted.
		Invited:         operatorIDs(os.Getenv("BETA_ALLOWLIST")),
		Providers:       providers,
		Quota:           quotaFromEnv(),
		GenerateQuota:   generateQuotaFromEnv(),
		GenerateExposed: generateExposedFromEnv(),
		RateLimits:      rateLimitsFromEnv(),
	})
	if err != nil {
		slog.Error("api composition", "error", err)
		os.Exit(1)
	}
	app.AuditRosters(ctx)

	// Clean mode's third and last consequence: the worker runs inside this same
	// process instead of cmd/worker's own (see the package doc above and ADR-060
	// 決策 6). PollOnly:true is required, not cosmetic — with pool_max_conns=1
	// there is no second connection for River to LISTEN on, and without
	// PollOnly the queue client fails to start rather than falling back.
	var cleanWorker *worker.Set
	if clean {
		if err := queue.EnsureSchema(ctx, pool); err != nil {
			slog.Error("clean mode: queue schema", "error", err)
			os.Exit(1)
		}
		cleanWorker, err = worker.BuildWorkers(pool, worker.Deps{
			Providers:          providers,
			Store:              store,
			Gateway:            run.GatewayFromEnv(),
			TraceSigner:        traceSigner,
			TraceIngestBaseURL: os.Getenv("SKILLHUB_TRACE_INGEST_URL"),
			LLM:                llm,
			PollOnly:           true,
		})
		if err != nil {
			slog.Error("clean mode: worker composition", "error", err)
			os.Exit(1)
		}
		if err := cleanWorker.Queue.Start(ctx); err != nil {
			slog.Error("clean mode: queue start", "error", err)
			os.Exit(1)
		}
		slog.Info("clean mode: worker started in-process")
	}

	// DEV_CORS_ORIGIN is the local Vite dev server (http://localhost:5173) and
	// nothing else. Unset in production, where the SPA is same-origin with the
	// API (ADR-018 E1) and no CORS header is wanted at all. See httpx.DevCORS for
	// why this is not a Vite proxy: the SPA's /skills/$skillId page route and the
	// API's /skills/{id} routes collide, so no path-prefix rule separates them.
	srv := &http.Server{
		Addr:              envx.Or("API_ADDR", ":8080"),
		Handler:           httpx.DevCORS(app.Handler(), os.Getenv("DEV_CORS_ORIGIN")),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// O11Y-001~003 on its own listener, never on the public mux: /metrics is an
	// operator surface and the public port is internet-reachable (NFR-005).
	go metrics.Serve(os.Getenv("METRICS_ADDR"))

	// The periodic work this process does for itself; the roster and the reason
	// for it are on backgroundLoops below.
	for _, loop := range backgroundLoops(app) {
		go loop(ctx)
	}

	go func() {
		slog.Info("api listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("api stopped", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("api shutdown", "error", err)
	}
	if cleanWorker != nil {
		// context.Background(), matching cmd/worker's own shutdown: the context
		// jobs were given is already cancelled, Stop just waits for them to notice.
		if err := cleanWorker.Queue.Stop(context.Background()); err != nil {
			slog.Error("clean mode: queue stop", "error", err)
		}
	}
}

// backgroundLoops is every long-running loop this process starts for itself, as
// a list rather than as bare `go` statements. Nothing here is I/O and nothing is
// started — main starts them — so main_test.go can say which loops a deployment
// is owed. A `go` statement inside main is deletable with nothing red, which is
// the whole reason this is a function.
//
// WatchReconciler is 03:SEC-012's automatic first action, for the one P1
// criterion of 02:SEC-010 that nothing inside the worker can report:
// 「Reconciler 停擺 > 10 分鐘」. It runs here rather than beside the other timers
// in cmd/worker on purpose — the reconciler is a River periodic job, so a worker
// that has died takes every watchdog running inside it along with it. The API is
// the other process that is always up, it already reads this database, and it is
// one of the two entry points the halt stops (iron rule 7 is untouched: this
// observes and declares, it never works a job or dispatches one).
func backgroundLoops(app *apiserver.App) []func(context.Context) {
	return []func(context.Context){
		app.RunSvc.WatchReconciler,
	}
}

// operatorIDs parses OPERATOR_USER_IDS, a comma-separated list of user ids
// (02:SEC-011). Unset — the shipped default — means nobody is an operator and
// every operator route answers 404. Nothing is validated as a UUID here: an id
// that is not one simply never matches a session user, which is the same
// outcome as leaving it out.
func operatorIDs(raw string) map[string]bool {
	out := map[string]bool{}
	for _, id := range strings.Split(raw, ",") {
		if id = strings.TrimSpace(id); id != "" {
			out[id] = true
		}
	}
	return out
}

// importFetcherFromEnv builds the URL-import fetcher: GitHub by default,
// extra hosts via IMPORT_EXTRA_HOSTS (comma-separated), plain http only when
// IMPORT_ALLOW_INSECURE=1 (local stubs and E2E, never production).
//
// That flag now carries a second meaning, and it is the more dangerous one:
// it also allows loopback and RFC1918 as destinations, because httptest and
// compose both live there. The addresses that make SSRF worth defending
// against -- link-local and its v6 mapping, CGNAT, broadcast, unspecified,
// multicast -- stay blocked either way (03:INGEST-014).
func importFetcherFromEnv() *ingest.URLFetcher {
	f := &ingest.URLFetcher{
		Allowed:       ingest.DefaultAllowedHosts(),
		AllowInsecure: os.Getenv("IMPORT_ALLOW_INSECURE") == "1",
	}
	for _, h := range strings.Split(os.Getenv("IMPORT_EXTRA_HOSTS"), ",") {
		if h = strings.TrimSpace(strings.ToLower(h)); h != "" {
			f.Allowed[h] = true
		}
	}
	return f
}

// retentionFromEnv reads how long a Download Artifact lives. Deployment
// configuration and not schema: PDM-006 proposes 90 days and that proposal is
// not ratified, so 0027 records the pointer and leaves the number here where a
// deployment can set it (m4/README §8.1). An unparseable value falls back to the
// package default rather than stopping the process.
func retentionFromEnv() time.Duration {
	raw := os.Getenv("DOWNLOAD_ARTIFACT_RETENTION")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		slog.Warn("DOWNLOAD_ARTIFACT_RETENTION is invalid; artifact creation is disabled", "value", raw)
		return 0
	}
	return d
}

// quotaFromEnv reads the PDM-010 free run allowance (ADR-028 決策 2).
//
// The proposal's numbers are the default rather than something a deployment has to
// set, and that asymmetry with the two retention knobs above is deliberate. An
// unset retention means data is not collected, which is safe; an unset allowance
// would mean the platform's only real cost ceiling is off, which is not — 01 §12
// lists unsustainable sandbox cost as a live risk and this is its one mitigation.
//
// RUN_QUOTA=off is the escape hatch, and it turns off the display as well: with no
// allowance enforced, GET /me/quota is not mounted and the pre-run summary carries
// no quota block. The two move together on purpose (04 乙-2).
func quotaFromEnv() policy.QuotaLimits {
	if strings.EqualFold(os.Getenv("RUN_QUOTA"), "off") {
		slog.Warn("RUN_QUOTA=off; the PDM-010 run allowance is not enforced and not shown")
		return policy.QuotaLimits{}
	}
	return policy.DefaultQuotaLimits()
}

// generateQuotaFromEnv reads the generation allowance (GEN-004, ADR-047 決策 5).
//
// Same asymmetry as RUN_QUOTA and for the same reason: unset means enforced, and
// turning it off takes an action somebody has to write down. ADR-055 made that
// mistake visible for runs — 05 R-1a had recorded unset as meaning unenforced,
// which is the opposite of what the code says, and the difference is whether a
// deployment that configured nothing has a cost ceiling.
//
// A second env var and not a shared one: the two allowances are counted
// separately on purpose, and one switch turning off both would be the shared
// pool ADR-047 決策 5 ruled against, wearing different clothes.
func generateQuotaFromEnv() policy.QuotaLimits {
	if strings.EqualFold(os.Getenv("GENERATE_QUOTA"), "off") {
		slog.Warn("GENERATE_QUOTA=off; the generation allowance is not enforced and not shown")
		return policy.QuotaLimits{}
	}
	return policy.DefaultGenerateQuotaLimits()
}

// rateLimitsFromEnv builds NFR-001 clause 5's limiter.
//
// Unset = enforced with defaults, `off` = none — the RUN_QUOTA convention: a
// protection left unconfigured must not silently be absent. The numbers are
// operational tuning (nothing displays them, so 04 乙-2 does not bite): 60
// requests a minute with a burst of 30, per client IP, across the three
// endpoints the clause names — invisible to a person, a wall to a loop.
func rateLimitsFromEnv() *httpx.RateLimiter {
	if strings.EqualFold(os.Getenv("RATE_LIMIT"), "off") {
		slog.Warn("RATE_LIMIT=off; anonymous search and the import endpoints have no rate limit (02:NFR-001 clause 5)")
		return nil
	}
	return httpx.NewRateLimiter(60, 30)
}

// generateExposedFromEnv reads ADR-052's exposure flag.
//
// The asymmetry runs the OTHER way from the two allowances above, and
// deliberately: unset means NOT exposed. An allowance left unconfigured leaves
// the platform's cost ceiling open, so unset has to mean enforced; an entry
// point left unconfigured just is not there yet, and ADR-052 says the default
// is off. Turning it on takes an action somebody has to write down, which is
// the point — 01 §11.2's first funnel segment measures whether search works,
// and a beta participant who meets "搜不到 → 生成一個" is measuring something
// else. That number has one chance, with twelve people.
//
// Not a rebuild-time constant and not a client-side flag: the web asks /me,
// because the same build has to be able to serve a cohort that sees it and one
// that does not.
func generateExposedFromEnv() bool {
	raw := os.Getenv("GENERATE_SKILL_EXPOSED")
	switch {
	case strings.EqualFold(raw, "on"):
		slog.Warn("GENERATE_SKILL_EXPOSED=on; the M5 generation entry point is visible. " +
			"ADR-052 requires 01 §11.2's first funnel segment to have a reading first")
		return true
	case raw != "" && !strings.EqualFold(raw, "off"):
		// `true`, `1`, `yes` and a stray space all mean off here, and silence
		// would let somebody believe they had opened the entry point. RATE_LIMIT
		// says so when it is turned off; this says so when it was not turned on.
		slog.Warn("GENERATE_SKILL_EXPOSED is neither `on` nor `off`; the M5 generation entry point stays hidden",
			"value", raw)
	}
	return false
}

// analyticsRetentionFromEnv reads how long a funnel event is kept, and therefore
// whether any are collected at all. Deployment configuration and not a constant,
// for the same reason DOWNLOAD_ARTIFACT_RETENTION is: ADR-029 決策 5's 180 days
// is a proposal, and compiling in an unratified number would make "已定值" and
// "已被追認" the same thing. Unset is off, not a default.
func analyticsRetentionFromEnv() time.Duration {
	raw := os.Getenv("ANALYTICS_RETENTION")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		slog.Warn("ANALYTICS_RETENTION is not a duration; funnel events are not collected", "value", raw)
		return 0
	}
	return d
}
