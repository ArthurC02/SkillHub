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

const anonID = "00000000-0000-0000-0000-000000000001"

type anonCase struct {
	pattern string

	query string

	want int

	conditional string
}

var anonymousRoutes = []anonCase{
	{pattern: "GET /creation-sessions", want: http.StatusNotFound, conditional: "creationEnabled"},
	{pattern: "POST /creation-sessions", want: http.StatusNotFound, conditional: "creationEnabled"},
	{pattern: "GET /creation-sessions/{session_id}", want: http.StatusNotFound, conditional: "creationEnabled"},
	{pattern: "GET /creation-sessions/{session_id}/events", want: http.StatusNotFound, conditional: "creationEnabled"},
	{pattern: "POST /creation-sessions/{session_id}/actions", want: http.StatusNotFound, conditional: "creationEnabled"},
	{pattern: "GET /creation-sessions/limits", want: http.StatusNotFound, conditional: "creationEnabled"},

	{pattern: "GET /auth/github/login", want: http.StatusFound},

	{pattern: "GET /auth/github/callback", want: http.StatusUnauthorized},

	{pattern: "POST /auth/logout", want: http.StatusNoContent},
	{pattern: "GET /me", want: http.StatusUnauthorized},
	{pattern: "DELETE /me", want: http.StatusUnauthorized},
	{pattern: "POST /me/deletion/cancel", want: http.StatusUnauthorized},

	{pattern: "POST /auth/dev/login", want: http.StatusNoContent, conditional: "Config.DevLogin"},

	{pattern: "GET /healthz", want: http.StatusOK},

	{pattern: "GET /readyz", want: http.StatusOK},
	{pattern: "GET /api/skills/search", query: "?q=anything", want: http.StatusOK},

	{pattern: "GET /api/skills/catalog", want: http.StatusOK},

	{pattern: "GET /api/skills/{id}", want: http.StatusNotFound},
	{pattern: "GET /api/skills/{id}/files", want: http.StatusNotFound},

	{pattern: "GET /policy/data-retention", want: http.StatusOK},

	{pattern: "POST " + trace.IngestPath + "{token}", want: http.StatusUnauthorized},

	{pattern: "POST /skills/import/upload", want: http.StatusUnauthorized},

	{pattern: "POST /skills/generate", want: http.StatusMethodNotAllowed, conditional: "Config.GenerateExposed"},

	{pattern: "GET /skills/generate/failures", want: http.StatusNotFound, conditional: "Config.GenerateExposed"},
	{pattern: "POST /skills/import/url", want: http.StatusUnauthorized},
	{pattern: "GET /skills/search", query: "?q=anything", want: http.StatusUnauthorized},
	{pattern: "GET /skills", want: http.StatusUnauthorized},
	{pattern: "POST /skills/{id}/fork", want: http.StatusUnauthorized},
	{pattern: "PUT /skills/{id}/category", want: http.StatusUnauthorized},
	{pattern: "POST /skills/{id}/versions", want: http.StatusUnauthorized},
	{pattern: "GET /skills/{id}/versions", want: http.StatusUnauthorized},
	{pattern: "GET /skills/{id}/diff", want: http.StatusUnauthorized},
	{pattern: "DELETE /skills/{id}", want: http.StatusUnauthorized},
	{pattern: "POST /skills/{id}/takedown", want: http.StatusUnauthorized},

	{pattern: "PUT /admin/skills/{id}/restriction", want: http.StatusNotFound},
	{pattern: "DELETE /admin/skills/{id}/restriction", want: http.StatusNotFound},
	{pattern: "PUT /admin/skills/{id}/redistribution", want: http.StatusNotFound},
	{pattern: "PUT /admin/skills/{id}/takedown", want: http.StatusNotFound},

	{pattern: "POST /admin/credits/{workspace_id}/grants", want: http.StatusNotFound},
	{pattern: "GET /admin/dispatch", want: http.StatusNotFound},
	{pattern: "PUT /admin/dispatch/halt", want: http.StatusNotFound},
	{pattern: "DELETE /admin/dispatch/halt", want: http.StatusNotFound},

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

	{pattern: "GET /skills/{id}/runs/preflight", want: http.StatusUnauthorized},
	{pattern: "POST /skills/{id}/runs/preflight/confirm", want: http.StatusUnauthorized},
	{pattern: "POST /skills/{id}/runs", want: http.StatusUnauthorized},

	{pattern: "GET /me/quota", want: http.StatusNotFound, conditional: "policy.QuotaLimits.Enforced()"},

	{pattern: "GET /me/credits", want: http.StatusUnauthorized},
	{pattern: "GET /runs", want: http.StatusUnauthorized},
	{pattern: "GET /runs/{id}", want: http.StatusUnauthorized},
	{pattern: "POST /runs/{id}/cancel", want: http.StatusUnauthorized},
	{pattern: "GET /runs/{id}/artifacts", want: http.StatusUnauthorized},
	{pattern: "DELETE /runs/{id}/artifacts/{artifactId}", want: http.StatusUnauthorized},

	{pattern: "GET /runs/{id}/trace", want: http.StatusUnauthorized},

	{pattern: "GET /runs/{id}/evaluation", want: http.StatusUnauthorized},
	{pattern: "GET /runs/{id}/evaluation/revisions", want: http.StatusUnauthorized},
	{pattern: "PUT /runs/{id}/evaluation/feedback", want: http.StatusUnauthorized},
	{pattern: "GET /runs/{id}/suggestions", want: http.StatusUnauthorized},
	{pattern: "PUT /suggestions/{id}/decision", want: http.StatusUnauthorized},
	{pattern: "GET /suggestions/{id}/diff", want: http.StatusUnauthorized},
	{pattern: "POST /skills/{id}/versions/from-suggestions", want: http.StatusUnauthorized},
	{pattern: "GET /runs/{id}/comparison", want: http.StatusUnauthorized},

	{pattern: "GET /packaging/targets", want: http.StatusUnauthorized},
	{pattern: "GET /skills/{id}/versions/{versionId}/packaging/preview",
		query: "?target=standard", want: http.StatusUnauthorized},
	{pattern: "POST /skills/{id}/versions/{versionId}/packaging", want: http.StatusUnauthorized},
	{pattern: "GET /downloads", want: http.StatusUnauthorized},
	{pattern: "GET /downloads/{artifactId}", want: http.StatusUnauthorized},
	{pattern: "GET /downloads/{artifactId}/records", want: http.StatusUnauthorized},

	{pattern: "GET /downloads/{artifactId}/content", want: http.StatusUnauthorized},
	{pattern: "DELETE /downloads/{artifactId}", want: http.StatusUnauthorized},

	{pattern: "POST /feedback", want: http.StatusUnauthorized},
}

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

func TestAnonymousCallersGetThePublicSurfaceAndNothingElse(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	anon := &client{Client: &http.Client{

		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}, base: a.URL}

	for _, tc := range anonymousRoutes {
		method, path := tc.request()
		if got := anon.status(t, method, path); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.pattern, got, tc.want)
		}
	}

	quota := betaAPI(t, pool, policy.DefaultQuotaLimits(), nil, 0)
	quotaAnon := &client{Client: http.DefaultClient, base: quota.URL}
	if got := quotaAnon.status(t, http.MethodGet, "/me/quota"); got != http.StatusUnauthorized {
		t.Errorf("GET /me/quota with an allowance enforced: got %d, want 401", got)
	}
}

var routeCallRE = regexp.MustCompile(`mux\.Handle(?:Func)?\(([^,]+),`)

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

func TestTheRouteTableScanFindsAPlausibleTable(t *testing.T) {
	t.Parallel()
	mounted := mountedPatterns(t)

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

func TestTheNamedEndpointsAreRateLimitedWhenALimiterIsConfigured(t *testing.T) {
	pool := requireDB(t)

	for _, route := range []struct {
		name string
		call func(*api, *client) (*http.Response, error)
	}{
		{"anonymous search", func(a *api, c *client) (*http.Response, error) {
			return c.Get(c.base + "/api/skills/search?q=x")
		}},
		{"import upload", func(a *api, c *client) (*http.Response, error) {
			return c.Post(c.base+"/skills/import/upload", "application/octet-stream", strings.NewReader("not a zip"))
		}},
		{"import url", func(a *api, c *client) (*http.Response, error) {
			return c.Post(c.base+"/skills/import/url", "application/json",
				strings.NewReader(`{"url":"https://example.invalid/x.zip"}`))
		}},
	} {
		t.Run(route.name, func(t *testing.T) {
			a := newAPITuned(t, pool, "", func(d *apiserver.Deps) {
				d.Limits = httpx.NewRateLimiter(60, 2)
			})
			c := a.login(t, "ratelimit-"+strings.ReplaceAll(route.name, " ", "-"))
			codes := []int{}
			for i := 0; i < 5; i++ {
				resp, err := route.call(a, c)
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
			t.Fatalf("five requests against a burst of two never saw a 429: %v", codes)
		})
	}
}
