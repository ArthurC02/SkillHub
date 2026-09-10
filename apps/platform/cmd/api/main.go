package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
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

func cleanModeFromEnv() bool {
	return os.Getenv("SKILLHUB_CLEAN_MODE") == "1"
}

var cleanModeFlagPlaceholder = []byte("<!--SKILLHUB_CLEAN_MODE_FLAG-->")

const cleanModeFlagJS = `window.__SKILLHUB_CLEAN_MODE__=true;`

var cleanModeFlagScript = []byte(`<script>` + cleanModeFlagJS + `</script>`)

const devLoginFlagJS = `window.__SKILLHUB_DEV_LOGIN__=true;`

var devLoginFlagScript = []byte(`<script>` + devLoginFlagJS + `</script>`)

var contentSecurityPolicyBase = []string{
	"default-src 'self'",
	"img-src 'self' data: blob:",
	"style-src 'self'",
	"font-src 'self'",
	"connect-src 'self'",
	"object-src 'none'",
	"base-uri 'self'",
	"form-action 'self'",
	"frame-ancestors 'none'",
}

func contentSecurityPolicy(inlineScripts ...string) string {
	sources := []string{"'self'"}
	for _, js := range inlineScripts {
		sum := sha256.Sum256([]byte(js))
		sources = append(sources, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	}
	return strings.Join(
		append(append([]string{}, contentSecurityPolicyBase...), "script-src "+strings.Join(sources, " ")),
		"; ")
}

func applyCleanModePool(cfg *pgxpool.Config, clean bool) {
	if !clean {
		return
	}
	cfg.MaxConns = 1

	// A fresh connection can land back on a Postgres session an earlier
	// connection left prepared statements in; clear it so pgx's own
	// statement cache does not collide with names already on the server.
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "DEALLOCATE ALL")
		return err
	}

	// CacheDescribe never names a server-side statement, so nothing here
	// needs deallocating on the next reconnect either.
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe
}

func newStore(clean bool) (*objstore.Client, func(), error) {
	if !clean {
		store, err := objstore.FromEnv()
		return store, nil, err
	}
	store, stop, err := objstore.NewInProcess(envx.Or("OBJSTORE_BUCKET", "skillhub"))
	return store, stop, err
}

func cleanModeStaticHandler(devLogin bool) (http.Handler, error) {
	distDir, err := webDistDir()
	if err != nil {
		return nil, err
	}
	return webStaticHandlerUnder(distDir, devLogin)
}

func webDistDir() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("cannot locate the web build: this binary carries no source path, so it was not built from this repository")
	}

	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
	return filepath.Join(repoRoot, "apps", "web", "dist"), nil
}

func webStaticHandlerUnder(distDir string, devLogin bool) (http.Handler, error) {
	indexPath := filepath.Join(distDir, "index.html")
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf(
			"clean mode cannot find the web build at %s (derived from this binary's build path; run `npm --prefix apps/web run build` first): %w",
			indexPath, err)
	}
	flags := cleanModeFlagScript
	inlineScripts := []string{cleanModeFlagJS}
	if devLogin {
		flags = append(append([]byte{}, flags...), devLoginFlagScript...)
		inlineScripts = append(inlineScripts, devLoginFlagJS)
	}

	csp := contentSecurityPolicy(inlineScripts...)
	injected := bytes.Replace(raw, cleanModeFlagPlaceholder, flags, 1)
	if bytes.Equal(injected, raw) {
		return nil, fmt.Errorf(
			"clean mode: %s has no %q placeholder to inject into; 02:PORT-003's disclosure would not reach a signed-out visitor",
			indexPath, cleanModeFlagPlaceholder)
	}

	mux := http.NewServeMux()

	mux.Handle("GET /assets/", http.FileServer(http.Dir(distDir)))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {

		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(injected)
	})
	return mux, nil
}

func cleanModeHandler(api http.Handler, clean bool, static http.Handler) http.Handler {
	if !clean {
		return api
	}
	mux := http.NewServeMux()
	mux.Handle("GET /assets/", static)
	mux.Handle("GET /{$}", static)
	mux.Handle("/", spaFallback(api, static))
	return mux
}

func spaFallback(api, static http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.Header.Get("Accept"), "text/html") {
			api.ServeHTTP(w, r)
			return
		}
		catcher := &navigationCatcher{ResponseWriter: w}
		api.ServeHTTP(catcher, r)
		if !catcher.swallowed {
			return
		}

		clear(w.Header())
		index := r.Clone(r.Context())
		index.URL = &url.URL{Path: "/"}
		index.RequestURI = "/"
		static.ServeHTTP(w, index)
	})
}

type navigationCatcher struct {
	http.ResponseWriter
	swallowed bool
	wrote     bool
}

func (c *navigationCatcher) WriteHeader(code int) {
	if c.wrote {
		return
	}
	c.wrote = true
	okJSON := code >= 200 && code < 300 &&
		strings.Contains(c.Header().Get("Content-Type"), "application/json")
	if code == http.StatusNotFound || code == http.StatusMethodNotAllowed || okJSON {
		c.swallowed = true
		return
	}
	c.ResponseWriter.WriteHeader(code)
}

func (c *navigationCatcher) Write(b []byte) (int, error) {
	if !c.wrote {
		c.WriteHeader(http.StatusOK)
	}
	if c.swallowed {
		return len(b), nil
	}
	return c.ResponseWriter.Write(b)
}

func devLoginRefusal(devLogin, secure bool) string {
	if !devLogin || !secure {
		return ""
	}
	return "DEV_LOGIN=1 with secure session cookies: the offline login provider " +
		"lets anybody sign in as any name without a credential (ADR-020), and a " +
		"deployment that terminates TLS is not a deployment that wants it. Unset " +
		"DEV_LOGIN, or set COOKIE_INSECURE=1 if this really is plain-http local dev."
}

func main() {
	creationLimits, _ := creation.LimitsFromEnv()
	var cleanWorker *worker.Set
	creationTransient := creation.TransientClient(os.Getenv("CREATION_WORKER_INTERNAL_URL"), os.Getenv("CREATION_WORKER_INTERNAL_TOKEN"), creationLimits.CallTimeout+30*time.Second)

	if len(os.Args) > 1 && os.Args[1] == "--capabilities" {
		if err := printCapabilitiesJSON(os.Stdout); err != nil {
			slog.Error("print capabilities", "error", err)
			os.Exit(1)
		}
		return
	}

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

	traceSigner := &trace.Signer{Secret: []byte(os.Getenv("SKILLHUB_TRACE_INGEST_SECRET"))}
	if !traceSigner.Enabled() {
		slog.Warn("SKILLHUB_TRACE_INGEST_SECRET not set; run traces will not be collected")
	}

	profileDir := envx.Or("PACKAGING_PROFILES_DIR", "contracts/packaging/profiles")
	profiles, err := packaging.LoadProfiles(profileDir)
	if err != nil {
		slog.Error("packaging profiles unreadable; packaging is unavailable", "error", err)
		profiles = nil
	}
	if len(profiles) == 0 {

		resolved, absErr := filepath.Abs(profileDir)
		if absErr != nil {
			resolved = profileDir
		}
		slog.Warn("no packaging profiles configured; PACK-001 routes will answer 503",
			"reason", profileDirReason(profileDir), "dir", profileDir, "resolved", resolved)
	}

	analyticsRetention := analyticsRetentionFromEnv()
	if analyticsRetention < time.Second {
		slog.Warn("ANALYTICS_RETENTION not set; the BETA-002 funnel is not being measured")
	}

	providers := run.NewRegistryFromEnv()

	secure := os.Getenv("COOKIE_INSECURE") != "1"
	devLogin := os.Getenv("DEV_LOGIN") == "1"
	if reason := devLoginRefusal(devLogin, secure); reason != "" {
		slog.Error("refusing to start", "reason", reason)
		os.Exit(1)
	}
	if devLogin {
		slog.Warn("DEV_LOGIN=1; POST /auth/dev/login is mounted and anybody can sign in " +
			"as any name without a credential (ADR-020). Never in production")
	}

	capabilities := capabilityTable(pool, len(profiles), clean)
	reportCapabilities(ctx, capabilities)

	if clean {
		creationTransient = func(ctx context.Context, a creation.JobArgs, d *llmclient.GenerateDiagram) error {
			if cleanWorker == nil {
				return creation.ErrUnavailable
			}
			return cleanWorker.Creation.Step(ctx, a, d)
		}
	}
	app, err := apiserver.NewApp(apiserver.Config{
		Pool:               pool,
		Readiness:          capabilities,
		Store:              store,
		LLM:                llm,
		Fetcher:            importFetcherFromEnv(),
		TraceSigner:        traceSigner,
		Profiles:           profiles,
		DownloadRetention:  retentionFromEnv(),
		AnalyticsRetention: analyticsRetention,
		FeedbackRetention:  feedbackRetentionFromEnv(),
		OAuth: &identity.GitHubOAuth{
			ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
			ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
			RedirectURL:  os.Getenv("OAUTH_REDIRECT_URL"),
		},
		Secure:    secure,
		AppURL:    os.Getenv("APP_URL"),
		DevLogin:  devLogin,
		Operators: operatorIDs(os.Getenv("OPERATOR_USER_IDS")),

		Invited:         operatorIDs(os.Getenv("BETA_ALLOWLIST")),
		Providers:       providers,
		Quota:           quotaFromEnv(),
		GenerateQuota:   generateQuotaFromEnv(),
		GenerateExposed: generateExposedFromEnv(),
		CreationExposed: creation.Exposed(), CreationLimits: creationLimits, CreationTransient: creationTransient,
		RateLimits: rateLimitsFromEnv(),

		CleanMode: clean,
	})
	if err != nil {
		slog.Error("api composition", "error", err)
		os.Exit(1)
	}
	for _, task := range startupTasks(app) {
		task(ctx)
	}

	if clean {
		if err := queue.EnsureSchema(ctx, pool); err != nil {
			slog.Error("clean mode: queue schema", "error", err)
			os.Exit(1)
		}
		cleanWorker, err = worker.BuildWorkers(pool, worker.Deps{
			CreationLimits:     creationLimits,
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

	handler := app.Handler()
	if clean {
		static, err := cleanModeStaticHandler(devLogin)
		if err != nil {
			slog.Error("clean mode: web assets", "error", err)
			os.Exit(1)
		}
		handler = cleanModeHandler(handler, clean, static)
		slog.Info("clean mode: serving the web build with the 02:PORT-003 disclosure flag injected")
	}

	srv := &http.Server{
		Addr:              envx.Or("API_ADDR", ":8080"),
		Handler:           httpx.DevCORS(handler, os.Getenv("DEV_CORS_ORIGIN")),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go metrics.Serve(os.Getenv("METRICS_ADDR"))

	for _, loop := range backgroundLoops(app) {
		go loop(ctx)
	}

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("api listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	failed := false
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		slog.Error("api stopped", "error", err)
		failed = true
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("api shutdown", "error", err)
	}
	if cleanWorker != nil {
		queue.Stop(cleanWorker.Queue)
	}
	if failed {

		pool.Close()
		if stopStore != nil {
			stopStore()
		}
		os.Exit(1)
	}
}

func startupTasks(app *apiserver.App) []func(context.Context) {
	return []func(context.Context){
		app.AuditRosters,
	}
}

func backgroundLoops(app *apiserver.App) []func(context.Context) {
	return []func(context.Context){
		app.RunSvc.WatchReconciler,
	}
}

func operatorIDs(raw string) map[string]bool {
	out := map[string]bool{}
	for _, id := range strings.Split(raw, ",") {
		if id = strings.TrimSpace(id); id != "" {
			out[id] = true
		}
	}
	return out
}

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

func retentionFromEnv() time.Duration {
	raw := os.Getenv("DOWNLOAD_ARTIFACT_RETENTION")
	if raw == "" {

		slog.Warn("DOWNLOAD_ARTIFACT_RETENTION is unset; building a download package will answer 503. " +
			"This value has no default on purpose: it is the retention promise shown to users, and PDM-006's " +
			"proposed 90 days is not ratified (GOV-RETENTION-001)")
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		slog.Warn("DOWNLOAD_ARTIFACT_RETENTION is invalid; artifact creation is disabled", "value", raw)
		return 0
	}
	return d
}

func profileDirReason(dir string) string {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return "the directory does not exist; note that a relative path resolves from this process's working directory, not from the repository root"
	}
	return "the directory exists but holds no *.json profile"
}

func quotaFromEnv() policy.QuotaLimits {
	if strings.EqualFold(os.Getenv("RUN_QUOTA"), "off") {
		slog.Warn("RUN_QUOTA=off; the PDM-010 run allowance is not enforced and not shown")
		return policy.QuotaLimits{}
	}
	return policy.DefaultQuotaLimits()
}

func generateQuotaFromEnv() policy.QuotaLimits {
	if strings.EqualFold(os.Getenv("GENERATE_QUOTA"), "off") {
		slog.Warn("GENERATE_QUOTA=off; the generation allowance is not enforced and not shown")
		return policy.QuotaLimits{}
	}
	return policy.DefaultGenerateQuotaLimits()
}

func rateLimitsFromEnv() *httpx.RateLimiter {
	if strings.EqualFold(os.Getenv("RATE_LIMIT"), "off") {
		slog.Warn("RATE_LIMIT=off; anonymous search and the import endpoints have no rate limit (02:NFR-001 clause 5)")
		return nil
	}
	return httpx.NewRateLimiter(60, 30)
}

func generateExposedFromEnv() bool {
	raw := os.Getenv("GENERATE_SKILL_EXPOSED")
	switch {
	case strings.EqualFold(raw, "on"):
		slog.Warn("GENERATE_SKILL_EXPOSED=on; the M5 generation entry point is visible. " +
			"ADR-052 requires 01 §11.2's first funnel segment to have a reading first")
		return true
	case raw != "" && !strings.EqualFold(raw, "off"):

		slog.Warn("GENERATE_SKILL_EXPOSED is neither `on` nor `off`; the M5 generation entry point stays hidden",
			"value", raw)
	}
	return false
}

func feedbackRetentionFromEnv() time.Duration {
	raw := os.Getenv("FEEDBACK_RETENTION")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		slog.Warn("FEEDBACK_RETENTION is not a positive duration; /policy/data-retention will report that feedback is kept indefinitely", "value", raw)
		return 0
	}
	return d
}

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
