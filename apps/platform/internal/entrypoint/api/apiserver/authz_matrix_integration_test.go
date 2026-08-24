// The AuthN/AuthZ boundary of the whole route table, as an anonymous caller sees
// it: one entry per mounted route, and a scan of the route table's own source
// that fails when the two lists drift apart (CORE-005/CORE-006, DDD-011).
//
// The matrix used to be hand-picked and covered about thirty of sixty-eight
// routes, which is the failure mode it was written to prevent: every route added
// after it was written — the whole Test Lab surface, downloads, trace, evaluation,
// the dispatch switch, feedback — was outside it, and a dropped RequireSession on
// any of them was invisible. A matrix that only its author remembers to extend is
// not a boundary, so TestEveryMountedRouteIsInTheAnonymousMatrix below reads
// router.go and identity/http.go and names anything missing.
//
// What is asserted per route is the anonymous answer the route's middleware
// promises, not the middleware itself: RequireSession answers 401, RequireOperator
// answers 404 (02:SEC-011 不揭露端點存在), and a public route answers whatever it
// serves — never 401, and never a 404 that would mean the route went missing.
package apiserver_test

import (
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/apiserver"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

// anonID is any well-formed uuid: every route below decides authorization before
// the handler parses its path values.
const anonID = "00000000-0000-0000-0000-000000000001"

type anonCase struct {
	// pattern is the route pattern verbatim, as mux.Handle receives it. It is the
	// join key against the source scan, so it must match character for character.
	pattern string
	// query is appended to the request when the handler needs one to reach the
	// answer under test. Empty for every route that refuses before parsing.
	query string
	// want is the status an anonymous caller gets from this deployment. For the
	// two conditional routes that means the state newAPI mounts them in — see
	// conditional, and the two extra assertions at the end of the matrix test.
	want int
	// conditional names the deployment setting that decides whether this route is
	// mounted at all. Empty for the routes that are always there.
	conditional string
}

// anonymousRoutes is every route in the table. Order follows router.go so the two
// can be read side by side; identity.Handler.Mount's routes come first because
// auth.Mount is the first call in NewRouter.
var anonymousRoutes = []anonCase{
	// --- identity.Handler.Mount (ADR-020) ---------------------------------------
	// The auth handshake is the public surface by definition: these are the routes
	// a caller with no session uses to get one.
	{pattern: "GET /auth/github/login", want: http.StatusFound},
	// 401 rather than a redirect: no state cookie means nothing to finish.
	{pattern: "GET /auth/github/callback", want: http.StatusUnauthorized},
	// Logging out with no session is a no-op, not an error.
	{pattern: "POST /auth/logout", want: http.StatusNoContent},
	{pattern: "GET /me", want: http.StatusUnauthorized},
	{pattern: "DELETE /me", want: http.StatusUnauthorized},
	{pattern: "POST /me/deletion/cancel", want: http.StatusUnauthorized},
	// Mounted only where DEV_LOGIN is on, which the test harness turns on and no
	// production deployment does.
	{pattern: "POST /auth/dev/login", want: http.StatusNoContent, conditional: "Config.DevLogin"},

	// --- public surface (DISC-001/006/007/008/010, O11Y-004) --------------------
	{pattern: "GET /healthz", want: http.StatusOK},
	{pattern: "GET /api/skills/search", query: "?q=anything", want: http.StatusOK},
	// OptionalSession: no session is not an error here, an unknown id is.
	{pattern: "GET /api/skills/{id}", want: http.StatusNotFound},
	{pattern: "GET /api/skills/{id}/files", want: http.StatusNotFound},
	// A data policy a visitor has to log in to read is not a policy they can
	// decide by, so this one answers anonymous callers in full.
	{pattern: "GET /policy/data-retention", want: http.StatusOK},
	// TRACE-002's machine-to-machine leg. No session by design: it authenticates
	// with the per-attempt signed token in its own path, and a caller without one
	// is refused there instead. 401 and not 404 — the route exists, the token does
	// not.
	{pattern: "POST " + trace.IngestPath + "{token}", want: http.StatusUnauthorized},

	// --- import and registry (SKILL-*, WS-001, INGEST-010) ----------------------
	{pattern: "POST /skills/import/upload", want: http.StatusUnauthorized},
	// Mounted only where ADR-052's exposure flag is on, which newAPI leaves off —
	// so what an anonymous caller gets here is what any unregistered path under
	// /skills gets, and that is 405 rather than 404 because DELETE /skills/{id}
	// matches the shape. That sameness IS the invisibility, and it is asserted
	// against a live sibling path in generate_integration_test rather than pinned
	// to a number here.
	{pattern: "POST /skills/generate", want: http.StatusMethodNotAllowed, conditional: "Config.GenerateExposed"},
	// GEN-003's read half, on the same flag. 404 and not 405 like the line
	// above: three segments match no other pattern, so an unmounted GET is
	// simply not a route — the same answer GET /me/quota gives where no
	// allowance is enforced. Mounted, it is RequireSession → 401.
	{pattern: "GET /skills/generate/failures", want: http.StatusNotFound, conditional: "Config.GenerateExposed"},
	{pattern: "POST /skills/import/url", want: http.StatusUnauthorized},
	{pattern: "GET /skills/search", query: "?q=anything", want: http.StatusUnauthorized},
	{pattern: "GET /skills", want: http.StatusUnauthorized},
	{pattern: "POST /skills/{id}/fork", want: http.StatusUnauthorized},
	{pattern: "POST /skills/{id}/versions", want: http.StatusUnauthorized},
	{pattern: "GET /skills/{id}/versions", want: http.StatusUnauthorized},
	{pattern: "GET /skills/{id}/diff", want: http.StatusUnauthorized},
	{pattern: "DELETE /skills/{id}", want: http.StatusUnauthorized},
	{pattern: "POST /skills/{id}/takedown", want: http.StatusUnauthorized},

	// --- operator surface (02:SEC-011, 03:SEC-012) ------------------------------
	// The one group that answers 404 instead of 401, deliberately: a caller must
	// not be able to learn these routes exist. Anonymous callers are never on the
	// operator list, so 404 is the whole assertion.
	{pattern: "PUT /admin/skills/{id}/restriction", want: http.StatusNotFound},
	{pattern: "DELETE /admin/skills/{id}/restriction", want: http.StatusNotFound},
	{pattern: "PUT /admin/skills/{id}/redistribution", want: http.StatusNotFound},
	{pattern: "GET /admin/dispatch", want: http.StatusNotFound},
	{pattern: "PUT /admin/dispatch/halt", want: http.StatusNotFound},
	{pattern: "DELETE /admin/dispatch/halt", want: http.StatusNotFound},

	// --- Test Lab (TEST-001/002/003/004) ----------------------------------------
	// Workspace scoped from the session throughout; there is no anonymous read
	// anywhere in this group, limits included.
	{pattern: "GET /test-cases/limits", want: http.StatusUnauthorized},
	{pattern: "POST /test-cases", want: http.StatusUnauthorized},
	{pattern: "GET /test-cases", want: http.StatusUnauthorized},
	{pattern: "GET /test-cases/{id}", want: http.StatusUnauthorized},
	{pattern: "PATCH /test-cases/{id}", want: http.StatusUnauthorized},
	{pattern: "DELETE /test-cases/{id}", want: http.StatusUnauthorized},
	{pattern: "POST /test-cases/{id}/criteria", want: http.StatusUnauthorized},
	{pattern: "POST /test-cases/{id}/criteria/suggest", want: http.StatusUnauthorized},
	{pattern: "PATCH /test-cases/{id}/criteria/{criterionId}", want: http.StatusUnauthorized},
	{pattern: "DELETE /test-cases/{id}/criteria/{criterionId}", want: http.StatusUnauthorized},
	{pattern: "POST /test-cases/{id}/datasets", want: http.StatusUnauthorized},
	{pattern: "GET /test-cases/{id}/datasets", want: http.StatusUnauthorized},
	{pattern: "DELETE /test-cases/{id}/datasets/{datasetId}", want: http.StatusUnauthorized},

	// --- runs (RUN-001~004, TEST-005, WS-002/004, SEC-006) ----------------------
	{pattern: "GET /skills/{id}/runs/preflight", want: http.StatusUnauthorized},
	{pattern: "POST /skills/{id}/runs/preflight/confirm", want: http.StatusUnauthorized},
	{pattern: "POST /skills/{id}/runs", want: http.StatusUnauthorized},
	// Mounted only where an allowance is enforced (ADR-028 決策 3). newAPI enforces
	// none, so the answer here is the unmounted one; the mounted one is asserted
	// separately below, because a route with two states needs both.
	{pattern: "GET /me/quota", want: http.StatusNotFound, conditional: "policy.QuotaLimits.Enforced()"},
	{pattern: "GET /runs", want: http.StatusUnauthorized},
	{pattern: "GET /runs/{id}", want: http.StatusUnauthorized},
	{pattern: "POST /runs/{id}/cancel", want: http.StatusUnauthorized},
	{pattern: "GET /runs/{id}/artifacts", want: http.StatusUnauthorized},
	{pattern: "DELETE /runs/{id}/artifacts/{artifactId}", want: http.StatusUnauthorized},
	// TRACE-006/007: a trace is user data, so its read half is session scoped even
	// though its write half above is not.
	{pattern: "GET /runs/{id}/trace", want: http.StatusUnauthorized},

	// --- evaluation and suggestions (EVAL-001/002/003) --------------------------
	{pattern: "GET /runs/{id}/evaluation", want: http.StatusUnauthorized},
	{pattern: "GET /runs/{id}/evaluation/revisions", want: http.StatusUnauthorized},
	{pattern: "PUT /runs/{id}/evaluation/feedback", want: http.StatusUnauthorized},
	{pattern: "GET /runs/{id}/suggestions", want: http.StatusUnauthorized},
	{pattern: "PUT /suggestions/{id}/decision", want: http.StatusUnauthorized},
	{pattern: "GET /suggestions/{id}/diff", want: http.StatusUnauthorized},
	{pattern: "POST /skills/{id}/versions/from-suggestions", want: http.StatusUnauthorized},
	{pattern: "GET /runs/{id}/comparison", want: http.StatusUnauthorized},

	// --- packaging and downloads (PACK-001~005, WS-002/004, SEC-006) ------------
	{pattern: "GET /packaging/targets", want: http.StatusUnauthorized},
	{pattern: "GET /skills/{id}/versions/{versionId}/packaging/preview",
		query: "?target=standard", want: http.StatusUnauthorized},
	{pattern: "POST /skills/{id}/versions/{versionId}/packaging", want: http.StatusUnauthorized},
	{pattern: "GET /downloads", want: http.StatusUnauthorized},
	{pattern: "GET /downloads/{artifactId}", want: http.StatusUnauthorized},
	{pattern: "GET /downloads/{artifactId}/records", want: http.StatusUnauthorized},
	// The bytes. RequireInvited sits inside RequireSession, so an anonymous caller
	// is refused by the outer one and never reaches the invite list.
	{pattern: "GET /downloads/{artifactId}/content", want: http.StatusUnauthorized},
	{pattern: "DELETE /downloads/{artifactId}", want: http.StatusUnauthorized},

	// --- closed beta (BETA-003/004/005) -----------------------------------------
	// Signed-in only: the surface it serves is a user who has just been refused
	// admission, not an anonymous visitor.
	{pattern: "POST /feedback", want: http.StatusUnauthorized},
}

// pathValue fills a route pattern's wildcards in. {token} gets something that is
// not a token, because that is the input the ingestion route's 401 is about.
var wildcardRE = regexp.MustCompile(`\{[^}]*\}`)

func (tc anonCase) request() (method, path string) {
	method, pattern, _ := strings.Cut(tc.pattern, " ")
	path = wildcardRE.ReplaceAllStringFunc(pattern, func(w string) string {
		if w == "{token}" {
			return "not-a-trace-ingestion-token"
		}
		return anonID
	})
	return method, path + tc.query
}

// CORE-005/CORE-006: every route in the real table, asserted against an anonymous
// caller. Two things fail here that nothing else catches: a route dropped from the
// table (the mux answers 404 where the matrix says 401), and a private route that
// starts answering anonymous callers.
//
// It does not distinguish "wrapped in RequireSession" from "not wrapped": each
// private handler re-checks SessionUser itself and returns the same 401, which is
// deliberate defence in depth, not redundancy. The boundary is the assertion here,
// not the mechanism that enforces it.
func TestAnonymousCallersGetThePublicSurfaceAndNothingElse(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	anon := &client{Client: &http.Client{
		// Follow no redirects: the login route's 302 is the assertion.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, base: a.URL}

	for _, tc := range anonymousRoutes {
		method, path := tc.request()
		if got := anon.status(t, method, path); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.pattern, got, tc.want)
		}
	}

	// The second state of the one conditional route the matrix cannot carry twice.
	// Enforced or not, an anonymous caller must never get an allowance back — the
	// difference between the two deployments is 404 versus 401, not 404 versus 200.
	quota := betaAPI(t, pool, policy.DefaultQuotaLimits(), nil, 0)
	quotaAnon := &client{Client: http.DefaultClient, base: quota.URL}
	if got := quotaAnon.status(t, http.MethodGet, "/me/quota"); got != http.StatusUnauthorized {
		t.Errorf("GET /me/quota with an allowance enforced: got %d, want 401", got)
	}
}

// --- the route table read from its own source --------------------------------

// routeCallRE captures the pattern argument of every mux.Handle/mux.HandleFunc
// call. Everything up to the first comma, because one pattern is a concatenation
// rather than a single literal (see resolvePattern).
var routeCallRE = regexp.MustCompile(`mux\.Handle(?:Func)?\(([^,]+),`)

// patternConstants are the identifiers a route pattern is allowed to be built
// from. Exactly one route uses one; resolving it beats skipping it, because a
// scan that quietly drops a route is a scan that enforces nothing.
var patternConstants = map[string]string{"trace.IngestPath": trace.IngestPath}

func resolvePattern(expr string) (string, bool) {
	var b strings.Builder
	for _, part := range strings.Split(expr, "+") {
		part = strings.TrimSpace(part)
		switch {
		case len(part) >= 2 && strings.HasPrefix(part, `"`) && strings.HasSuffix(part, `"`):
			b.WriteString(part[1 : len(part)-1])
		case patternConstants[part] != "":
			b.WriteString(patternConstants[part])
		default:
			return "", false
		}
	}
	return b.String(), true
}

// routeTableSources are the two files that mount routes: NewRouter's own table and
// the auth routes it delegates to identity.Handler.Mount. Read as source rather
// than reflected off a built mux because http.ServeMux does not expose its
// patterns, and because a route deleted from the source is what this is looking
// for.
var routeTableSources = []string{"router.go", "../../../creator/workspace/http.go"}

func mountedPatterns(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, name := range routeTableSources {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range routeCallRE.FindAllStringSubmatch(string(src), -1) {
			pattern, ok := resolvePattern(m[1])
			if !ok {
				t.Fatalf("%s: route pattern %s is not a literal or a known constant; "+
					"add it to patternConstants or this scan is silently missing a route",
					name, m[1])
			}
			out = append(out, pattern)
		}
	}
	return out
}

// DDD-011: the matrix above is only a boundary while it is complete, and nothing
// but this test makes it stay complete. Add a route to NewRouter and forget the
// matrix, and this names the route you forgot.
func TestEveryMountedRouteIsInTheAnonymousMatrix(t *testing.T) {
	t.Parallel()
	declared := make(map[string]bool, len(anonymousRoutes))
	for _, tc := range anonymousRoutes {
		if declared[tc.pattern] {
			t.Errorf("the matrix declares %q twice", tc.pattern)
		}
		declared[tc.pattern] = true
	}

	mounted := mountedPatterns(t)
	for _, p := range mounted {
		if !declared[p] {
			t.Errorf("route %q is mounted but has no entry in anonymousRoutes; "+
				"add one saying what an anonymous caller gets from it "+
				"(RequireSession→401, RequireOperator→404, public→its own status)", p)
		}
		delete(declared, p)
	}
	for p := range declared {
		t.Errorf("anonymousRoutes covers %q, which is no longer mounted; "+
			"delete the entry or restore the route", p)
	}
}

// A broken scan passes the test above by finding nothing, so the scan itself gets
// the same treatment as the table. No database: this reads source files.
func TestTheRouteTableScanFindsAPlausibleTable(t *testing.T) {
	t.Parallel()
	mounted := mountedPatterns(t)
	// A floor, not the count: the point is that the regex still matches a route
	// table rather than that the table has stopped growing.
	if len(mounted) < 60 {
		t.Fatalf("route scan found %d patterns; the table has had more than 60 since M4, "+
			"so the scan is broken rather than the table shrunk", len(mounted))
	}
	seen := make(map[string]bool, len(mounted))
	for _, p := range mounted {
		if seen[p] {
			t.Errorf("route %q is mounted twice; http.ServeMux panics on that at startup", p)
		}
		seen[p] = true
		if method, path, ok := strings.Cut(p, " "); !ok || !strings.HasPrefix(path, "/") ||
			strings.ToUpper(method) != method {
			t.Errorf("route %q is not a %q pattern; the scan is matching something else", p, "METHOD /path")
		}
	}
}

// NFR-001 clause 5, wired: the limiter sits on the three named routes and a
// refusal surfaces as a 429 with Retry-After. Nil Limits — every other test —
// means no limiting, which is why nothing else here ever trips it.
func TestTheNamedEndpointsAreRateLimitedWhenALimiterIsConfigured(t *testing.T) {
	pool := requireDB(t)
	a := newAPITuned(t, pool, "", func(d *apiserver.Deps) {
		d.Limits = httpx.NewRateLimiter(60, 2)
	})
	c := a.login(t, "ratelimit")

	codes := []int{}
	for i := 0; i < 4; i++ {
		resp, err := c.Get(c.base + "/api/skills/search?q=x")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		codes = append(codes, resp.StatusCode)
		if resp.StatusCode == http.StatusTooManyRequests {
			if resp.Header.Get("Retry-After") == "" {
				t.Error("429 without Retry-After")
			}
			return
		}
	}
	t.Fatalf("four requests against a burst of two never saw a 429: %v", codes)
}
