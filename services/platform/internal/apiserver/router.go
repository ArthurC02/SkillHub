// Package apiserver holds the public HTTP API's route table.
//
// It lives here rather than inside cmd/api so integration tests can import the
// real thing. When each test re-declared its own mux, a dropped RequireSession
// in cmd/api left every test green, and routes absent from the hand-copied
// tables (POST /skills/import/url, POST /skills/{id}/versions) had no coverage
// at all.
package apiserver

import (
	"net/http"

	"github.com/ArthurC02/skillhub/services/platform/internal/catalog"
	"github.com/ArthurC02/skillhub/services/platform/internal/identity"
	"github.com/ArthurC02/skillhub/services/platform/internal/ingest"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/httpx"
	"github.com/ArthurC02/skillhub/services/platform/internal/registry"
	"github.com/ArthurC02/skillhub/services/platform/internal/run"
	"github.com/ArthurC02/skillhub/services/platform/internal/testlab"
	"github.com/ArthurC02/skillhub/services/platform/internal/trace"
)

// Deps are the wired handlers the routes dispatch to. Constructing them
// (database pool, object store, LLM client, cookie policy) stays with the
// caller: cmd/api reads them from the environment, tests point them at a
// throwaway database.
type Deps struct {
	Auth     *identity.Handler
	Importer *ingest.Handler
	Search   *catalog.Handler
	Registry *registry.Handler
	TestLab  *testlab.Handler
	Runs     *run.Handler
	Trace    *trace.Handler
}

// NewRouter returns the API route table. Callers wrap it as needed — cmd/api
// adds httpx.DevCORS — but the routes and the middleware on each one are fixed
// here, so the authorization boundary is one reviewable list.
func NewRouter(d Deps) *http.ServeMux {
	auth := d.Auth

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", httpx.Health)
	auth.Mount(mux)
	mux.HandleFunc("POST /skills/import/upload", auth.RequireSession(d.Importer.Upload))
	mux.HandleFunc("POST /skills/import/url", auth.RequireSession(d.Importer.ImportURL))

	// DISC-001: public search works without login.
	mux.HandleFunc("GET /api/skills/search", d.Search.PublicSearch)
	// DISC-006/007/008/010: public detail and file views, no login required.
	// OptionalSession, not RequireSession: anonymous callers get the catalog and
	// a signed-in caller additionally gets their own private skills through the
	// same handler (see catalog/detail.go). The literal "search" segment above is
	// the more specific pattern, so it still wins over {id}.
	mux.HandleFunc("GET /api/skills/{id}", auth.OptionalSession(d.Search.SkillDetail))
	mux.HandleFunc("GET /api/skills/{id}/files", auth.OptionalSession(d.Search.SkillFiles))
	// Workspace-scoped search (existing, requires session).
	mux.HandleFunc("GET /skills/search", auth.RequireSession(d.Search.Search))

	mux.HandleFunc("GET /skills", auth.RequireSession(d.Registry.List))
	mux.HandleFunc("POST /skills/{id}/fork", auth.RequireSession(d.Registry.Fork))
	mux.HandleFunc("POST /skills/{id}/versions", auth.RequireSession(d.Importer.SaveVersion))
	mux.HandleFunc("GET /skills/{id}/diff", auth.RequireSession(d.Registry.Diff))
	mux.HandleFunc("DELETE /skills/{id}", auth.RequireSession(d.Registry.Delete))
	// INGEST-010: manual takedown of content in the caller's own workspace,
	// which for curated catalog entries is the operator's workspace.
	mux.HandleFunc("POST /skills/{id}/takedown", auth.RequireSession(d.Registry.Takedown))

	// Test Lab (TEST-001/003/004). Everything is workspace scoped from the
	// session; there is no anonymous read here. The literal "limits" segment is
	// more specific than {id}, so it still wins.
	lab := d.TestLab
	mux.HandleFunc("GET /test-cases/limits", auth.RequireSession(lab.Limits))
	mux.HandleFunc("POST /test-cases", auth.RequireSession(lab.Create))
	mux.HandleFunc("GET /test-cases", auth.RequireSession(lab.List))
	mux.HandleFunc("GET /test-cases/{id}", auth.RequireSession(lab.Get))
	mux.HandleFunc("PATCH /test-cases/{id}", auth.RequireSession(lab.Update))
	mux.HandleFunc("DELETE /test-cases/{id}", auth.RequireSession(lab.Delete))
	mux.HandleFunc("POST /test-cases/{id}/criteria", auth.RequireSession(lab.AddCriterion))
	// TEST-002: the literal "suggest" segment beats the {criterionId} patterns.
	mux.HandleFunc("POST /test-cases/{id}/criteria/suggest", auth.RequireSession(lab.SuggestCriteria))
	mux.HandleFunc("PATCH /test-cases/{id}/criteria/{criterionId}", auth.RequireSession(lab.UpdateCriterion))
	mux.HandleFunc("DELETE /test-cases/{id}/criteria/{criterionId}", auth.RequireSession(lab.DeleteCriterion))
	mux.HandleFunc("POST /test-cases/{id}/datasets", auth.RequireSession(lab.UploadDataset))
	mux.HandleFunc("GET /test-cases/{id}/datasets", auth.RequireSession(lab.ListDatasets))
	mux.HandleFunc("DELETE /test-cases/{id}/datasets/{datasetId}", auth.RequireSession(lab.DeleteDataset))

	// Run orchestration (RUN-001/002/004). Runs are addressed by the platform
	// run_id and nothing else (iron rule 10): no route here takes a provider id.
	//
	// The two preflight routes are the 02:TEST-005 gate: read what the run may
	// touch, agree to it, and only then create the run. The literal "preflight"
	// segment is more specific than the POST above, so both patterns coexist.
	mux.HandleFunc("GET /skills/{id}/runs/preflight", auth.RequireSession(d.Runs.Preflight))
	mux.HandleFunc("POST /skills/{id}/runs/preflight/confirm", auth.RequireSession(d.Runs.ConfirmPreflight))
	mux.HandleFunc("POST /skills/{id}/runs", auth.RequireSession(d.Runs.Create))
	mux.HandleFunc("GET /runs/{id}", auth.RequireSession(d.Runs.Get))
	mux.HandleFunc("POST /runs/{id}/cancel", auth.RequireSession(d.Runs.Cancel))
	// TRACE-006/007: one route, two modes. Session-scoped like every other run
	// route - a trace is user data.
	mux.HandleFunc("GET /runs/{id}/trace", auth.RequireSession(d.Trace.Get))

	// TRACE-002: the execution plane pushes collected events here. Deliberately
	// the only route in this table with no session and no RequireSession wrapper:
	// its caller is a sandbox provider, and it authenticates with the per-attempt
	// signed token in its own path. It grants append-only access to one run's
	// trace and can read nothing (see internal/trace/token.go).
	mux.HandleFunc("POST "+trace.IngestPath+"{token}", d.Trace.Ingest)

	return mux
}
