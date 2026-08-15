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
	Runs     *run.Handler
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

	// Run orchestration (RUN-001/002/004). Runs are addressed by the platform
	// run_id and nothing else (iron rule 10): no route here takes a provider id.
	mux.HandleFunc("POST /skills/{id}/runs", auth.RequireSession(d.Runs.Create))
	mux.HandleFunc("GET /runs/{id}", auth.RequireSession(d.Runs.Get))
	mux.HandleFunc("POST /runs/{id}/cancel", auth.RequireSession(d.Runs.Cancel))

	return mux
}
