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

	"github.com/ArthurC02/skillhub/services/platform/internal/catalog"
	"github.com/ArthurC02/skillhub/services/platform/internal/identity"
	"github.com/ArthurC02/skillhub/services/platform/internal/ingest"
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

	importer := &ingest.Handler{
		Svc: &ingest.Service{
			Pool:  pool,
			Store: store,
			Fetcher: importFetcherFromEnv(),
		},
		Identity: auth.Service,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", httpx.Health)
	auth.Mount(mux)
	mux.HandleFunc("POST /skills/import/upload", auth.RequireSession(importer.Upload))
	mux.HandleFunc("POST /skills/import/url", auth.RequireSession(importer.ImportURL))

	search := &catalog.Handler{Pool: pool, Identity: auth.Service}
	mux.HandleFunc("GET /skills/search", auth.RequireSession(search.Search))

	reg := &registry.Handler{Svc: &registry.Service{Pool: pool, Store: store}, Identity: auth.Service}
	mux.HandleFunc("GET /skills", auth.RequireSession(reg.List))
	mux.HandleFunc("POST /skills/{id}/fork", auth.RequireSession(reg.Fork))
	mux.HandleFunc("POST /skills/{id}/versions", auth.RequireSession(importer.SaveVersion))
	mux.HandleFunc("GET /skills/{id}/diff", auth.RequireSession(reg.Diff))
	mux.HandleFunc("DELETE /skills/{id}", auth.RequireSession(reg.Delete))

	srv := &http.Server{
		Addr:              addrFromEnv("API_ADDR", ":8080"),
		Handler:           mux,
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
