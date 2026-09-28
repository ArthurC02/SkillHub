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
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/worker"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/queue"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/envx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/delivery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func cleanModeFromEnv() bool {
	return os.Getenv("SKILLHUB_CLEAN_MODE") == "1"
}

type deployment int

const (
	productionDeployment deployment = iota
	cleanModeDeployment
)

func deploymentFromEnv() deployment {
	if cleanModeFromEnv() {
		return cleanModeDeployment
	}
	return productionDeployment
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

func applyCleanModePool(cfg *pgxpool.Config, d deployment) {
	if d != cleanModeDeployment {
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

func newStore(d deployment) (*objstore.Client, func(), error) {
	if d != cleanModeDeployment {
		store, err := wiring.ObjectStoreFromEnv()
		return store, nil, err
	}
	store, stop, err := objstore.NewInProcess(envx.Or(os.Getenv("OBJSTORE_BUCKET"), "skillhub"))
	return store, stop, err
}

func cleanModeStaticHandler(posture envx.Posture) (http.Handler, error) {
	distDir, err := webDistDir()
	if err != nil {
		return nil, err
	}
	return webStaticHandlerUnder(distDir, posture)
}

func webDistDir() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("cannot locate the web build: this binary carries no source path, so it was not built from this repository")
	}

	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
	return filepath.Join(repoRoot, "apps", "web", "dist"), nil
}

func webStaticHandlerUnder(distDir string, posture envx.Posture) (http.Handler, error) {
	indexPath := filepath.Join(distDir, "index.html")
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf(
			"clean mode cannot find the web build at %s (derived from this binary's build path; run `npm --prefix apps/web run build` first): %w",
			indexPath, err)
	}
	flags := cleanModeFlagScript
	inlineScripts := []string{cleanModeFlagJS}
	if posture.DevLogin {
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

func cleanModeHandler(api http.Handler, d deployment, static http.Handler) http.Handler {
	if d != cleanModeDeployment {
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

func startupRefusals(posture envx.Posture, providers *run.Registry, rateLimitErr error) []string {
	refusals := append(posture.APIRefusals(), providers.UnauthenticatedProviderRefusals()...)
	if rateLimitErr != nil {
		refusals = append(refusals, rateLimitErr.Error())
	}
	return refusals
}

func main() {
	if failed := runAPI(); failed {
		os.Exit(1)
	}
}

func runAPI() (failed bool) {
	creationLimits, _ := wiring.CreationLimitsFromEnv()
	var cleanWorker *worker.Set
	creationTransient := wiring.CreationTransientFromEnv(creationLimits)

	if len(os.Args) > 1 && os.Args[1] == "--capabilities" {
		exitOn(printCapabilitiesJSON(os.Stdout), "print capabilities")
		return false
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mode := deploymentFromEnv()
	clean := mode == cleanModeDeployment

	pool := openPool(ctx, mode)
	defer pool.Close()

	store, stopStore := openStore(ctx, mode)
	if stopStore != nil {
		defer stopStore()
	}

	llm := llmFromEnv()
	traceSigner := traceSignerFromEnv()
	profiles := packagingProfilesFromEnv()
	analyticsRetention := measuredAnalyticsRetention()

	providers := wiring.NewRunRegistryFromEnv()
	runDeployment := wiring.RunDeploymentFromEnv()

	posture := wiring.PostureFromEnv()
	rateLimits, rateLimitErr := rateLimitsFromEnv()
	refuseToStartOn(startupRefusals(posture, providers, rateLimitErr))
	warnWhenDevLoginOpen(posture)

	capabilities := capabilityTableFor(mode, pool, len(profiles))
	reportCapabilities(ctx, capabilities)

	if clean {
		creationTransient = inProcessCreation(&cleanWorker)
	}
	cfg := apiConfigFromEnv(posture)
	cfg.Pool, cfg.Readiness, cfg.Store, cfg.LLM, cfg.TraceSigner = pool, capabilities, store, llm, traceSigner
	cfg.Profiles, cfg.AnalyticsRetention, cfg.Providers, cfg.RunDeployment = profiles, analyticsRetention, providers, runDeployment
	cfg.CreationLimits, cfg.CreationTransient, cfg.RateLimits, cfg.CleanMode = creationLimits, creationTransient, rateLimits, clean
	app, err := apiserver.NewApp(cfg)
	exitOn(err, "api composition")
	for _, task := range startupTasks(app) {
		task(ctx)
	}

	if clean {
		cleanWorker = startCleanWorker(ctx, pool, inProcessWorkerDeps(cfg, store))
	}

	handler := app.Handler()
	if clean {
		handler = cleanModeServing(handler, mode, posture)
	}
	srv := newAPIServer(handler, posture)

	go metrics.Serve(os.Getenv("METRICS_ADDR"))

	for _, loop := range backgroundLoops(app) {
		go loop(ctx)
	}

	failed = serveUntilStopped(ctx, srv)
	shutdownAPI(srv, cleanWorker)
	return failed
}

const (
	apiReadHeaderTimeout = 5 * time.Second
	apiShutdownGrace     = 10 * time.Second
	githubOAuthTimeout   = 15 * time.Second
)

func openPool(ctx context.Context, mode deployment) *pgxpool.Pool {
	poolCfg, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	exitOn(err, "database pool: DATABASE_URL is not a valid connection string")
	applyCleanModePool(poolCfg, mode)
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	exitOn(err, "database pool")
	return pool
}

func openStore(ctx context.Context, mode deployment) (*objstore.Client, func()) {
	store, stopStore, err := newStore(mode)
	exitOn(err, "object store")
	exitOn(store.EnsureBucket(ctx), "object store bucket")
	return store, stopStore
}

func measuredAnalyticsRetention() time.Duration {
	analyticsRetention := analyticsRetentionFromEnv()
	if analyticsRetention < time.Second {
		slog.Warn("ANALYTICS_RETENTION not set; the BETA-002 funnel is not being measured")
	}
	return analyticsRetention
}

func warnWhenDevLoginOpen(posture envx.Posture) {
	if posture.DevLogin {
		slog.Warn("DEV_LOGIN=1; POST /auth/dev/login is mounted and anybody can sign in " +
			"as any name without a credential. Never in production")
	}
}

func capabilityTableFor(mode deployment, pool *pgxpool.Pool, packagingTargets int) *envx.Registry {
	if mode == cleanModeDeployment {
		return cleanModeCapabilityTable(pool, packagingTargets)
	}
	return capabilityTable(pool, packagingTargets)
}

func apiConfigFromEnv(posture envx.Posture) apiserver.Config {
	return apiserver.Config{
		Fetcher:           wiring.ImportFetcher(posture),
		DownloadRetention: retentionFromEnv(),
		FeedbackRetention: feedbackRetentionFromEnv(),
		OAuth: &identity.GitHubOAuth{
			ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
			ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
			RedirectURL:  os.Getenv("OAUTH_REDIRECT_URL"),
			Client:       &http.Client{Timeout: githubOAuthTimeout},
		},
		Secure:    posture.SecureCookies,
		AppURL:    posture.AppURL,
		DevLogin:  posture.DevLogin,
		Operators: operatorIDs(os.Getenv("OPERATOR_USER_IDS")),

		Invited:         operatorIDs(os.Getenv("BETA_ALLOWLIST")),
		Quota:           quotaFromEnv(),
		GenerateQuota:   generateQuotaFromEnv(),
		GenerateExposed: generateExposedFromEnv(),
		CreationExposed: wiring.CreationExposedFromEnv(),

		PublicationDownloadsOpen: publicationDownloadsOpenFromEnv(),
	}
}

func newAPIServer(handler http.Handler, posture envx.Posture) *http.Server {
	return &http.Server{
		Addr:              envx.Or(os.Getenv("API_ADDR"), ":8080"),
		Handler:           httpx.DevCORS(handler, posture.DevCORSOrigin),
		ReadHeaderTimeout: apiReadHeaderTimeout,
	}
}

func shutdownAPI(srv *http.Server, cleanWorker *worker.Set) {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), apiShutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("api shutdown", "error", err)
	}
	if cleanWorker != nil {
		queue.Stop(cleanWorker.Queue)
	}
}

func exitOn(err error, msg string) {
	if err != nil {
		slog.Error(msg, "error", err)
		os.Exit(1)
	}
}

func refuseToStartOn(refusals []string) {
	if len(refusals) == 0 {
		return
	}
	for _, reason := range refusals {
		slog.Error("refusing to start", "reason", reason)
	}
	os.Exit(1)
}

func llmFromEnv() *llmclient.Client {
	llmURL := os.Getenv("LLM_SERVICE_URL")
	if llmURL == "" {
		slog.Warn("LLM_SERVICE_URL not set; search will use FTS-only fallback and imports will not be enriched")
		return nil
	}
	token := os.Getenv("LLM_SERVICE_TOKEN")
	if token == "" {
		slog.Error("LLM_SERVICE_TOKEN is required when LLM_SERVICE_URL is set")
		os.Exit(1)
	}
	llm := wiring.LLMClient(llmURL, token)
	slog.Info("llm service configured", "url", llmURL)
	return llm
}

func traceSignerFromEnv() *trace.Signer {
	traceSigner := &trace.Signer{Secret: []byte(os.Getenv("SKILLHUB_TRACE_INGEST_SECRET"))}
	if !traceSigner.Enabled() {
		slog.Warn("SKILLHUB_TRACE_INGEST_SECRET not set; run traces will not be collected")
	}
	return traceSigner
}

func packagingProfilesFromEnv() packaging.Profiles {
	profileDir := envx.Or(os.Getenv("PACKAGING_PROFILES_DIR"), "contracts/packaging/profiles")
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
	return profiles
}

func inProcessCreation(set **worker.Set) func(context.Context, creation.JobArgs, *creation.Diagram) error {
	return func(ctx context.Context, a creation.JobArgs, d *creation.Diagram) error {
		if *set == nil {
			return creation.ErrUnavailable
		}
		return (*set).Creation.Step(ctx, a, d)
	}
}

func inProcessWorkerDeps(cfg apiserver.Config, store *objstore.Client) func() worker.Deps {
	return func() worker.Deps {
		return worker.Deps{
			CreationLimits:     cfg.CreationLimits,
			Providers:          cfg.Providers,
			Store:              store,
			Gateway:            wiring.GatewayFromEnv(),
			RunDeployment:      cfg.RunDeployment,
			TraceSigner:        cfg.TraceSigner,
			TraceIngestBaseURL: os.Getenv("SKILLHUB_TRACE_INGEST_URL"),
			LLM:                cfg.LLM,
			PollOnly:           true,
		}
	}
}

func startCleanWorker(ctx context.Context, pool *pgxpool.Pool, deps func() worker.Deps) *worker.Set {
	exitOn(queue.EnsureSchema(ctx, pool), "clean mode: queue schema")
	set, err := worker.BuildWorkers(pool, deps())
	exitOn(err, "clean mode: worker composition")
	exitOn(set.Queue.Start(ctx), "clean mode: queue start")
	slog.Info("clean mode: worker started in-process")
	return set
}

func cleanModeServing(handler http.Handler, mode deployment, posture envx.Posture) http.Handler {
	static, err := cleanModeStaticHandler(posture)
	exitOn(err, "clean mode: web assets")
	slog.Info("clean mode: serving the web build with the 02:PORT-003 disclosure flag injected")
	return cleanModeHandler(handler, mode, static)
}

func serveUntilStopped(ctx context.Context, srv *http.Server) (failed bool) {
	serveErr := make(chan error, 1)
	go func() {
		slog.Info("api listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()
	select {
	case <-ctx.Done():
		return false
	case err := <-serveErr:
		slog.Error("api stopped", "error", err)
		return true
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

func rateLimitsFromEnv() (*httpx.RateLimiter, error) {
	if strings.EqualFold(os.Getenv("RATE_LIMIT"), "off") {
		slog.Warn("RATE_LIMIT=off; anonymous search and the import endpoints have no rate limit (02:NFR-001 clause 5)")
		return nil, nil
	}
	trusted, err := httpx.ParseTrustedProxies(os.Getenv("TRUSTED_PROXIES"))
	if err != nil {
		return nil, err
	}
	return httpx.NewRateLimiter(rateLimitPerMinute, rateLimitBurst).TrustProxies(trusted), nil
}

const (
	rateLimitPerMinute = 60
	rateLimitBurst     = 30
)

func generateExposedFromEnv() bool {
	raw := os.Getenv("GENERATE_SKILL_EXPOSED")
	switch {
	case strings.EqualFold(raw, "on"):
		slog.Warn("GENERATE_SKILL_EXPOSED=on; the M5 generation entry point is visible. " +
			"01 §11.2's first funnel segment must have a reading first")
		return true
	case raw != "" && !strings.EqualFold(raw, "off"):

		slog.Warn("GENERATE_SKILL_EXPOSED is neither `on` nor `off`; the M5 generation entry point stays hidden",
			"value", raw)
	}
	return false
}

func publicationDownloadsOpenFromEnv() bool {
	raw := os.Getenv("PUBLICATION_DOWNLOADS_UNINVITED")
	switch {
	case strings.EqualFold(raw, "on"):
		slog.Warn("PUBLICATION_DOWNLOADS_UNINVITED=on; accounts outside the beta roster can download publications")
		return true
	case raw != "" && !strings.EqualFold(raw, "off"):
		slog.Warn("PUBLICATION_DOWNLOADS_UNINVITED is neither `on` nor `off`; publication downloads stay invite-only",
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
