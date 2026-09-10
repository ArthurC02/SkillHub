package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/apiserver"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func setenv(t *testing.T, key, value string, unset bool) {
	t.Helper()
	t.Setenv(key, value)
	if unset {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
	}
}

func TestGenerateExposedFromEnv(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		unset bool
		want  bool
	}{
		{name: "unset (the shipped default)", unset: true},
		{name: "empty", value: ""},
		{name: "off", value: "off"},
		{name: "OFF", value: "OFF"},

		{name: "false", value: "false"},
		{name: "0", value: "0"},
		{name: "true", value: "true"},
		{name: "1", value: "1"},

		{name: "on", value: "on", want: true},
		{name: "ON", value: "ON", want: true},
		{name: "oN", value: "oN", want: true},

		{name: "padded on", value: " on "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setenv(t, "GENERATE_SKILL_EXPOSED", tc.value, tc.unset)
			if got := generateExposedFromEnv(); got != tc.want {
				t.Errorf("GENERATE_SKILL_EXPOSED=%q exposes the generation entry point: %v, want %v",
					tc.value, got, tc.want)
			}
		})
	}
}

func TestRateLimitsFromEnv(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   string
		unset   bool
		limited bool
	}{
		{name: "unset", unset: true, limited: true},
		{name: "empty", value: "", limited: true},
		{name: "off", value: "off"},
		{name: "OFF", value: "OFF"},
		{name: "malformed", value: "no", limited: true},
		{name: "0", value: "0", limited: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setenv(t, "RATE_LIMIT", tc.value, tc.unset)
			if got := rateLimitsFromEnv() != nil; got != tc.limited {
				t.Errorf("RATE_LIMIT=%q leaves anonymous search and the import endpoints rate limited: %v, want %v",
					tc.value, got, tc.limited)
			}
		})
	}
}

func TestQuotaFromEnv(t *testing.T) {
	enforced := policy.DefaultQuotaLimits()
	if enforced == (policy.QuotaLimits{}) {
		t.Fatal("policy.DefaultQuotaLimits() is the zero value; there is no allowance to enforce")
	}
	for _, tc := range []struct {
		name  string
		value string
		unset bool
		want  policy.QuotaLimits
	}{
		{name: "unset", unset: true, want: enforced},
		{name: "empty", value: "", want: enforced},
		{name: "off", value: "off"},
		{name: "OFF", value: "OFF"},
		{name: "malformed", value: "disabled", want: enforced},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setenv(t, "RUN_QUOTA", tc.value, tc.unset)
			if got := quotaFromEnv(); got != tc.want {
				t.Errorf("RUN_QUOTA=%q gives %+v, want %+v", tc.value, got, tc.want)
			}
		})
	}
}

func TestGenerateQuotaFromEnv(t *testing.T) {
	enforced := policy.DefaultGenerateQuotaLimits()
	if enforced == (policy.QuotaLimits{}) {
		t.Fatal("policy.DefaultGenerateQuotaLimits() is the zero value; there is no allowance to enforce")
	}
	for _, tc := range []struct {
		name  string
		value string
		unset bool
		want  policy.QuotaLimits
	}{
		{name: "unset", unset: true, want: enforced},
		{name: "empty", value: "", want: enforced},
		{name: "off", value: "off"},
		{name: "OFF", value: "OFF"},
		{name: "malformed", value: "disabled", want: enforced},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setenv(t, "GENERATE_QUOTA", tc.value, tc.unset)
			if got := generateQuotaFromEnv(); got != tc.want {
				t.Errorf("GENERATE_QUOTA=%q gives %+v, want %+v", tc.value, got, tc.want)
			}
		})
	}
}

func TestTheTwoAllowancesHaveSeparateSwitches(t *testing.T) {
	setenv(t, "RUN_QUOTA", "off", false)
	setenv(t, "GENERATE_QUOTA", "", true)
	if quotaFromEnv() != (policy.QuotaLimits{}) {
		t.Error("RUN_QUOTA=off did not turn the run allowance off")
	}
	if generateQuotaFromEnv() == (policy.QuotaLimits{}) {
		t.Error("RUN_QUOTA=off also turned off the generation allowance")
	}

	setenv(t, "RUN_QUOTA", "", true)
	setenv(t, "GENERATE_QUOTA", "off", false)
	if quotaFromEnv() == (policy.QuotaLimits{}) {
		t.Error("GENERATE_QUOTA=off also turned off the run allowance")
	}
	if generateQuotaFromEnv() != (policy.QuotaLimits{}) {
		t.Error("GENERATE_QUOTA=off did not turn the generation allowance off")
	}
}

func TestImportFetcherAllowsInsecureOnlyWhenAskedTo(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		unset bool
		want  bool
	}{
		{name: "unset", unset: true},
		{name: "empty", value: ""},
		{name: "0", value: "0"},
		{name: "true", value: "true"},
		{name: "yes", value: "yes"},
		{name: "1", value: "1", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setenv(t, "IMPORT_ALLOW_INSECURE", tc.value, tc.unset)
			setenv(t, "IMPORT_EXTRA_HOSTS", "", true)
			if got := importFetcherFromEnv().AllowInsecure; got != tc.want {
				t.Errorf("IMPORT_ALLOW_INSECURE=%q allows plain http: %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestImportFetcherHostsFromEnv(t *testing.T) {
	setenv(t, "IMPORT_ALLOW_INSECURE", "", true)
	setenv(t, "IMPORT_EXTRA_HOSTS", " Files.Example.Com , ,gitlab.example.com ", false)
	allowed := importFetcherFromEnv().Allowed
	for host := range ingest.DefaultAllowedHosts() {
		if !allowed[host] {
			t.Errorf("IMPORT_EXTRA_HOSTS replaced the default host %q instead of adding to it", host)
		}
	}
	for _, host := range []string{"files.example.com", "gitlab.example.com"} {
		if !allowed[host] {
			t.Errorf("extra host %q was not allowed (case-folded, trimmed)", host)
		}
	}
	if allowed[""] {
		t.Error("the empty element of IMPORT_EXTRA_HOSTS became an allowed host")
	}
}

func TestCleanModeFromEnv(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		unset bool
		want  bool
	}{
		{name: "unset (the shipped default)", unset: true},
		{name: "empty", value: ""},
		{name: "0", value: "0"},
		{name: "true", value: "true"},
		{name: "TRUE", value: "TRUE"},
		{name: "1", value: "1", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setenv(t, "SKILLHUB_CLEAN_MODE", tc.value, tc.unset)
			if got := cleanModeFromEnv(); got != tc.want {
				t.Errorf("SKILLHUB_CLEAN_MODE=%q -> clean mode %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestApplyCleanModePoolLeavesProductionAlone(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://skillhub@127.0.0.1:1/skillhub")
	if err != nil {
		t.Fatalf("pgxpool.ParseConfig: %v", err)
	}
	before := cfg.MaxConns

	applyCleanModePool(cfg, false)
	if cfg.MaxConns != before {
		t.Errorf("clean=false changed MaxConns from %d to %d; the flag being unset must not touch the pool config", before, cfg.MaxConns)
	}

	applyCleanModePool(cfg, true)
	if cfg.MaxConns != 1 {
		t.Errorf("clean=true left MaxConns at %d, want 1 (a single PGlite-backed connection, ADR-060 決策 6)", cfg.MaxConns)
	}
}

func TestNewStoreTakesFromEnvPathWhenNotClean(t *testing.T) {
	store, stopFn, err := newStore(false)
	if err != nil {
		t.Fatalf("newStore(false): %v", err)
	}
	if store == nil {
		t.Error("newStore(false) returned a nil store")
	}
	if stopFn != nil {
		t.Error("newStore(false) returned a non-nil stop func; that only happens on the in-process path, so clean=false took the wrong branch")
	}
}

func TestNewStoreTakesInProcessPathWhenClean(t *testing.T) {
	store, stopFn, err := newStore(true)
	if err != nil {
		t.Fatalf("newStore(true): %v", err)
	}
	if store == nil {
		t.Fatal("newStore(true) returned a nil store")
	}
	if stopFn == nil {
		t.Fatal("newStore(true) returned a nil stop func; the in-process backend it should have started is now unreachable to shut down")
	}
	stopFn()
}

func writeCleanModeFixture(t *testing.T, indexBody string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(indexBody), 0o644); err != nil {
		t.Fatalf("write index.html: %v", err)
	}
	assetsDir := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "app.js"), []byte("console.log('fixture')"), 0o644); err != nil {
		t.Fatalf("write assets/app.js: %v", err)
	}
	return dir
}

func TestWebStaticHandlerUnderInjectsTheFlag(t *testing.T) {
	dir := writeCleanModeFixture(t, "<html><head><title>t</title>\n<!--SKILLHUB_CLEAN_MODE_FLAG-->\n</head><body></body></html>")

	handler, err := webStaticHandlerUnder(dir, false)
	if err != nil {
		t.Fatalf("webStaticHandlerUnder: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "SKILLHUB_CLEAN_MODE_FLAG") {
		t.Error("GET / still carries the raw placeholder; it was not replaced")
	}
	if !strings.Contains(body, "window.__SKILLHUB_CLEAN_MODE__=true") {
		t.Errorf("GET / does not carry the injected flag; body = %q", body)
	}
	if !strings.Contains(body, "<title>t</title>") || !strings.Contains(body, "<body></body>") {
		t.Errorf("injection touched more than the placeholder; body = %q", body)
	}
}

func TestWebStaticHandlerUnderInjectsTheOfflineSignInFlagOnlyWhenTheRouteExists(t *testing.T) {
	const page = "<html><head><!--SKILLHUB_CLEAN_MODE_FLAG--></head><body></body></html>"

	for _, tc := range []struct {
		name     string
		devLogin bool
		want     bool
	}{
		{"DEV_LOGIN off", false, false},
		{"DEV_LOGIN on", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler, err := webStaticHandlerUnder(writeCleanModeFixture(t, page), tc.devLogin)
			if err != nil {
				t.Fatalf("webStaticHandlerUnder: %v", err)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			body := rec.Body.String()

			if got := strings.Contains(body, "window.__SKILLHUB_DEV_LOGIN__=true"); got != tc.want {
				t.Errorf("offline sign-in flag present = %v, want %v; body = %q", got, tc.want, body)
			}

			if strings.Contains(body, "__SKILLHUB_DEV_LOGIN__=false") {
				t.Error("the flag was written as false; it may only ever be written as true")
			}

			if !strings.Contains(body, "window.__SKILLHUB_CLEAN_MODE__=true") {
				t.Errorf("the clean-mode disclosure went missing; body = %q", body)
			}
		})
	}
}

func TestWebStaticHandlerUnderServesAssets(t *testing.T) {
	dir := writeCleanModeFixture(t, "<html><head><!--SKILLHUB_CLEAN_MODE_FLAG--></head></html>")

	handler, err := webStaticHandlerUnder(dir, false)
	if err != nil {
		t.Fatalf("webStaticHandlerUnder: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /assets/app.js: status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "console.log('fixture')" {
		t.Errorf("GET /assets/app.js body = %q, want the fixture file's content", rec.Body.String())
	}
}

func TestWebStaticHandlerUnderNamesTheMissingBuild(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")

	_, err := webStaticHandlerUnder(dir, false)
	if err == nil {
		t.Fatal("webStaticHandlerUnder on a missing directory returned no error")
	}
	wantPath := filepath.Join(dir, "index.html")
	if !strings.Contains(err.Error(), wantPath) {
		t.Errorf("error %q does not name the path it looked for (%s)", err.Error(), wantPath)
	}
}

func TestWebStaticHandlerUnderRequiresThePlaceholder(t *testing.T) {
	dir := writeCleanModeFixture(t, "<html><head><title>no placeholder here</title></head></html>")

	_, err := webStaticHandlerUnder(dir, false)
	if err == nil {
		t.Fatal("webStaticHandlerUnder on an index.html with no placeholder returned no error")
	}
	if !strings.Contains(err.Error(), "placeholder") {
		t.Errorf("error %q does not say what is missing", err.Error())
	}
}

func TestCleanModeHandlerLeavesProductionAlone(t *testing.T) {
	api := http.NewServeMux()
	static := http.NewServeMux()

	got := cleanModeHandler(api, false, static)
	if got != http.Handler(api) {
		t.Error("cleanModeHandler(api, false, static) did not return api unchanged; the flag being unset must not touch the handler")
	}
}

func TestCleanModeHandlerRoutesStaticOnlyWhenClean(t *testing.T) {
	var apiHit, staticHit []string
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiHit = append(apiHit, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	})
	static := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		staticHit = append(staticHit, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	})

	handler := cleanModeHandler(api, true, static)
	for _, path := range []string{"/", "/assets/app.js", "/skills/abc-123", "/me"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	wantStatic := []string{"/", "/assets/app.js"}
	if !slices.Equal(staticHit, wantStatic) {
		t.Errorf("static handler saw %v, want %v", staticHit, wantStatic)
	}
	wantAPI := []string{"/skills/abc-123", "/me"}
	if !slices.Equal(apiHit, wantAPI) {
		t.Errorf("api handler saw %v, want %v", apiHit, wantAPI)
	}
}

func TestBackgroundLoopsWatchTheReconciler(t *testing.T) {

	pool, err := pgxpool.New(context.Background(), "postgres://skillhub@127.0.0.1:1/skillhub")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	app, err := apiserver.NewApp(apiserver.Config{Pool: pool, Secure: true})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}

	got := make([]string, 0, 1)
	for _, loop := range backgroundLoops(app) {
		got = append(got, loopName(loop))
	}
	want := []string{loopName((&run.Service{}).WatchReconciler)}
	if !slices.Equal(got, want) {
		t.Errorf("the API's background loops are %v, want %v", got, want)
	}
}

func loopName(f func(context.Context)) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}

func TestDevLoginRefusal(t *testing.T) {
	for _, tc := range []struct {
		name            string
		devLogin, https bool
		refuses         bool
	}{
		{name: "production: neither", devLogin: false, https: true},
		{name: "local dev: dev login on plain http", devLogin: true, https: false},
		{name: "dev login with secure cookies", devLogin: true, https: true, refuses: true},
		{name: "no dev login on plain http", devLogin: false, https: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reason := devLoginRefusal(tc.devLogin, tc.https)
			if (reason != "") != tc.refuses {
				t.Fatalf("devLoginRefusal(%v, %v) = %q, want refusal=%v", tc.devLogin, tc.https, reason, tc.refuses)
			}
			if tc.refuses && !strings.Contains(reason, "COOKIE_INSECURE") {
				t.Errorf("the refusal does not say how to resolve it: %q", reason)
			}
		})
	}
}

func TestStartupTasksAuditTheRosters(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://skillhub@127.0.0.1:1/skillhub")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	app, err := apiserver.NewApp(apiserver.Config{Pool: pool, Secure: true})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}

	got := make([]string, 0, 1)
	for _, task := range startupTasks(app) {
		got = append(got, loopName(task))
	}
	want := []string{loopName((&apiserver.App{}).AuditRosters)}
	if !slices.Equal(got, want) {
		t.Errorf("the API's start-up tasks are %v, want %v", got, want)
	}
}

func TestCleanModeFallsBackToTheSPAOnlyForUnroutedBrowserGets(t *testing.T) {

	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/me":

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"api":true}`))
		case r.URL.Path == "/auth/github/callback":

			w.Header().Set("Location", "/")
			w.WriteHeader(http.StatusFound)
		case r.URL.Path == "/auth/github/callback/fail":

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"oauth state mismatch"}`))
		case r.URL.Path == "/runs/real-id":

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"run":"real-id"}`))
		case r.URL.Path == "/downloads/pkg":

			w.Header().Set("Content-Type", "application/zip")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("PK\x03\x04zipbytes"))
		case r.URL.Path == "/skills/abc-123" && r.Method == http.MethodGet:

			w.Header().Set("Allow", "DELETE")
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte("Method Not Allowed\n"))
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	})
	static := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html>index for " + r.URL.Path + "</html>"))
	})
	handler := cleanModeHandler(api, true, static)

	for _, tc := range []struct {
		name, method, path, accept string
		wantCode                   int
		wantHTML                   bool
	}{
		{

			name: "a pasted deep link loads the app", method: http.MethodGet,
			path: "/skills/abc-123", accept: "text/html,application/xhtml+xml",
			wantCode: http.StatusOK, wantHTML: true,
		},
		{

			name: "refreshing a page whose address is also an API resource", method: http.MethodGet,
			path: "/runs/real-id", accept: "text/html,application/xhtml+xml",
			wantCode: http.StatusOK, wantHTML: true,
		},
		{

			name: "the OAuth callback still reaches the API", method: http.MethodGet,
			path: "/auth/github/callback", accept: "text/html",
			wantCode: http.StatusFound,
		},
		{

			name: "a failed OAuth callback still says why", method: http.MethodGet,
			path: "/auth/github/callback/fail", accept: "text/html",
			wantCode: http.StatusUnauthorized,
		},
		{

			name: "a download's bytes still reach the browser", method: http.MethodGet,
			path: "/downloads/pkg", accept: "text/html,application/xhtml+xml",
			wantCode: http.StatusOK,
		},
		{

			name: "a fetch for a missing resource still gets 404", method: http.MethodGet,
			path: "/skills/no-such-skill", accept: "application/json",
			wantCode: http.StatusNotFound,
		},
		{
			name: "a write is never answered with a page", method: http.MethodPost,
			path: "/skills/abc-123", accept: "text/html",
			wantCode: http.StatusNotFound,
		},
		{
			name: "an API route the caller can reach is untouched", method: http.MethodGet,
			path: "/me", accept: "application/json",
			wantCode: http.StatusOK,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Header.Set("Accept", tc.accept)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("%s %s -> %d, want %d", tc.method, tc.path, rec.Code, tc.wantCode)
			}
			isHTML := strings.Contains(rec.Header().Get("Content-Type"), "text/html")
			if isHTML != tc.wantHTML {
				t.Errorf("%s %s answered Content-Type %q; want HTML=%v",
					tc.method, tc.path, rec.Header().Get("Content-Type"), tc.wantHTML)
			}
			if tc.wantHTML && !strings.Contains(rec.Body.String(), "index for /") {

				t.Errorf("the fallback served %q, want index.html", rec.Body.String())
			}
			if !tc.wantHTML && strings.Contains(rec.Body.String(), "<html>") {
				t.Errorf("a non-browser caller was handed a page: %q", rec.Body.String())
			}
		})
	}
}

func TestTheTwoWaysPackagingHasNoTargetsAreToldApart(t *testing.T) {
	missing := profileDirReason(filepath.Join(t.TempDir(), "no-such-dir"))
	if !strings.Contains(missing, "does not exist") {
		t.Errorf("a missing directory must say so, got %q", missing)
	}

	if !strings.Contains(missing, "working directory") {
		t.Errorf("the missing-directory reason no longer explains how a relative path resolves, got %q", missing)
	}

	empty := profileDirReason(t.TempDir())
	if !strings.Contains(empty, "holds no") {
		t.Errorf("an existing but empty directory must say so, got %q", empty)
	}
	if missing == empty {
		t.Error("both causes produce the same sentence again, which is the defect this test exists for")
	}
}

func TestCleanModeEmptiesTheSessionItInheritsOnConnect(t *testing.T) {
	dsn := os.Getenv("SKILLHUB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SKILLHUB_TEST_DATABASE_URL not set; skipping the database-backed half")
	}
	ctx := context.Background()

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("pgxpool.ParseConfig: %v", err)
	}
	applyCleanModePool(cfg, false)
	if cfg.AfterConnect != nil {
		t.Fatal("clean=false installed an AfterConnect hook; the flag being unset must not touch the pool config (02:PORT-005)")
	}
	applyCleanModePool(cfg, true)
	if cfg.AfterConnect == nil {
		t.Fatal("clean=true left AfterConnect nil, so a retired connection's prepared statements survive into the next one and wedge the carrier")
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pgxpool.NewWithConfig: %v", err)
	}
	defer pool.Close()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer conn.Release()

	count := func(where string) int {
		var n int
		row := conn.QueryRow(ctx, "SELECT count(*)::int FROM pg_prepared_statements", pgx.QueryExecModeSimpleProtocol)
		if err := row.Scan(&n); err != nil {
			t.Fatalf("count prepared statements (%s): %v", where, err)
		}
		return n
	}

	if _, err := conn.Exec(ctx, "PREPARE skillhub_reconnect_probe AS SELECT 1"); err != nil {
		t.Fatalf("dirty the session: %v", err)
	}
	if n := count("after dirtying"); n == 0 {
		t.Fatal("the probe left no prepared statement behind, so this test is not measuring what it claims to")
	}

	if err := cfg.AfterConnect(ctx, conn.Conn()); err != nil {
		t.Fatalf("AfterConnect on an inherited session: %v", err)
	}
	if n := count("after AfterConnect"); n != 0 {
		t.Errorf("the session still carries %d prepared statement(s) after AfterConnect; the next connection will collide with them and take the carrier down with it", n)
	}
}

func TestCleanModeSurvivesAQueryErrorInsteadOfDyingOfOne(t *testing.T) {
	dsn := os.Getenv("SKILLHUB_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SKILLHUB_TEST_DATABASE_URL not set; skipping the database-backed half")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("pgxpool.ParseConfig: %v", err)
	}
	applyCleanModePool(cfg, true)
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pgxpool.NewWithConfig: %v", err)
	}
	defer pool.Close()

	alive := func(when string) {
		t.Helper()
		var n int

		if err := pool.QueryRow(ctx, "SELECT $1::int", 7).Scan(&n); err != nil {
			t.Fatalf("%s: the deployment is gone, not just this query: %v", when, err)
		}
		if n != 7 {
			t.Fatalf("%s: SELECT 7 returned %d", when, n)
		}

		var out string
		if err := pool.QueryRow(ctx, "SELECT ($1::jsonb)->>'k'", []byte(`{"k":"v"}`)).Scan(&out); err != nil {
			t.Fatalf("%s: a jsonb parameter did not survive the query mode: %v", when, err)
		}
		if out != "v" {
			t.Fatalf("%s: jsonb round trip returned %q, want \"v\"", when, out)
		}
	}

	alive("before the error")

	var s string
	if err := pool.QueryRow(ctx, "SELECT $1::text", string([]byte{0xa7, 'A'})).Scan(&s); err == nil {
		t.Fatal("PostgreSQL accepted invalid UTF-8; this test is not producing the error it is named for")
	}

	alive("after one query error")
	alive("after one query error, second call")
}

func TestWebStaticHandlerUnderSendsAPolicyNoRemoteImageCanCross(t *testing.T) {
	dir := writeCleanModeFixture(t, "<html><head><!--SKILLHUB_CLEAN_MODE_FLAG--></head><body></body></html>")

	handler, err := webStaticHandlerUnder(dir, false)
	if err != nil {
		t.Fatalf("webStaticHandlerUnder: %v", err)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	policy := rec.Header().Get("Content-Security-Policy")
	if policy == "" {
		t.Fatal("the document went out with no Content-Security-Policy")
	}
	for _, directive := range contentSecurityPolicyBase {
		if !strings.Contains(policy, directive) {
			t.Errorf("policy is missing %q; policy = %q", directive, policy)
		}
	}

	if !strings.Contains(policy, "img-src 'self' data: blob:;") {
		t.Errorf("img-src does not read exactly 'self' data: blob:; policy = %q", policy)
	}
	if strings.Contains(policy, "img-src") && strings.Contains(policy, "https:") {
		t.Errorf("some directive admits a remote scheme; policy = %q", policy)
	}
	if strings.Contains(policy, "*") {
		t.Errorf("policy carries a wildcard source; policy = %q", policy)
	}
}

func TestWebStaticHandlerUnderHashesEveryScriptItInjected(t *testing.T) {
	const page = "<html><head><!--SKILLHUB_CLEAN_MODE_FLAG--></head><body></body></html>"
	inline := regexp.MustCompile(`(?s)<script>(.*?)</script>`)

	for _, tc := range []struct {
		name      string
		devLogin  bool
		wantCount int
	}{
		{"DEV_LOGIN off", false, 1},
		{"DEV_LOGIN on", true, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler, err := webStaticHandlerUnder(writeCleanModeFixture(t, page), tc.devLogin)
			if err != nil {
				t.Fatalf("webStaticHandlerUnder: %v", err)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			policy := rec.Header().Get("Content-Security-Policy")

			bodies := inline.FindAllStringSubmatch(rec.Body.String(), -1)
			if len(bodies) != tc.wantCount {
				t.Fatalf("served %d inline scripts, want %d; this test is not measuring what it names", len(bodies), tc.wantCount)
			}
			for _, m := range bodies {
				sum := sha256.Sum256([]byte(m[1]))
				want := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
				if !strings.Contains(policy, want) {
					t.Errorf("the policy would block the injected script %q (no %s in %q)", m[1], want, policy)
				}
			}

			if strings.Contains(policy, "unsafe-inline") || strings.Contains(policy, "unsafe-eval") {
				t.Errorf("the policy waves inline script through instead of naming it; policy = %q", policy)
			}

			sum := sha256.Sum256([]byte(devLoginFlagJS))
			named := strings.Contains(policy, base64.StdEncoding.EncodeToString(sum[:]))
			if named != tc.devLogin {
				t.Errorf("dev-login hash present = %v, want %v; policy = %q", named, tc.devLogin, policy)
			}
		})
	}
}

func TestNginxServesTheSamePolicyAsCleanMode(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no source path for this test file")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
	confPath := filepath.Join(repoRoot, "infra", "images", "web", "nginx.conf")
	raw, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("read %s: %v", confPath, err)
	}
	conf := string(raw)

	line := ""
	for _, l := range strings.Split(conf, "\n") {
		if strings.Contains(l, "add_header Content-Security-Policy") {
			line = l
		}
	}
	if line == "" {
		t.Fatal("infra/images/web/nginx.conf sends no Content-Security-Policy; the deployed product has none")
	}
	for _, directive := range contentSecurityPolicyBase {
		if !strings.Contains(line, directive) {
			t.Errorf("nginx.conf is missing %q; its policy line is %s", directive, strings.TrimSpace(line))
		}
	}

	if !strings.Contains(line, "script-src 'self'") || strings.Contains(line, "unsafe-inline") {
		t.Errorf("nginx.conf's script-src is not a plain 'self'; its policy line is %s", strings.TrimSpace(line))
	}

	if !strings.Contains(line, "always") {
		t.Errorf("nginx.conf's policy is not marked `always`, so error responses go out without it: %s", strings.TrimSpace(line))
	}
}

func TestNginxDoesNotBufferTheEventStream(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no source path for this test file")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
	raw, err := os.ReadFile(filepath.Join(repoRoot, "infra", "images", "web", "nginx.conf"))
	if err != nil {
		t.Fatalf("read nginx.conf: %v", err)
	}

	conf := string(raw)
	at := strings.Index(conf, "location ~ ^/creation-sessions/")
	if at < 0 {
		t.Fatal("nginx.conf has no location for the creation event stream; the deployed product buffers it")
	}
	end := strings.Index(conf[at:], "\n    }")
	if end < 0 {
		t.Fatal("could not find the end of the stream location block")
	}
	block := conf[at : at+end]

	for _, want := range []string{
		"proxy_buffering off",

		"proxy_http_version 1.1",
		"proxy_pass",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("the stream location is missing %q:\n%s", want, block)
		}
	}

	if !strings.Contains(block, "proxy_read_timeout") {
		t.Errorf("the stream location sets no proxy_read_timeout, so nginx's 60s default cuts a quiet stream:\n%s", block)
	}
}
