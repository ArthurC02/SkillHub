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
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/objstore"
	"github.com/ArthurC02/skillhub/services/platform/internal/registry"
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
		Secure:   os.Getenv("COOKIE_INSECURE") != "1", // 1 only for plain-http local dev
		AppURL:   os.Getenv("APP_URL"),
		DevLogin: os.Getenv("DEV_LOGIN") == "1", // offline dev provider; never in production
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

	// Routes live in internal/apiserver so the integration tests serve this exact
	// table instead of a hand-copied one.
	mux := apiserver.NewRouter(apiserver.Deps{
		Auth:     auth,
		Importer: importer,
		Search:   &catalog.Handler{Pool: pool, Identity: auth.Service, LLMClient: llm, Store: store},
		Registry: &registry.Handler{Svc: &registry.Service{Pool: pool, Store: store}, Identity: auth.Service},
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

func addrFromEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
