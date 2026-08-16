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

	"github.com/ArthurC02/skillhub/services/platform/internal/apiserver"
	"github.com/ArthurC02/skillhub/services/platform/internal/catalog"
	"github.com/ArthurC02/skillhub/services/platform/internal/identity"
	"github.com/ArthurC02/skillhub/services/platform/internal/ingest"
	"github.com/ArthurC02/skillhub/services/platform/internal/llmclient"
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

	importer := &ingest.Handler{
		Svc: &ingest.Service{
			Pool:    pool,
			Store:   store,
			Fetcher: importFetcherFromEnv(),
			LLM:     llm,
		},
		Identity: auth.Service,
	}

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

	// Routes live in internal/apiserver so the integration tests serve this exact
	// table instead of a hand-copied one.
	mux := apiserver.NewRouter(apiserver.Deps{
		Auth:     auth,
		Importer: importer,
		Search:   &catalog.Handler{Pool: pool, Identity: auth.Service, LLMClient: llm, Store: store},
		Registry: &registry.Handler{Svc: &registry.Service{Pool: pool, Store: store}, Identity: auth.Service},
		// llm may be nil: TEST-002's suggestions are then unavailable and the test
		// lab's manual paths carry on unaffected.
		TestLab: &testlab.Handler{
			Svc:      &testlab.Service{Pool: pool, Store: store, LLM: llmOrNil(llm)},
			Identity: auth.Service,
		},
		// The API needs the provider registry for one thing only: refusing a run no
		// configured provider can carry, before it is queued (RUN-005, ADR-004). It
		// never dispatches — that is the worker's job (iron rule 7).
		Runs: &run.Handler{
			// Store is read-only here: the pre-run permission summary scans the
			// stored package to answer "does this carry a script" (02:TEST-005).
			Svc:      &run.Service{Pool: pool, Queue: jobs, Providers: run.NewRegistryFromEnv(), Store: store},
			Identity: auth.Service,
		},
		Trace: &trace.Handler{Svc: traceSvc, Identity: auth.Service},
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

func addrFromEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
