package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/apiserver"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

// This process reads its whole deployment from the environment, and until now
// nothing executed a line of it: every test that cares about an allowance, a
// limiter or the exposure flag sets the assembled field directly, which
// exercises the enforcement and never the configuration. The functions below
// are where "unset" acquires its meaning, and the meanings disagree on purpose
// — an allowance left unconfigured is enforced, an entry point left
// unconfigured is hidden, a retention left unconfigured collects nothing — so
// each one is pinned here rather than inferred from its neighbours.
//
// No database and no network: everything under test is a string and a switch.

// setenv is t.Setenv with an "unset" case. t.Setenv restores the previous state
// (including having had none) on cleanup either way, so unsetting through it is
// safe and is the only way to test the shipped default on a machine that
// happens to export the variable.
func setenv(t *testing.T, key, value string, unset bool) {
	t.Helper()
	t.Setenv(key, value)
	if unset {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
	}
}

// --- 1. ADR-052's exposure flag ----------------------------------------------

// The whole truth table of GENERATE_SKILL_EXPOSED, because only one reading of
// it is safe and every other plausible one opens the M5 generation entry point
// on a deployment that never asked for it (⛔ boundary 1, 01 §10). The mutation
// this exists for is the tidy-up that harmonises this with RATE_LIMIT and
// RUN_QUOTA — `!EqualFold(raw, "off")` — which is correct for an allowance and
// backwards for an entry point.
//
// Only the returned bool is asserted. The warning next to it is deliberately not
// matched on: a message somebody rewords is not a regression, and a test that
// says otherwise gets edited out the first time it lies.
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
		// The four values somebody reaches for when they mean "on" and that this
		// flag does not accept. They stay hidden — noisily, but hidden.
		{name: "false", value: "false"},
		{name: "0", value: "0"},
		{name: "true", value: "true"},
		{name: "1", value: "1"},
		// The one accepted spelling, in any case.
		{name: "on", value: "on", want: true},
		{name: "ON", value: "ON", want: true},
		{name: "oN", value: "oN", want: true},
		// Not trimmed, and that is the conservative direction: a padded value is
		// a value nobody typed on purpose.
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

// --- 2. the ceilings ---------------------------------------------------------

// NFR-001 clause 5's limiter. Unset is enforced, `off` is the escape hatch, and
// anything else is enforced too — a protection left unconfigured must not
// silently be absent, and a typo in the off switch is exactly "unconfigured".
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

// PDM-010's free run allowance (ADR-028 決策 2) — the platform's only cost
// ceiling, so unset means enforced with the package defaults and only the
// written-down `off` turns it off.
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

// GEN-004's generation allowance (ADR-047 決策 5). A second variable and not a
// shared one, which is the half of ADR-055's lesson a test can hold: turning off
// one allowance must not turn off the other.
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

// RUN_QUOTA=off must not take the generation allowance with it, and the reverse.
// The two are counted separately on purpose (ADR-047 決策 5 ruled against a
// shared pool), and one switch that moved both would be that shared pool in
// different clothes.
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

// The URL-import fetcher. AllowInsecure gets its own assertion because it is the
// one setting here whose wrong default reaches the network: plain http to an
// attacker-positioned host, on a deployment that configured nothing.
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

// The host allow list: the defaults are always there, extras are added rather
// than replacing them, and the parsing is the same trim-and-drop-empties the
// operator roster uses.
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

// --- X. SKILLHUB_CLEAN_MODE (ADR-060 決策 6) ----------------------------------
//
// One flag, one branch, one choice point — so what needs pinning is exactly
// two things: the flag reads as expected, and the flag *off* reproduces
// today's behaviour bit for bit. That second half is 02:PORT-005's literal
// acceptance test; the mutation this whole section exists for is someone
// widening the branch's condition (e.g. defaulting to clean) or hard-coding
// its consequence instead of gating it on `clean`.

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
		{name: "true", value: "true"}, // not accepted; only the literal "1" is
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

// The literal 02:PORT-005 acceptance test: flag unset must leave the pool
// config exactly where pgxpool.ParseConfig itself put it — not pinned to a
// particular number (that number is pgx's default and not this file's to
// own), just untouched by this file.
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

// The other half of the literal acceptance test: flag unset takes the
// objstore.FromEnv path, not objstore.NewInProcess. The two *objstore.Client
// values have no exported field a test can compare, so this checks the one
// externally visible difference between the two paths instead: NewInProcess
// starts a server and hands back a non-nil stop func to shut it down again,
// FromEnv touches no network and has nothing to stop.
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

// --- X.5 the web build overlay (02:PORT-003 anonymous disclosure) -----------
//
// `GET /me`'s features.clean_mode never reached a signed-out visitor —
// RequireSession answers 401 before the flag is ever read, and `/` and
// `/skills/$id` are both reachable signed out. webStaticHandlerUnder and
// cleanModeHandler are what let cmd/api serve the flag on the response
// itself, so what needs pinning mirrors the section above: the injection
// happens and is reachable, the missing-build and missing-placeholder cases
// name what is wrong (02:PORT-005), and — the acceptance test for this whole
// axis — clean=false leaves the API's own handler completely untouched.

// writeCleanModeFixture lays out a directory shaped like apps/web/dist: an
// index.html carrying the placeholder plus caller-supplied padding (to prove
// the replacement only touches the placeholder bytes, nothing around them),
// and one static asset under assets/.
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

// The literal injection: the placeholder is gone, the script (and nothing
// but the script) is there in its place, and the surrounding bytes this test
// put around the placeholder on purpose are untouched.
func TestWebStaticHandlerUnderInjectsTheFlag(t *testing.T) {
	dir := writeCleanModeFixture(t, "<html><head><title>t</title>\n<!--SKILLHUB_CLEAN_MODE_FLAG-->\n</head><body></body></html>")

	handler, err := webStaticHandlerUnder(dir)
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

// The one route this handler adds beyond "/": Vite's build output directory.
func TestWebStaticHandlerUnderServesAssets(t *testing.T) {
	dir := writeCleanModeFixture(t, "<html><head><!--SKILLHUB_CLEAN_MODE_FLAG--></head></html>")

	handler, err := webStaticHandlerUnder(dir)
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

// 02:PORT-005: a missing build must name the path it looked for, not just say
// "failed" — this is the case a deployment hits when nobody ran the web build
// before setting SKILLHUB_CLEAN_MODE=1.
func TestWebStaticHandlerUnderNamesTheMissingBuild(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")

	_, err := webStaticHandlerUnder(dir)
	if err == nil {
		t.Fatal("webStaticHandlerUnder on a missing directory returned no error")
	}
	wantPath := filepath.Join(dir, "index.html")
	if !strings.Contains(err.Error(), wantPath) {
		t.Errorf("error %q does not name the path it looked for (%s)", err.Error(), wantPath)
	}
}

// A build with no placeholder is the case a stale or hand-edited index.html
// produces: the response would be served with no way to ever carry the flag,
// silently failing the exact thing this handler exists for.
func TestWebStaticHandlerUnderRequiresThePlaceholder(t *testing.T) {
	dir := writeCleanModeFixture(t, "<html><head><title>no placeholder here</title></head></html>")

	_, err := webStaticHandlerUnder(dir)
	if err == nil {
		t.Fatal("webStaticHandlerUnder on an index.html with no placeholder returned no error")
	}
	if !strings.Contains(err.Error(), "placeholder") {
		t.Errorf("error %q does not say what is missing", err.Error())
	}
}

// The 02:PORT-005 acceptance test for this axis: clean=false must return api
// completely untouched, not merely "behaving the same" — a same-origin
// pointer comparison is what a mutation that wraps unconditionally cannot
// pass, the way TestApplyCleanModePoolLeavesProductionAlone pins the pool.
func TestCleanModeHandlerLeavesProductionAlone(t *testing.T) {
	api := http.NewServeMux()
	static := http.NewServeMux()

	got := cleanModeHandler(api, false, static)
	if got != http.Handler(api) {
		t.Error("cleanModeHandler(api, false, static) did not return api unchanged; the flag being unset must not touch the handler")
	}
}

// clean=true must route the two static paths to static and leave everything
// else — including a path an anonymous deep link could hit, like /skills/{id}
// — going to api, per cleanModeHandler's doc comment on the collision
// DEV_CORS_ORIGIN's comment in main() also describes.
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

// --- 4. the background loops -------------------------------------------------

// 03:SEC-012's automatic first action: the reconciler-stall watchdog, which is
// the one P1 criterion of 02:SEC-010 nothing inside the worker can report (a
// dead worker takes every watchdog running inside it along with it). Its
// detection logic has a test of its own; what had none was the fact that this
// process starts it at all.
//
// Asserted by name rather than by behaviour, because there is no honest way to
// assert a running ticker without a clock: what can go wrong here is the loop
// disappearing from the roster, not the loop being wrong.
func TestBackgroundLoopsWatchTheReconciler(t *testing.T) {
	// A parseable DSN nothing listens on; pgxpool connects lazily and NewApp's
	// only I/O is a queue client that does not dial (see apiserver/app_test.go).
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

// loopName is the qualified name of the function a method value wraps, which is
// the same for every receiver — so the expectation above is written as a method
// expression the compiler checks, not as a string somebody keeps in step.
func loopName(f func(context.Context)) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}
