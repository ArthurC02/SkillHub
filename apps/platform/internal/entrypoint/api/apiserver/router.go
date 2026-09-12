package apiserver

import (
	"net/http"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/envx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/learning"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/delivery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

type Deps struct {
	Creation        *creationHandler
	CreationExposed bool
	Auth            *identity.Handler
	Importer        *ingest.Handler
	Search          *catalog.Handler
	Registry        *registry.Handler
	TestLab         *testlab.Handler
	Runs            *run.Handler
	Trace           *trace.Handler
	Eval            *eval.Handler
	Packaging       *packaging.Handler

	Credits *creditsHandler

	OperatorAudit *operatorAuditHandler

	Trends *trendsHandler

	Analytics *analytics.Handler

	Limits *httpx.RateLimiter

	AppURL string

	GenerateExposed bool

	Readiness *envx.Registry

	CleanMode bool
}

func NewRouter(d Deps) http.Handler {
	auth := d.Auth

	mux := http.NewServeMux()

	mux.Handle("GET /healthz", newGeneratedHealthHandler())

	mux.HandleFunc("GET /readyz", readinessHandler(d.Readiness, d.CleanMode))
	auth.Mount(mux)

	mux.HandleFunc("POST /skills/import/upload", limited(d, metrics.RouteImportUpload, auth.RequireSession(d.Importer.Upload)))
	mux.HandleFunc("POST /skills/import/url", limited(d, metrics.RouteImportURL, auth.RequireSession(d.Importer.ImportURL)))

	if d.GenerateExposed {

		mux.HandleFunc("POST /skills/generate", limited(d, metrics.RouteGenerate, auth.RequireSession(auth.RequireInvited(d.Importer.Generate))))

		mux.HandleFunc("GET /skills/generate/failures",
			auth.RequireSession(auth.RequireInvited(d.Importer.GenerateFailures)))
	}

	if d.GenerateExposed && d.CreationExposed && d.Creation != nil {
		mux.HandleFunc("GET /creation-sessions", auth.RequireSession(auth.RequireInvited(d.Creation.List)))
		mux.HandleFunc("POST /creation-sessions", limited(d, metrics.RouteGenerate, auth.RequireSession(auth.RequireInvited(d.Creation.Create))))
		mux.HandleFunc("GET /creation-sessions/{session_id}", auth.RequireSession(auth.RequireInvited(d.Creation.Get)))

		mux.HandleFunc("GET /creation-sessions/{session_id}/events", auth.RequireSession(auth.RequireInvited(d.Creation.Stream)))
		mux.HandleFunc("POST /creation-sessions/{session_id}/actions", limited(d, metrics.RouteGenerate, auth.RequireSession(auth.RequireInvited(d.Creation.Act))))
		mux.HandleFunc("GET /creation-sessions/limits", auth.RequireSession(auth.RequireInvited(d.Creation.Limits)))
	}

	mux.HandleFunc("GET /api/skills/search", limited(d, metrics.RoutePublicSearch, d.Search.PublicSearch))

	mux.HandleFunc("GET /api/skills/catalog", limited(d, metrics.RouteCatalog, d.Search.BrowseCatalog))

	mux.HandleFunc("GET /api/skills/{id}", auth.OptionalSession(d.Search.SkillDetail))
	mux.HandleFunc("GET /api/skills/{id}/files", auth.OptionalSession(d.Search.SkillFiles))

	mux.HandleFunc("GET /skills/search", auth.RequireSession(d.Search.Search))

	mux.HandleFunc("GET /skills", auth.RequireSession(d.Registry.List))

	mux.HandleFunc("POST /skills/{id}/fork", auth.RequireSession(auth.RequireInvited(d.Registry.Fork)))
	mux.HandleFunc("POST /skills/{id}/versions", auth.RequireSession(d.Importer.SaveVersion))

	mux.HandleFunc("GET /skills/{id}/versions", auth.RequireSession(d.Registry.Versions))
	mux.HandleFunc("GET /skills/{id}/diff", auth.RequireSession(d.Registry.Diff))
	mux.HandleFunc("DELETE /skills/{id}", auth.RequireSession(d.Registry.Delete))

	mux.HandleFunc("POST /skills/{id}/takedown", auth.RequireSession(d.Registry.Takedown))

	mux.HandleFunc("PUT /skills/{id}/category", auth.RequireSession(d.Registry.SetCategory))

	mux.HandleFunc("PUT /admin/skills/{id}/restriction", auth.RequireOperator(d.Search.SetRestriction))
	mux.HandleFunc("DELETE /admin/skills/{id}/restriction", auth.RequireOperator(d.Search.ClearRestriction))

	mux.HandleFunc("PUT /admin/skills/{id}/redistribution", auth.RequireOperator(d.Search.SetRedistribution))

	mux.HandleFunc("PUT /admin/skills/{id}/takedown", auth.RequireOperator(d.Search.Takedown))

	if d.Credits != nil {
		mux.HandleFunc("POST /admin/credits/{workspace_id}/grants", auth.RequireOperator(d.Credits.Grant))
		mux.HandleFunc("GET /admin/credits/{workspace_id}", auth.RequireOperator(d.Credits.Account))
		mux.HandleFunc("GET /admin/cost-statistics", auth.RequireOperator(d.Credits.CostStatistics))
		mux.HandleFunc("GET /admin/trends/cost", auth.RequireOperator(d.Trends.Cost))
		mux.HandleFunc("GET /admin/trends/credits", auth.RequireOperator(d.Trends.CreditMovement))
	}

	mux.HandleFunc("GET /admin/dispatch", auth.RequireOperator(d.Runs.Halts))
	mux.HandleFunc("PUT /admin/dispatch/halt", auth.RequireOperator(d.Runs.DeclareHalt))
	mux.HandleFunc("DELETE /admin/dispatch/halt", auth.RequireOperator(d.Runs.LiftHalt))

	mux.HandleFunc("GET /admin/accounts", auth.RequireOperator(auth.LookupAccount))
	mux.HandleFunc("GET /admin/rosters", auth.RequireOperator(auth.Rosters))
	mux.HandleFunc("GET /admin/skills", auth.RequireOperator(d.Search.FindSkillsForGovernance))
	mux.HandleFunc("GET /admin/audit-log", auth.RequireOperator(d.OperatorAudit.List))
	mux.HandleFunc("GET /admin/trends/runs", auth.RequireOperator(d.Trends.Runs))
	mux.HandleFunc("GET /admin/trends/operator-actions", auth.RequireOperator(d.Trends.OperatorActions))

	lab := d.TestLab
	mux.HandleFunc("GET /test-cases/limits", auth.RequireSession(lab.Limits))
	mux.HandleFunc("POST /test-cases", auth.RequireSession(lab.Create))
	mux.HandleFunc("GET /test-cases", auth.RequireSession(lab.List))
	mux.HandleFunc("GET /test-cases/{id}", auth.RequireSession(lab.Get))
	mux.HandleFunc("PATCH /test-cases/{id}", auth.RequireSession(lab.Update))
	mux.HandleFunc("DELETE /test-cases/{id}", auth.RequireSession(lab.Delete))
	mux.HandleFunc("POST /test-cases/{id}/criteria", auth.RequireSession(lab.AddCriterion))

	mux.HandleFunc("POST /test-cases/{id}/criteria/suggest", auth.RequireSession(lab.SuggestCriteria))
	mux.HandleFunc("PATCH /test-cases/{id}/criteria/{criterionId}", auth.RequireSession(lab.UpdateCriterion))
	mux.HandleFunc("DELETE /test-cases/{id}/criteria/{criterionId}", auth.RequireSession(lab.DeleteCriterion))
	mux.HandleFunc("POST /test-cases/{id}/datasets", auth.RequireSession(lab.UploadDataset))
	mux.HandleFunc("GET /test-cases/{id}/datasets", auth.RequireSession(lab.ListDatasets))
	mux.HandleFunc("DELETE /test-cases/{id}/datasets/{datasetId}", auth.RequireSession(lab.DeleteDataset))

	mux.HandleFunc("GET /skills/{id}/runs/preflight", auth.RequireSession(d.Runs.Preflight))
	mux.HandleFunc("POST /skills/{id}/runs/preflight/confirm", auth.RequireSession(d.Runs.ConfirmPreflight))
	mux.HandleFunc("POST /skills/{id}/runs", auth.RequireSession(auth.RequireInvited(d.Runs.Create)))

	if d.Runs.Svc.Quota.Enforced() {
		mux.HandleFunc("GET /me/quota", auth.RequireSession(d.Runs.Quota))
	}

	if d.Credits != nil {
		mux.HandleFunc("GET /me/credits", auth.RequireSession(d.Credits.Get))
	}

	mux.HandleFunc("GET /runs", auth.RequireSession(d.Runs.List))
	mux.HandleFunc("GET /runs/{id}", auth.RequireSession(d.Runs.Get))
	mux.HandleFunc("POST /runs/{id}/cancel", auth.RequireSession(d.Runs.Cancel))

	mux.HandleFunc("GET /runs/{id}/artifacts", auth.RequireSession(d.Runs.Artifacts))
	mux.HandleFunc("DELETE /runs/{id}/artifacts/{artifactId}",
		auth.RequireSession(d.Runs.DeleteArtifact))

	mux.HandleFunc("GET /runs/{id}/trace", auth.RequireSession(d.Trace.Get))

	mux.HandleFunc("GET /runs/{id}/evaluation", auth.RequireSession(d.Eval.Get))
	mux.HandleFunc("GET /runs/{id}/evaluation/revisions", auth.RequireSession(d.Eval.Revisions))
	mux.HandleFunc("PUT /runs/{id}/evaluation/feedback", auth.RequireSession(d.Eval.SetFeedback))

	mux.HandleFunc("GET /runs/{id}/suggestions", auth.RequireSession(d.Eval.Suggestions))
	mux.HandleFunc("PUT /suggestions/{id}/decision", auth.RequireSession(d.Eval.Decide))
	mux.HandleFunc("GET /suggestions/{id}/diff", auth.RequireSession(d.Eval.Diff))
	mux.HandleFunc("POST /skills/{id}/versions/from-suggestions",
		auth.RequireSession(d.Eval.ApplySuggestions))

	mux.HandleFunc("GET /runs/{id}/comparison", auth.RequireSession(d.Eval.Comparison))

	mux.HandleFunc("GET /packaging/targets", auth.RequireSession(d.Packaging.Targets))
	mux.HandleFunc("GET /skills/{id}/versions/{versionId}/packaging/preview",
		auth.RequireSession(d.Packaging.Preview))
	mux.HandleFunc("POST /skills/{id}/versions/{versionId}/packaging",
		auth.RequireSession(auth.RequireInvited(d.Packaging.Create)))

	mux.HandleFunc("GET /downloads", auth.RequireSession(d.Packaging.Downloads))
	mux.HandleFunc("GET /downloads/{artifactId}", auth.RequireSession(d.Packaging.Download))

	mux.HandleFunc("GET /downloads/{artifactId}/records",
		auth.RequireSession(d.Packaging.DownloadRecords))

	mux.HandleFunc("GET /downloads/{artifactId}/content",
		auth.RequireSession(auth.RequireInvited(d.Analytics.DownloadStartedOn(d.Packaging.DownloadContent))))
	mux.HandleFunc("DELETE /downloads/{artifactId}", auth.RequireSession(d.Packaging.DeleteDownload))

	mux.HandleFunc("POST /feedback", auth.RequireSession(d.Analytics.Feedback))

	mux.HandleFunc("GET /policy/data-retention", d.Analytics.DataRetention)

	mux.HandleFunc("POST "+trace.IngestPath+"{token}", d.Trace.Ingest)

	return d.Analytics.Svc.Sessions(httpx.SameOriginWrites(mux, d.AppURL))
}

func limited(d Deps, route string, next http.HandlerFunc) http.HandlerFunc {
	if d.Limits == nil {
		return next
	}
	return d.Limits.Limit(route, next)
}
