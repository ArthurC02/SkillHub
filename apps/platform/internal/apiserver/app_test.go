package apiserver

import (
	"context"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/identity"
)

// The one invariant BetaGateClosed exists for: an unaudited cohort must not
// produce an empty invite list, because an empty one admits everybody (ADR-028
// 決策 1). A "fail closed" that returns nothing fails open.
func TestBetaGateClosedIsNotEmpty(t *testing.T) {
	if len(BetaGateClosed()) == 0 {
		t.Fatal("BetaGateClosed() is empty; an empty invite list admits every signed-in user")
	}
}

// The API's wiring, checked without a database: NewApp is a graph of struct
// literals and the only I/O it does is building an insert-only queue client,
// which does not dial. Nothing here asserts behaviour — it asserts that the
// dependencies exist, because every field involved is legitimately nil in some
// deployment and a forgotten one therefore fails at request time, in one route,
// as a nil-pointer panic.
func TestNewAppWiresEveryRouteAndService(t *testing.T) {
	// A parseable DSN nothing listens on; pgxpool connects lazily and nothing here
	// queries. River needs the pool to exist, not to work.
	pool, err := pgxpool.New(context.Background(), "postgres://skillhub@127.0.0.1:1/skillhub")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	app, err := NewApp(Config{Pool: pool, Secure: true})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}

	// Every route's handler, and for each of them the service it calls and the
	// identity it authorises with. Reflection rather than a list, so a handler
	// added to Deps without being wired in NewApp is caught by this test on the
	// day it is added and not by whoever calls its route first.
	deps := reflect.ValueOf(app.Deps)
	for i := range deps.NumField() {
		name := deps.Type().Field(i).Name
		handler := deps.Field(i)
		if handler.IsNil() {
			t.Errorf("Deps.%s is nil: its routes are mounted on nothing", name)
			continue
		}
		for _, dep := range []string{"Svc", "Service", "Identity"} {
			f := handler.Elem().FieldByName(dep)
			if f.IsValid() && f.Kind() == reflect.Pointer && f.IsNil() {
				t.Errorf("Deps.%s was wired without %s", name, dep)
			}
		}
	}

	// The exposed handles must be the instances the routes actually use. A second
	// Service built for the handler would compile, serve, and quietly ignore
	// everything an integration test set up through the handle.
	for _, pair := range []struct {
		name       string
		route, own any
	}{
		{"Auth", app.Deps.Auth, app.Auth},
		{"Runs", app.Deps.Runs.Svc, app.RunSvc},
		{"Eval", app.Deps.Eval.Svc, app.EvalSvc},
		{"Packaging", app.Deps.Packaging.Svc, app.PackagingSvc},
		{"Importer", app.Deps.Importer.Svc, app.Versions},
		{"Trace", app.Deps.Trace.Svc, app.TraceSvc},
	} {
		if pair.route != pair.own {
			t.Errorf("Deps.%s serves a different instance than the App handle of the same name", pair.name)
		}
	}

	// Iron rule 7 in the object graph: producing a verdict is the worker's job, so
	// the API's evaluation service must reach no judge even when one is configured
	// — see the Config comment. A judge here would mean an HTTP request paying for
	// a model call inline.
	if app.EvalSvc.Judge != nil || app.EvalSvc.Suggester != nil {
		t.Error("the API's evaluation service holds a judge or suggester; producing a verdict belongs to cmd/worker")
	}

	// The search projection writes: catalog owns `search_documents`, but the rows
	// are produced by ingest (import) and registry (fork, delete, takedown) inside
	// their own transactions, so only this composition root puts the two together
	// (ADR-034). Unset, both services refuse to write — which is the safe failure,
	// but it is a failure, and it belongs here rather than at the first import.
	if app.Versions.IndexSkill == nil {
		t.Error("the import path is missing catalog's search projection write")
	}
	if reg := app.Deps.Registry.Svc; reg.IndexSkill == nil || reg.RemoveFromIndex == nil {
		t.Error("the registry service is missing a search projection write")
	}

	// The account purge's cross-context steps (ADR-034). identity owns the
	// transaction; the rows belong to five other contexts, all of which import
	// identity, so a composition root is the only place the two can meet.
	// Reflection rather than a list for the same reason as Deps above: the day a
	// sixth context gains rows in a workspace, a field appears here and this test
	// covers it without being edited. Unwired, the purge refuses the batch — safe,
	// but it is still a compliance obligation not being met, and it should fail on
	// the pull request rather than on the first account that asked to be deleted.
	purgeSteps := reflect.ValueOf(*app.Auth.Service)
	for i := range purgeSteps.NumField() {
		field := purgeSteps.Field(i)
		if field.Type() == reflect.TypeFor[identity.WorkspacePurge]() && field.IsNil() {
			t.Errorf("identity.Service.%s is nil: the account purge would refuse to run",
				purgeSteps.Type().Field(i).Name)
		}
	}

	// suggesterOrNil's reason for existing: a nil *llmclient.Client must not land
	// in the interface field as a non-nil value, or "no LLM configured" panics
	// instead of reporting itself unavailable.
	if llm := app.Deps.TestLab.Svc.LLM; llm != nil {
		t.Errorf("no LLM service was configured, yet the test lab holds a suggester (%T)", llm)
	}
}
