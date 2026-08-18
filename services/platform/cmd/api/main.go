// Command api serves the Skill Hub public HTTP API (ADR-010 deployment unit 2).
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

	"github.com/ArthurC02/skillhub/services/platform/internal/analytics"
	"github.com/ArthurC02/skillhub/services/platform/internal/apiserver"
	"github.com/ArthurC02/skillhub/services/platform/internal/catalog"
	"github.com/ArthurC02/skillhub/services/platform/internal/eval"
	"github.com/ArthurC02/skillhub/services/platform/internal/identity"
	"github.com/ArthurC02/skillhub/services/platform/internal/ingest"
	"github.com/ArthurC02/skillhub/services/platform/internal/llmclient"
	"github.com/ArthurC02/skillhub/services/platform/internal/packaging"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/httpx"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/metrics"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/objstore"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/queue"
	"github.com/ArthurC02/skillhub/services/platform/internal/registry"
	"github.com/ArthurC02/skillhub/services/platform/internal/run"
	"github.com/ArthurC02/skillhub/services/platform/internal/testlab"
	"github.com/ArthurC02/skillhub/services/platform/internal/trace"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("database pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	auth := &identity.Handler{
		Service: &identity.Service{
			Pool: pool,
			OAuth: &identity.GitHubOAuth{
				ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
				ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
				RedirectURL:  os.Getenv("OAUTH_REDIRECT_URL"),
			},
		},
		Secure:    os.Getenv("COOKIE_INSECURE") != "1", // 1 only for plain-http local dev
		AppURL:    os.Getenv("APP_URL"),
		DevLogin:  os.Getenv("DEV_LOGIN") == "1", // offline dev provider; never in production
		Operators: operatorIDs(os.Getenv("OPERATOR_USER_IDS")),
		// BETA-001's admission list (ADR-028 決策 1), read exactly like the operator
		// roster above and keyed by provider_user_id. Unset — the shipped default —
		// means no closed beta is running and every signed-in user is admitted.
		Invited: operatorIDs(os.Getenv("BETA_ALLOWLIST")),
	}
	// 02:SEC-011 「授予或撤銷 operator 角色本身也是 audit event」. The roster is
	// deployment configuration, so the grant happens outside the application and
	// this row at start-up is the only durable record that it happened.
	//
	// Failing to write it revokes the roster rather than stopping the process:
	// an unaudited operator is exactly what the requirement forbids, while an API
	// that will not boot because of a database hiccup takes the whole platform
	// with it. Nobody is an operator until a start-up manages to record who is.
	if err := auth.LogOperatorRoster(ctx); err != nil {
		slog.Error("operator roster not audited; no operator will be recognised", "error", err)
		auth.Operators = nil
	}
	// ADR-028 決策 1 asks for the same record, and fails closed in the other
	// direction. An unaudited operator list has to become empty, because an empty
	// one grants nothing; an unaudited invite list must not become empty, because
	// an empty one admits everybody. So a cohort that could not be recorded shuts
	// the gate on everyone until a start-up manages to record it.
	if err := auth.LogInviteRoster(ctx); err != nil {
		slog.Error("beta roster not audited; the closed beta gate admits nobody", "error", err)
		auth.Invited = betaGateClosed()
	}

	store, err := objstore.New(
		addrFromEnv("OBJSTORE_ENDPOINT", "localhost:8333"),
		os.Getenv("OBJSTORE_ACCESS_KEY"), // empty = anonymous, local dev only
		os.Getenv("OBJSTORE_SECRET_KEY"),
		addrFromEnv("OBJSTORE_BUCKET", "skillhub"),
		os.Getenv("OBJSTORE_SSL") == "1",
	)
	if err != nil {
		slog.Error("object store", "error", err)
		os.Exit(1)
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
		llm = &llmclient.Client{BaseURL: llmURL}
		slog.Info("llm service configured", "url", llmURL)
	} else {
		slog.Warn("LLM_SERVICE_URL not set; search will use FTS-only fallback and imports will not be enriched")
	}

	versions := &ingest.Service{
		Pool:    pool,
		Store:   store,
		Fetcher: importFetcherFromEnv(),
		LLM:     llm,
	}
	importer := &ingest.Handler{Svc: versions, Identity: auth.Service}

	// Insert-only queue client: the API enqueues run jobs in the same transaction
	// as the run row, and never works one (iron rule 7). Schema migration belongs
	// to cmd/worker, so an API rollout does not touch the queue's tables.
	jobs, err := queue.New(pool, nil)
	if err != nil {
		slog.Error("queue client", "error", err)
		os.Exit(1)
	}

	// TRACE-002: the ingestion credential. Without a secret no ingestion URL is
	// minted, the provider is handed no destination and no events are collected -
	// the honest state for a deployment that has not configured one, and safer
	// than an endpoint anybody could post to.
	traceSigner := &trace.Signer{Secret: []byte(os.Getenv("SKILLHUB_TRACE_INGEST_SECRET"))}
	if !traceSigner.Enabled() {
		slog.Warn("SKILLHUB_TRACE_INGEST_SECRET not set; run traces will not be collected")
	}
	traceSvc := &trace.Service{Pool: pool, Signer: traceSigner}

	// PACK-002: the packaging targets are versioned configuration, read from
	// contracts/packaging/profiles at start-up rather than compiled in, so
	// changing an install path or a support status is a reviewed file and not a
	// release. A deployment with no directory gets no targets and says so on every
	// packaging route — never a hard-coded fallback, which would be the second
	// truth the endpoint exists to avoid.
	profiles, err := packaging.LoadProfiles(addrFromEnv("PACKAGING_PROFILES_DIR", "contracts/packaging/profiles"))
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
	funnel := &analytics.Service{
		Pool:      pool,
		Retention: analyticsRetentionFromEnv(),
		Secure:    os.Getenv("COOKIE_INSECURE") != "1",
	}
	if !funnel.Enabled() {
		slog.Warn("ANALYTICS_RETENTION not set; the BETA-002 funnel is not being measured")
	}

	// The API needs the provider registry for one thing only: refusing a run no
	// configured provider can carry, before it is queued (RUN-005, ADR-004). It
	// never dispatches — that is the worker's job (iron rule 7).
	//
	// Store is read-only here: the pre-run permission summary scans the stored
	// package to answer "does this carry a script" (02:TEST-005).
	runs := &run.Service{
		Pool: pool, Queue: jobs, Providers: run.NewRegistryFromEnv(), Store: store,
		// PDM-010's free allowance, enforced inside the create-run transaction
		// (ADR-028 決策 2). The proposed numbers are the shipped default because
		// the enforcement point does not depend on the final values — what does
		// depend on them is showing them, and GET /me/quota is only mounted where
		// this is enforced. RUN_QUOTA=off turns both off.
		Quota: quotaFromEnv(),
	}

	// Routes live in internal/apiserver so the integration tests serve this exact
	// table instead of a hand-copied one.
	mux := apiserver.NewRouter(apiserver.Deps{
		Auth:     auth,
		Importer: importer,
		Search: &catalog.Handler{
			Pool: pool, Identity: auth.Service, LLMClient: llm, Store: store, Analytics: funnel,
		},
		Registry: &registry.Handler{Svc: &registry.Service{Pool: pool, Store: store}, Identity: auth.Service},
		// llm may be nil: TEST-002's suggestions are then unavailable and the test
		// lab's manual paths carry on unaffected.
		TestLab: &testlab.Handler{
			Svc:      &testlab.Service{Pool: pool, Store: store, LLM: llmOrNil(llm)},
			Identity: auth.Service,
		},
		Runs:  &run.Handler{Svc: runs, Identity: auth.Service},
		Trace: &trace.Handler{Svc: traceSvc, Identity: auth.Service},
		// No judge and no suggester here: producing a verdict and asking for advice
		// are the worker's jobs (iron rule 7). The store and the version writer are
		// wired, because EVAL-002's diff and apply are user actions and both need
		// package bytes — and applying goes through the ordinary version-creation
		// path rather than around it.
		Eval: &eval.Handler{
			Svc:      &eval.Service{Pool: pool, Store: store, Versions: versions},
			Identity: auth.Service,
		},
		// Packaging runs in the control plane and needs no sandbox: it reads
		// stored bytes, filters them and writes a zip, and executes nothing
		// (iron rules 1 and 2).
		Packaging: &packaging.Handler{
			Svc: &packaging.Service{
				Pool: pool, Store: store, Profiles: profiles,
				Retention: retentionFromEnv(),
			},
			Identity: auth.Service,
		},
		Analytics: &analytics.Handler{Svc: funnel, Identity: auth.Service},
	})

	// DEV_CORS_ORIGIN is the local Vite dev server (http://localhost:5173) and
	// nothing else. Unset in production, where the SPA is same-origin with the
	// API (ADR-018 E1) and no CORS header is wanted at all. See httpx.DevCORS for
	// why this is not a Vite proxy: the SPA's /skills/$skillId page route and the
	// API's /skills/{id} routes collide, so no path-prefix rule separates them.
	srv := &http.Server{
		Addr:              addrFromEnv("API_ADDR", ":8080"),
		Handler:           httpx.DevCORS(mux, os.Getenv("DEV_CORS_ORIGIN")),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// O11Y-001~003 on its own listener, never on the public mux: /metrics is an
	// operator surface and the public port is internet-reachable (NFR-005).
	go metrics.Serve(os.Getenv("METRICS_ADDR"))

	// 03:SEC-012's automatic first action, for the one P1 criterion of 02:SEC-010
	// that nothing inside the worker can report: 「Reconciler 停擺 > 10 分鐘」.
	//
	// This is the only periodic work the API process does, and it is here rather
	// than beside the other timers in cmd/worker on purpose — the reconciler is a
	// River periodic job, so a worker that has died takes every watchdog running
	// inside it along with it. The API is the other process that is always up, it
	// already reads this database, and it is one of the two entry points the halt
	// stops (iron rule 7 is untouched: this observes and declares, it never works a
	// job or dispatches one).
	go runs.WatchReconciler(ctx)

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

// betaGateClosed is the invite list of a start-up that could not audit its own
// cohort (ADR-028 決策 1 fail-closed). It has to be non-empty, because an empty
// list means "no closed beta is running" and admits everyone — the opposite of
// what failing closed means here. One entry nobody can hold: a GitHub
// provider_user_id is a decimal string.
func betaGateClosed() map[string]bool {
	return map[string]bool{"\x00 roster was not recorded": true}
}

// importFetcherFromEnv builds the URL-import fetcher: GitHub by default,
// extra hosts via IMPORT_EXTRA_HOSTS (comma-separated), plain http only when
// IMPORT_ALLOW_INSECURE=1 (local stubs and E2E, never production).
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

// llmOrNil keeps a nil *llmclient.Client from becoming a non-nil interface value.
// Without it, "LLM_SERVICE_URL is unset" would reach the test lab as a configured
// suggester and every suggestion would fail with a nil-pointer panic instead of
// the honest "unavailable".
func llmOrNil(c *llmclient.Client) testlab.CriteriaSuggester {
	if c == nil {
		return nil
	}
	return c
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
	if err != nil {
		slog.Warn("DOWNLOAD_ARTIFACT_RETENTION is not a duration; using the default", "value", raw)
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
func quotaFromEnv() run.QuotaLimits {
	if strings.EqualFold(os.Getenv("RUN_QUOTA"), "off") {
		slog.Warn("RUN_QUOTA=off; the PDM-010 run allowance is not enforced and not shown")
		return run.QuotaLimits{}
	}
	return run.DefaultQuotaLimits()
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

func addrFromEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
