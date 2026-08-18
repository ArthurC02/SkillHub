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

	"github.com/ArthurC02/skillhub/services/platform/internal/analytics"
	"github.com/ArthurC02/skillhub/services/platform/internal/catalog"
	"github.com/ArthurC02/skillhub/services/platform/internal/eval"
	"github.com/ArthurC02/skillhub/services/platform/internal/identity"
	"github.com/ArthurC02/skillhub/services/platform/internal/ingest"
	"github.com/ArthurC02/skillhub/services/platform/internal/packaging"
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
	Auth      *identity.Handler
	Importer  *ingest.Handler
	Search    *catalog.Handler
	Registry  *registry.Handler
	TestLab   *testlab.Handler
	Runs      *run.Handler
	Trace     *trace.Handler
	Eval      *eval.Handler
	Packaging *packaging.Handler
	// Analytics serves POST /feedback and carries the funnel-event writer the
	// public handlers use (02:O11Y-004, BETA-003/004/005).
	Analytics *analytics.Handler
}

// NewRouter returns the API route table. Callers wrap it as needed — cmd/api
// adds httpx.DevCORS — but the routes and the middleware on each one are fixed
// here, so the authorization boundary is one reviewable list.
//
// Returns an http.Handler rather than the bare mux because one piece of
// middleware belongs to the table itself: analytics.Sessions wraps every route
// (see the bottom of this function). Leaving that to the caller is what this
// file's own history warns about — a wrapper cmd/api adds and the tests do not is
// a wrapper the tests cannot catch the loss of.
func NewRouter(d Deps) http.Handler {
	auth := d.Auth

	mux := http.NewServeMux()
	// Ogen pilot: only this exact public operation reaches the generated
	// router. Every authenticated/operator route remains explicitly mounted
	// below, so the reviewable AuthN/AuthZ matrix does not move into codegen.
	mux.Handle("GET /healthz", newGeneratedHealthHandler())
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
	// BETA-001's admission gate (ADR-028 決策 1) sits on exactly three actions —
	// fork, start a run, take a download — and on nothing else. Search and skill
	// detail stay open, because they already were: DISC-010 has served the
	// catalogue without a login since M1, so "an uninvited visitor can see the
	// catalogue" is existing behaviour rather than a hole opened for the beta.
	//
	// RequireInvited is inside RequireSession, never instead of it: it reads the
	// session user that wrapper put in the context. With no BETA_ALLOWLIST
	// configured it is a pass-through, which is every deployment up to now.
	mux.HandleFunc("POST /skills/{id}/fork", auth.RequireSession(auth.RequireInvited(d.Registry.Fork)))
	mux.HandleFunc("POST /skills/{id}/versions", auth.RequireSession(d.Importer.SaveVersion))
	// WS-001's read half, on the same path and a different method. The list has
	// existed as a query since M1 and nothing called it, which is why the two
	// screens that need to pick a version (pre-run permissions, packaging) took
	// the id from the URL and asked the reader to paste one (04 丙-14).
	mux.HandleFunc("GET /skills/{id}/versions", auth.RequireSession(d.Registry.Versions))
	mux.HandleFunc("GET /skills/{id}/diff", auth.RequireSession(d.Registry.Diff))
	mux.HandleFunc("DELETE /skills/{id}", auth.RequireSession(d.Registry.Delete))
	// INGEST-010: manual takedown of content in the caller's own workspace,
	// which for curated catalog entries is the operator's workspace.
	mux.HandleFunc("POST /skills/{id}/takedown", auth.RequireSession(d.Registry.Takedown))

	// 02:SEC-011 operator surface: the licensing hold of 0023, set and lifted.
	// Its own prefix and its own middleware — RequireOperator answers 404 to
	// everybody not on the deployment's operator list, so these two lines are the
	// only place in this table where "not allowed" and "no such route" are the
	// same answer. That is deliberate (SEC-011 不揭露端點存在).
	//
	// It is the only cross-workspace write in the table. Everything above scopes
	// to the session's workspace; a licensing question is about a source, so a
	// hold has to reach the catalogue entry and every fork of it alike.
	mux.HandleFunc("PUT /admin/skills/{id}/restriction", auth.RequireOperator(d.Search.SetRestriction))
	mux.HandleFunc("DELETE /admin/skills/{id}/restriction", auth.RequireOperator(d.Search.ClearRestriction))

	// 03:SEC-012's operator surface: 02:SEC-010's P1 first action, and the one place
	// 「現在到底有沒有在派送」 is answered. Same RequireOperator and therefore the same
	// 404 to everybody else — an endpoint that stops the whole fleet is one whose
	// existence is worth not advertising.
	//
	// Deliberately not a per-skill or per-workspace route: what it moves is the
	// execution plane's own switch, shared with the ADR-022 X-04 reconciler, and
	// nothing here reads workspace-private data (SEC-011 最小權力原則 still holds).
	mux.HandleFunc("GET /admin/dispatch", auth.RequireOperator(d.Runs.Halts))
	mux.HandleFunc("PUT /admin/dispatch/halt", auth.RequireOperator(d.Runs.DeclareHalt))
	mux.HandleFunc("DELETE /admin/dispatch/halt", auth.RequireOperator(d.Runs.LiftHalt))

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
	mux.HandleFunc("POST /skills/{id}/runs", auth.RequireSession(auth.RequireInvited(d.Runs.Create)))
	// PDM-010's allowance as the account holder sees it. Mounted only where one is
	// actually enforced, so a deployment with no allowance answers 404 here rather
	// than serving a ceiling nothing applies — the order ADR-028 決策 3 makes
	// binding, and the mistake 04 乙-2 records.
	if d.Runs.Svc.Quota.Enforced() {
		mux.HandleFunc("GET /me/quota", auth.RequireSession(d.Runs.Quota))
	}
	// WS-004's Run history. A list route beside the detail one, and the literal
	// segments below stay more specific than {id}, so all of them coexist.
	mux.HandleFunc("GET /runs", auth.RequireSession(d.Runs.List))
	mux.HandleFunc("GET /runs/{id}", auth.RequireSession(d.Runs.Get))
	mux.HandleFunc("POST /runs/{id}/cancel", auth.RequireSession(d.Runs.Cancel))
	// 02:WS-002 第 3 條 and 02:SEC-006 第 1 條: the owner sees and deletes what a
	// run produced. The list is what makes the delete reachable — a delete for
	// something nobody can enumerate is a delete nobody can press — and the two
	// arrived together for that reason (04 丙-22).
	//
	// No route serves the bytes: the archive is a sandbox's output and the control
	// plane does not open it (iron rule 1). What is served is the manifest row,
	// which is what evaluation reads too.
	mux.HandleFunc("GET /runs/{id}/artifacts", auth.RequireSession(d.Runs.Artifacts))
	mux.HandleFunc("DELETE /runs/{id}/artifacts/{artifactId}",
		auth.RequireSession(d.Runs.DeleteArtifact))
	// TRACE-006/007: one route, two modes. Session-scoped like every other run
	// route - a trace is user data.
	mux.HandleFunc("GET /runs/{id}/trace", auth.RequireSession(d.Trace.Get))

	// EVAL-001. A separate resource from the run on purpose: "what happened while
	// this executed" and "was the task achieved" are two facts, and no route here
	// lets one be read as the other (ADR-025). Session scoped like the run routes -
	// a verdict about a user's run is that user's data.
	mux.HandleFunc("GET /runs/{id}/evaluation", auth.RequireSession(d.Eval.Get))
	mux.HandleFunc("GET /runs/{id}/evaluation/revisions", auth.RequireSession(d.Eval.Revisions))
	mux.HandleFunc("PUT /runs/{id}/evaluation/feedback", auth.RequireSession(d.Eval.SetFeedback))

	// EVAL-002. Deciding and applying are separate routes because accepting five
	// suggestions has to produce one new version, not five. The apply route is a
	// skill route rather than a suggestion one for the same reason
	// POST /skills/{id}/versions is: what it creates is a version of that skill,
	// and the literal "from-suggestions" segment is more specific than the pattern
	// above it, so both coexist.
	mux.HandleFunc("GET /runs/{id}/suggestions", auth.RequireSession(d.Eval.Suggestions))
	mux.HandleFunc("PUT /suggestions/{id}/decision", auth.RequireSession(d.Eval.Decide))
	mux.HandleFunc("GET /suggestions/{id}/diff", auth.RequireSession(d.Eval.Diff))
	mux.HandleFunc("POST /skills/{id}/versions/from-suggestions",
		auth.RequireSession(d.Eval.ApplySuggestions))

	// EVAL-003's read half, and the whole of it. There is deliberately no re-run
	// route beside it: re-running is POST /skills/{id}/runs above with the new
	// version_id and the same test_case_id, which keeps preflight and
	// confirmed_summary_hash on the path (TEST-009). A "re-run" route here could
	// differ from that one in exactly one way — by skipping the permission screen.
	mux.HandleFunc("GET /runs/{id}/comparison", auth.RequireSession(d.Eval.Comparison))

	// PACK-001/002. Session scoped like everything else that touches a workspace:
	// a package built from a user's version is that user's, and the target list is
	// an endpoint rather than a frontend constant because support status changes
	// when a target is measured.
	//
	// There is deliberately no "re-package" route beside the POST: re-packaging is
	// the POST again, and because it is idempotent a repeat returns the artifact
	// that already exists. A second route to here would be a second route around
	// the four gates — the ruling that left EVAL-003 without a re-run route.
	//
	// The literal "versions/{versionId}/packaging" segments are more specific than
	// POST /skills/{id}/versions above, so both patterns coexist.
	mux.HandleFunc("GET /packaging/targets", auth.RequireSession(d.Packaging.Targets))
	mux.HandleFunc("GET /skills/{id}/versions/{versionId}/packaging/preview",
		auth.RequireSession(d.Packaging.Preview))
	mux.HandleFunc("POST /skills/{id}/versions/{versionId}/packaging",
		auth.RequireSession(auth.RequireInvited(d.Packaging.Create)))

	// WS-002/WS-004 and SEC-006: the download history, one package, its bytes and
	// its deletion. Same handler and the same workspace-from-session rule as the
	// routes above.
	//
	// The bytes are served by the platform rather than by a pre-signed URL
	// (packaging-design §7.1), so this is a route and not a redirect: the status,
	// expiry and licensing checks then happen on every request instead of once at
	// signing time, and the URL is not secret material, which is what lets a
	// download be recorded and audited at all.
	//
	// The literal "content" segment is more specific than {artifactId}, so the two
	// GET patterns coexist.
	mux.HandleFunc("GET /downloads", auth.RequireSession(d.Packaging.Downloads))
	mux.HandleFunc("GET /downloads/{artifactId}", auth.RequireSession(d.Packaging.Download))
	// WS-004 asks for "誰、何時、哪一筆 artifact、哪一個 profile", and the list above
	// answers the last two plus a count. This answers the first two, one row per
	// download. Deliberately not the audit trail: same event, two records, and
	// their retention and visibility differ (see internal/audit).
	mux.HandleFunc("GET /downloads/{artifactId}/records",
		auth.RequireSession(d.Packaging.DownloadRecords))
	// The third of the three gated actions, and the one funnel event that is not
	// emitted from the handler that owns it. DownloadStartedOn records "somebody
	// pressed download" before the handler decides whether the bytes go out; the
	// half that succeeded stays in download_records, which is a domain fact and
	// belongs in a domain table (ADR-029 決策 1). Splitting them is the entire
	// reason download_started exists as an event.
	mux.HandleFunc("GET /downloads/{artifactId}/content",
		auth.RequireSession(auth.RequireInvited(d.Analytics.DownloadStartedOn(d.Packaging.DownloadContent))))
	mux.HandleFunc("DELETE /downloads/{artifactId}", auth.RequireSession(d.Packaging.DeleteDownload))

	// BETA-003/004/005: one endpoint for the three entry points, split by `kind`.
	// Signed-in only — the surface it serves for an uninvited visitor is a user who
	// is logged in and has just been refused above, not an anonymous one.
	mux.HandleFunc("POST /feedback", auth.RequireSession(d.Analytics.Feedback))

	// 02:O11Y-004's disclosure half, and the only route in this table with no
	// session that is not an auth handshake: a data policy a visitor has to log in
	// to read is not a policy they can decide by, and the funnel's first segment is
	// measured before any login anyway. It reads nothing user-specific — the four
	// event names, their attribute whitelist, and the retention this deployment
	// actually applies (04 丙-25②).
	mux.HandleFunc("GET /policy/data-retention", d.Analytics.DataRetention)

	// TRACE-002: the execution plane pushes collected events here. Deliberately
	// the only route in this table with no session and no RequireSession wrapper:
	// its caller is a sandbox provider, and it authenticates with the per-attempt
	// signed token in its own path. It grants append-only access to one run's
	// trace and can read nothing (see internal/trace/token.go).
	mux.HandleFunc("POST "+trace.IngestPath+"{token}", d.Trace.Ingest)

	// 02:O11Y-004. Around the whole table and not per route: the funnel's first
	// segment happens on the public catalogue, which deliberately has no session
	// middleware (DISC-010), so anything mounted per authenticated route would
	// measure only signed-in traffic — precisely the population the funnel is not
	// about. A pass-through, cookie included, when the deployment collects nothing.
	return d.Analytics.Svc.Sessions(mux)
}
