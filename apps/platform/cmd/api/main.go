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
	"bytes"
	"context"
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

// SKILLHUB_CLEAN_MODE is the one flag ADR-060 決策 6 grants this process, read
// exactly once, right here — everything it changes is downstream of this
// single bool, and nothing downstream reads the variable itself (⛔ single
// choice point). Unset (the shipped default) must leave every line below
// behaving exactly as it did before this flag existed; that is 02:PORT-005's
// literal acceptance test, in main_test.go.
func cleanModeFromEnv() bool {
	return os.Getenv("SKILLHUB_CLEAN_MODE") == "1"
}

// cleanModeFlagPlaceholder marks the exact spot in apps/web/index.html that
// carries the anonymous half of 02:PORT-003. GET /me's features.clean_mode
// (ADR-052's flag mechanism, already read by useCleanMode in
// apps/web/src/api/me.ts) requires a session, so / and /skills/$id — both
// reachable signed out — never resolved it. This placeholder is what lets
// cleanModeStaticHandler answer the question before any session exists: it is
// inert HTML everywhere it is not rewritten, which is every build except this
// one flag.
var cleanModeFlagPlaceholder = []byte("<!--SKILLHUB_CLEAN_MODE_FLAG-->")

// cleanModeFlagScript replaces the placeholder above, once, at process
// startup (not per request — see webStaticHandlerUnder). It only ever adds a
// disclosure; it must never carry `false`, because an injected `false` could
// read as "checked and clean" to useCleanMode's fallback onto GET /me and
// quiet a disclosure GET /me would otherwise still show once a session
// exists (⛔ boundary: the flag may only turn the notice on).
var cleanModeFlagScript = []byte(`<script>window.__SKILLHUB_CLEAN_MODE__=true;</script>`)

// devLoginFlagScript is the same mechanism for a different question, and the
// difference matters: clean_mode is a DISCLOSURE (public.yaml says outright that
// a client treating it as something to unlock 「has read it backwards」), while
// this one is an ENTRY POINT in the `generate_skill` sense — it says a route
// exists. They are injected together because they are needed at the same moment,
// by the same signed-out visitor, and answered by nothing else.
//
// Why it had to exist at all: the only sign-in affordance the app has ever had
// is `<a href=…/auth/github/login>使用 GitHub 登入</a>`, in AuthControls and in
// LoginRequired. On the machine 02:PORT-005 is about — no software may be
// installed and the only reachable network is the model provider — that link
// leaves the product and lands on a browser network error. DEV_LOGIN=1 is set by
// tools/cleanmode/start.mjs and POST /auth/dev/login is mounted and working, so
// the offline path existed the whole time; nothing on any screen called it.
// Everything behind a session — 匯入、fork、Test Case、試跑、打包、工作區 — was
// therefore unreachable from the browser, including 04 丙-114's recorded demo
// instruction (「以 seed-importer 的身分展示」), which names an identity only that
// endpoint can produce.
//
// Same ⛔ boundary as the flag above and for a stricter reason: it is written
// only when the route is really mounted, never as `false`. A screen offering a
// sign-in that 404s is worse than one offering none.
var devLoginFlagScript = []byte(`<script>window.__SKILLHUB_DEV_LOGIN__=true;</script>`)

// applyCleanModePool is clean mode's first consequence: a single database
// connection, because the PGlite socket behind it serves one client at a time
// and this same process is about to also run the worker (see the package doc
// on cmd/api and ADR-060 決策 6 for why the two share a pool instead of two).
// Left alone, cfg keeps whatever pgxpool.ParseConfig derived from
// DATABASE_URL on its own — the same value pgxpool.New would have produced,
// which is what main_test.go pins for the flag-unset case.
func applyCleanModePool(cfg *pgxpool.Config, clean bool) {
	if !clean {
		return
	}
	cfg.MaxConns = 1
	// The second consequence of that single connection, and the one that is
	// not a performance note: reconnecting to this carrier is fatal, and
	// pgxpool reconnects on a schedule nobody set.
	//
	// The PGlite socket puts every new TCP connection into the SAME Postgres
	// session (measured: `pg_backend_pid()` is unchanged across a clean
	// Terminate and reconnect). pgx's statement cache is per-connection and
	// starts empty, so the fresh connection prepares names the old one left
	// behind, gets 42P05, and tries to DEALLOCATE its whole cache to recover.
	// That recovery desyncs the pipeline, and from there the carrier answers
	// nobody -- not this pool, not a new process. It is not a stall that
	// clears.
	//
	// pgxpool's MaxConnLifetime defaults to one hour, so an untouched clean
	// mode dies exactly sixty minutes after it starts. Observed 2026-08-31:
	// started 23:49:56, first 42P05 at 00:49:56, everything after it a
	// connection error -- while GET /healthz went on answering 200, because
	// it is a constant (04 丙-110).
	//
	// Raising the lifetime would only move the funeral. Emptying the session
	// on arrival is what makes a reconnect survivable, whenever it happens
	// and whatever triggers it. Exec with no arguments goes over the simple
	// protocol (pgx conn.go: "Always use simple protocol when there are no
	// arguments"), so this statement cannot itself be the one that collides.
	// Against a real PostgreSQL a fresh session has nothing to deallocate and
	// this is a no-op, which is why it is safe to state unconditionally here
	// rather than guessing which reconnects are the dangerous ones.
	//
	// Kept after the exec-mode line below, which stops THIS process from
	// naming statements at all: the session on the other side is shared with
	// whatever else opens the carrier, and arriving in a clean one should not
	// depend on every other client's choice of protocol.
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "DEALLOCATE ALL")
		return err
	}
	// Third consequence, and the one that closes the class rather than a
	// member of it.
	//
	// The recovery above is aimed at a reconnect, but a reconnect is not the
	// only thing that sends pgx down that path. ANY error on a cached
	// statement invalidates the cache entry, and pgx clears it with a
	// PIPELINED Deallocate — which this carrier answers with an unexpected
	// ReadyForQuery, desyncing the protocol exactly as the 42P05 recovery did.
	// The blast radius is identical: the carrier stops answering everyone,
	// permanently.
	//
	// Measured 2026-08-31, and the trigger needs no privilege at all: GET
	// /api/skills/search is anonymous, its `q` reaches PostgreSQL as raw bytes,
	// and one query string that is not valid UTF-8 earns SQLSTATE 22021 —
	// an ordinary, recoverable query error. One second later the deployment was
	// gone. Reproduced in isolation: a healthy pool, one 22021, and the next
	// query fails with "failed to deallocate cached statement(s)" (04 丙-112).
	//
	// The mode below was chosen by measuring all five against this carrier --
	// one fresh carrier each, a jsonb round trip before, one 22021, then three
	// more attempts (2026-08-31):
	//
	//	mode              jsonb   after one query error
	//	cache_statement   ok      carrier dead, permanently   <- the default
	//	cache_describe    ok      clean on the first attempt  <- chosen
	//	describe_exec     ok      next query fails once, then recovers
	//	exec              22P02   survives
	//	simple            22P02   survives
	//
	// Only cache_describe is right in both columns, and the table is here
	// because two of the four wrong answers look right from one angle each.
	//
	// It works for the same reason exec and simple do -- no NAMED server-side
	// statement, so pgx has nothing to invalidate and never sends the pipelined
	// Deallocate this carrier chokes on -- and it stays correct because it
	// still takes its parameter OIDs from a real statement description instead
	// of guessing them from the Go type. That guess is what exec and simple do,
	// and it is why they put every jsonb argument on the wire as text.
	//
	// QueryExecModeExec was the first attempt here and stood for about an hour.
	// It took the operator roster and the feature-flag audit down with 22P02
	// three seconds into a real boot. Two things let it through: this file's own
	// test asserted the pool survives an error while passing nothing but an int
	// -- the one type the guess gets right -- and the other 689 tests in this
	// module build their own pools and never call this function. The mode that
	// fixes the protocol must not change what the protocol carries (04 丙-112).
	//
	// pgx's caveat for this mode is that a cached description goes stale if the
	// schema changes underneath it. Clean mode applies all migrations before
	// this process starts and never migrates while running.
	//
	// The input is fixed at the boundary too (discovery.isComprehensible now
	// refuses bytes that are not text), and that is the fix for the 500. This
	// is the fix for the amplification: the next unhandled error must cost a
	// response, not the deployment.
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe
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

// cleanModeStaticHandler is clean mode's fourth consequence, and the one that
// makes 02:PORT-003's disclosure reachable signed out: it serves apps/web's
// built assets and index.html (with cleanModeFlagScript burned in) from this
// same process, so the flag reaches the browser without a session.
//
// The build's location is derived from this binary's own build path, the way
// cleanModeRunnerScript in apps/sandbox/cmd/sandboxd/main.go derives run.mjs:
// ADR-060 決策 6 forbids a second env var for this axis, and unlike a
// registry reference that varies by deployment, this repo's own layout is
// nothing a node operator configures.
func cleanModeStaticHandler(devLogin bool) (http.Handler, error) {
	distDir, err := webDistDir()
	if err != nil {
		return nil, err
	}
	return webStaticHandlerUnder(distDir, devLogin)
}

// webDistDir locates apps/web/dist from this binary's own build path.
//
// One derivation, two readers: the handler above, which serves the build, and
// the web_app capability probe in capabilities.go, which measures it. A second
// copy of this path arithmetic would be free to drift from the first, and the
// symptom would be a probe reporting a directory nobody serves.
func webDistDir() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("cannot locate the web build: this binary carries no source path, so it was not built from this repository")
	}
	// apps/platform/cmd/api/main.go -> repo root is four directories up.
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
	return filepath.Join(repoRoot, "apps", "web", "dist"), nil
}

// webStaticHandlerUnder is split out so the missing-build and
// missing-placeholder failures are reachable from a test without moving this
// binary to another machine (mirrors sandboxd's runnerScriptUnder for the
// same reason). 02:PORT-005 requires a startup failure to name what is
// missing rather than surface later as a page that never got the flag.
//
// index.html is read and rewritten once, here, rather than per request: the
// file is small and static for the life of the process, and reading it once
// means the handler either exists with the flag already burned in or main()
// has already exited — there is no request-time path where the injection can
// silently not happen.
func webStaticHandlerUnder(distDir string, devLogin bool) (http.Handler, error) {
	indexPath := filepath.Join(distDir, "index.html")
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf(
			"clean mode cannot find the web build at %s (derived from this binary's build path; run `npm --prefix apps/web run build` first): %w",
			indexPath, err)
	}
	flags := cleanModeFlagScript
	if devLogin {
		flags = append(append([]byte{}, flags...), devLoginFlagScript...)
	}
	injected := bytes.Replace(raw, cleanModeFlagPlaceholder, flags, 1)
	if bytes.Equal(injected, raw) {
		return nil, fmt.Errorf(
			"clean mode: %s has no %q placeholder to inject into; 02:PORT-003's disclosure would not reach a signed-out visitor",
			indexPath, cleanModeFlagPlaceholder)
	}

	mux := http.NewServeMux()
	// Vite's default build output; distDir/assets/<hashed file>.
	mux.Handle("GET /assets/", http.FileServer(http.Dir(distDir)))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(injected)
	})
	return mux, nil
}

// cleanModeHandler wires the static overlay above the API's own handler when
// clean is true, and returns api itself — untouched — when it is not. That
// second half is 02:PORT-005's literal acceptance test for this axis, pinned
// by TestCleanModeHandlerLeavesProductionAlone the way applyCleanModePool is
// pinned for the connection pool: a static-serving overlay that only turns on
// when a flag is set and one that is always on look identical in a screenshot.
//
// "GET /" and "GET /assets/" go straight to static. Everything else goes to the
// API first and falls back to the SPA only where the API has no route — see
// spaFallback, which is what makes a pasted deep link like /skills/$id load the
// application instead of answering a browser with JSON.
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

// spaFallback answers a browser navigation the API has no route for with
// index.html, so /skills/$id pasted into an address bar loads the application.
//
// Clean mode is the one deployment shape where nothing else can do this. Its
// whole point is a portable bundle handed to somebody else to open on their own
// machine, and opening things includes pasting a link; there is no reverse proxy
// underneath to push the problem to, because this process IS the deployment.
// Without it a deep link produced a page of JSON — and, worse, no
// window.__SKILLHUB_CLEAN_MODE__, so 02:PORT-003's disclosure never loaded
// either.
//
// The condition is what the API answered, not a copy of the route table. A
// hand-maintained list of API prefixes is exactly what apiserver/router.go's
// package comment records being burned by, and the response already carries
// the answer: 404 (no route), 405 (a route, but no GET), or a JSON body (a
// route that answered with data, which is not what a navigation asked for).
// See navigationCatcher for why those three and no others. Routes that answer
// a navigation properly are untouched, which is what keeps GET
// /auth/github/callback — a browser navigation the API must handle itself —
// working, and what keeps a download's bytes streaming.
//
// Only GET, and only for a caller that asked for text/html. Every client of this
// API is fetch(), which sends */* or application/json; a browser address bar is
// the only thing that says text/html. A 404 for either of the other two stays a
// 404, so a script cannot mistake "no such run" for a page.
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
		// The API's headers (Content-Type: application/json, and whatever the
		// error writer added) were never flushed, so they are still ours to
		// drop before the HTML goes out.
		clear(w.Header())
		index := r.Clone(r.Context())
		index.URL = &url.URL{Path: "/"}
		index.RequestURI = "/"
		static.ServeHTTP(w, index)
	})
}

// navigationCatcher swallows the answers that cannot be what a browser
// navigation asked for, so spaFallback can serve the application instead. It
// only ever sees requests that already passed that gate — GET, Accept:
// text/html — so "what the caller asked for" is a page, and these three
// answers are each a way of not being one:
//
//   - 404: the API has no route. The original case.
//   - 405: the API has the path but no GET. That is /skills/{id}, which
//     spaFallback's own comment names as its motivating example and which it
//     did not actually handle: DELETE /skills/{id} and friends make the mux
//     match the path and refuse the method, so the 404 never happens. Measured
//     2026-08-31 — a pasted or refreshed skill detail link answered
//     `405 method not allowed` in plain text (04 丙-111).
//   - a successful JSON body: the API answered, correctly, with data. GET
//     /runs/{id} is a real route AND the address of the run detail page, so
//     refreshing the Trace screen — the demo's centrepiece — printed the run's
//     JSON into the browser. This is the dangerous one, because it is a
//     success: nothing in the status code says anything went wrong.
//
// Two deliberate narrowings on that last case.
//
// Content-Type, not status, is what separates it from a download: a 2xx that
// is not JSON streams through untouched, which is what keeps the package bytes
// flowing when a browser navigates to a download link (Accept: text/html,
// response application/zip).
//
// 2xx, not any status, is what keeps a failure legible. GET
// /auth/github/callback succeeds with a 302 and fails with a JSON 401 or 500
// ("oauth state mismatch", "login failed"). Swallowing those would turn a
// broken login into a page that loads and quietly does nothing, which is the
// harder fault to diagnose of the two. An error the browser can read is worth
// more than a tidy page.
//
// The decision is made in WriteHeader, where the API's Content-Type is already
// set and no body has been written, so nothing is buffered either way.
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
		return len(b), nil // discarded; the SPA answers instead
	}
	return c.ResponseWriter.Write(b)
}

// devLoginRefusal is why this process will not start with the offline login
// provider mounted, or the empty string when the combination is coherent.
//
// DEV_LOGIN=1 mounts POST /auth/dev/login, where any name is a signed-in account
// with no credential (ADR-020, creator/workspace/http.go). Secure cookies mean
// the deployment is served over https, which is a deployment somebody put a
// certificate on. Wanting both is a contradiction a machine can settle: nobody
// terminates TLS in front of a platform they also want anyone to log into as
// anyone. Three comments say "never in production" and none of them was
// executable; this one is.
//
// COOKIE_INSECURE=1 (the .env.example default for plain-http local dev) is what
// makes Secure false, so the local demo and the E2E suite are untouched.
//
// A function rather than an inline if, so the refusal is reachable from a test
// without exiting the test binary.
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
	// `--capabilities` prints the declared table and exits, before anything is
	// read or dialled. `devctl automation-check` runs this to compare the table
	// against .env.example without standing a deployment up (05 R-36's checker).
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
	profileDir := envx.Or("PACKAGING_PROFILES_DIR", "contracts/packaging/profiles")
	profiles, err := packaging.LoadProfiles(profileDir)
	if err != nil {
		slog.Error("packaging profiles unreadable; packaging is unavailable", "error", err)
		profiles = nil
	}
	if len(profiles) == 0 {
		// Which of the two, and the path as this process resolved it (04 丙-102 ③).
		// The default above is relative, so it resolves from THIS process's working
		// directory — which is not the repository root under `go -C apps/platform
		// run`, and was not on the clean-mode launch measured on 2026-08-30. The
		// operator saw "no packaging profiles configured", which reads as a choice
		// this deployment made, and packaging answered 503 for the rest of the day.
		//
		// The user-facing 503 stays as it is: from a member's side "this deployment
		// cannot package" is true either way, and a filesystem path is not theirs to
		// see. It is the operator who needs the two told apart.
		resolved, absErr := filepath.Abs(profileDir)
		if absErr != nil {
			resolved = profileDir
		}
		slog.Warn("no packaging profiles configured; PACK-001 routes will answer 503",
			"reason", profileDirReason(profileDir), "dir", profileDir, "resolved", resolved)
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

	// ADR-020's offline provider. Both halves are here rather than inside NewApp:
	// whether this deployment may exist at all is the process's question, and the
	// warning is the shape RATE_LIMIT=off, RUN_QUOTA=off and
	// GENERATE_SKILL_EXPOSED=on already use — until now the one switch that
	// bypasses authentication entirely was the one that said nothing.
	secure := os.Getenv("COOKIE_INSECURE") != "1" // 1 only for plain-http local dev
	devLogin := os.Getenv("DEV_LOGIN") == "1"
	if reason := devLoginRefusal(devLogin, secure); reason != "" {
		slog.Error("refusing to start", "reason", reason)
		os.Exit(1)
	}
	if devLogin {
		slog.Warn("DEV_LOGIN=1; POST /auth/dev/login is mounted and anybody can sign in " +
			"as any name without a credential (ADR-020). Never in production")
	}

	// One table, built once, read by three callers: the boot print below,
	// GET /readyz, and (through that endpoint) the launcher — which is R-36's
	// hard condition, that no second list of the same preconditions exists.
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
		// BETA-001's admission list (ADR-028 決策 1), read exactly like the operator
		// roster above and keyed by provider_user_id. Unset — the shipped default —
		// means no closed beta is running and every signed-in user is admitted.
		Invited:         operatorIDs(os.Getenv("BETA_ALLOWLIST")),
		Providers:       providers,
		Quota:           quotaFromEnv(),
		GenerateQuota:   generateQuotaFromEnv(),
		GenerateExposed: generateExposedFromEnv(),
		CreationExposed: creation.Exposed(), CreationLimits: creationLimits, CreationTransient: creationTransient,
		RateLimits: rateLimitsFromEnv(),
		// The same `clean` bool every other consequence below is gated on. It
		// travels as a Config field so features() does not read the variable a
		// second time: cleanModeFromEnv's comment promises one choice point, and
		// the promise is only true if nothing downstream reads the name.
		CleanMode: clean,
	})
	if err != nil {
		slog.Error("api composition", "error", err)
		os.Exit(1)
	}
	for _, task := range startupTasks(app) {
		task(ctx)
	}

	// Clean mode's third and last consequence: the worker runs inside this same
	// process instead of cmd/worker's own (see the package doc above and ADR-060
	// 決策 6). PollOnly:true is required, not cosmetic — with pool_max_conns=1
	// there is no second connection for River to LISTEN on, and without
	// PollOnly the queue client fails to start rather than falling back.

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

	// Clean mode's fourth and last consequence: the built web app is served
	// from this same process so 02:PORT-003's disclosure reaches a signed-out
	// visitor (see cleanModeHandler and cleanModeStaticHandler above). Flag
	// unset takes handler := app.Handler() and nothing past this block runs.
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

	// DEV_CORS_ORIGIN is the local Vite dev server (http://localhost:5173) and
	// nothing else. Unset in production, where the SPA is same-origin with the
	// API (ADR-018 E1) and no CORS header is wanted at all. See httpx.DevCORS for
	// why this is not a Vite proxy: the SPA's /skills/$skillId page route and the
	// API's /skills/{id} routes collide, so no path-prefix rule separates them.
	srv := &http.Server{
		Addr:              envx.Or("API_ADDR", ":8080"),
		Handler:           httpx.DevCORS(handler, os.Getenv("DEV_CORS_ORIGIN")),
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

	// The listener's failure travels on a channel instead of calling os.Exit from
	// inside the goroutine. An os.Exit there skips every defer this function has
	// registered — pool.Close and stopStore among them — so a port already in
	// use used to leak the connection pool and the in-process store on the way
	// out.
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
		// After the two closers above have had their turn, which is the whole
		// reason the goroutine no longer does this itself.
		pool.Close()
		if stopStore != nil {
			stopStore()
		}
		os.Exit(1)
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
// startupTasks is the work this process does once, after NewApp and before it
// serves, as a list rather than as bare calls — the same treatment and the same
// reason as backgroundLoops below: a call inside main is deletable with nothing
// red.
//
// AuditRosters is the whole list today, and it is the one that could least
// afford to be a bare call. It carries 02:SEC-011's "granting or revoking the
// operator role is itself an audit event" and it fails closed in both
// directions — an operator roster it cannot record is emptied, an invite list
// it cannot record is replaced with one nobody holds. Delete the call and the
// roster keeps working while nothing records it, which turns a fail-closed
// guarantee into a fail-open one with the whole suite green.
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
		// Said out loud, because this was the only one of the deployment settings
		// that produced no start-up line at all: the deployment came up looking
		// healthy and answered 503 on the last click of the core journey, with
		// nothing anywhere connecting the two (04 丙-102 ②).
		//
		// The message says why there is no default rather than just naming the
		// variable. A default here would be this process choosing a retention
		// period on the owner's behalf, and that period is quoted to users in the
		// consent form — it is a promise, not a parameter.
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

// profileDirReason says WHICH of the two ways a deployment ends up with no
// packaging targets, because the operator's next action for them is opposite: a
// missing directory is a path to fix, an empty one is a deployment that has not
// been given any profiles yet.
//
// LoadProfiles deliberately does not distinguish them — a missing directory is
// an empty set there, so that packaging refuses rather than inventing a target.
// That is right for the caller that serves the route and wrong for the caller
// that writes the start-up line, which is this one.
func profileDirReason(dir string) string {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return "the directory does not exist; note that a relative path resolves from this process's working directory, not from the repository root"
	}
	return "the directory exists but holds no *.json profile"
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

// feedbackRetentionFromEnv reads how long BETA-003/004/005's free-text reports
// are kept, for GET /policy/data-retention to disclose. The same variable
// `maintenance purge-feedback` refuses to run without, read here only to say
// what that sweep is configured to do: this process deletes nothing.
//
// Unset is zero and the endpoint prints it as "no retention period is
// configured... kept indefinitely", which is the truthful answer for a
// deployment PDM-006 has not given a number to. Deliberately NOT a default —
// feedback is a participant's own account of where the product failed them, and
// a default would put a deadline on it that nobody signed.
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
