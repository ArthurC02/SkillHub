package apiserver

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

func TestCatalogRegistryFactsPreserveEveryConsumedField(t *testing.T) {
	uuid := func(marker byte) pgtype.UUID { return pgtype.UUID{Bytes: [16]byte{marker}, Valid: true} }
	skillID, workspaceID := uuid(1), uuid(2)
	forkSkillID, forkVersionID, curatedVersionID := uuid(3), uuid(4), uuid(5)
	versionID, sourceID := uuid(6), uuid(7)
	ts := pgtype.Timestamptz{Time: time.Unix(123, 0), Valid: true}
	summary, restriction, category := "summary", "license-review", "automation"
	skill := registry.Skill{
		ID: skillID, WorkspaceID: workspaceID, Name: "name", Summary: &summary,
		ForkedFromSkillID: forkSkillID, ForkedFromVersionID: forkVersionID, TakedownAt: ts,
		AccessRestriction: &restriction, Redistribution: "allowed", CurationTier: "curated",
		CuratedVersionID: curatedVersionID, Category: &category,
	}
	wantSkill := catalog.SkillFacts{
		ID: skillID, WorkspaceID: workspaceID, Name: "name", Summary: &summary,
		ForkedFromSkillID: forkSkillID, ForkedFromVersionID: forkVersionID, TakedownAt: ts,
		AccessRestriction: &restriction, Redistribution: "allowed", CurationTier: "curated",
		CuratedVersionID: curatedVersionID, Category: &category,
	}
	if got := catalogSkillFacts(skill); !reflect.DeepEqual(got, wantSkill) {
		t.Fatalf("catalog skill facts lost a Registry field: got %+v want %+v", got, wantSkill)
	}

	license, source := "MIT", "package"
	version := registry.Version{
		ID: versionID, WorkspaceID: workspaceID, SourceID: sourceID, VersionNumber: 7,
		ContentHash: "sha256:abc", PackageObjectKey: "packages/a.zip",
		LicenseExpression: &license, CreatedAt: ts, LicenseSource: &source,
	}
	wantVersion := catalog.VersionFacts{
		ID: versionID, WorkspaceID: workspaceID, SourceID: sourceID, VersionNumber: 7,
		ContentHash: "sha256:abc", PackageObjectKey: "packages/a.zip",
		LicenseExpression: &license, CreatedAt: ts, LicenseSource: &source,
	}
	if got := catalogVersionFacts(version); !reflect.DeepEqual(got, wantVersion) {
		t.Fatalf("catalog version facts lost a Registry field: got %+v want %+v", got, wantVersion)
	}

	compat := registry.RuntimeCompatibility{Capability: "activated", Runtime: "native", RuntimeImage: "sha256:image", MeasuredAt: ts}
	wantCompat := catalog.RuntimeCompatibilityFacts{Capability: "activated", Runtime: "native", RuntimeImage: "sha256:image", MeasuredAt: ts}
	if got := catalogRuntimeCompatibilityFacts(compat); !reflect.DeepEqual(got, wantCompat) {
		t.Fatalf("catalog compatibility facts lost a Registry field: got %+v want %+v", got, wantCompat)
	}
}

func TestBetaGateClosedIsNotEmpty(t *testing.T) {
	if len(BetaGateClosed()) == 0 {
		t.Fatal("BetaGateClosed() is empty; an empty invite list admits every signed-in user")
	}
}

func TestNewAppWiresEveryRouteAndService(t *testing.T) {

	pool, err := pgxpool.New(context.Background(), "postgres://skillhub@127.0.0.1:1/skillhub")
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	app, err := NewApp(Config{Pool: pool, Secure: true})
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}

	deps := reflect.ValueOf(app.Deps)
	checked := 0
	for i := range deps.NumField() {
		name := deps.Type().Field(i).Name
		handler := deps.Field(i)

		if handler.Kind() != reflect.Pointer {
			continue
		}

		if handler.Type().Elem().Name() != "Handler" {
			continue
		}
		checked++
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

	if checked < 8 {
		t.Errorf("the handler sweep checked %d fields; it is skipping things it should not", checked)
	}

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

	if app.EvalSvc.Judge != nil || app.EvalSvc.Suggester != nil {
		t.Error("the API's evaluation service holds a judge or suggester; producing a verdict belongs to cmd/worker")
	}

	if app.Versions.IndexSkill == nil {
		t.Error("the import path is missing catalog's search projection write")
	}
	if reg := app.Deps.Registry.Svc; reg.IndexSkill == nil || reg.RemoveFromIndex == nil {
		t.Error("the registry service is missing a search projection write")
	}

	if reg := app.Deps.Registry.Svc; reg.SkillRisks == nil || reg.CatalogSkillRisks == nil {
		t.Error("the registry service is missing catalog's projected scan read")
	}
	if search := app.Deps.Search.Svc; search.ReadCatalogSkill == nil || search.ReadWorkspaceSkill == nil ||
		search.ReadLatestVersion == nil || search.ReadRuntimeCompatibility == nil || search.SourceByID == nil {
		t.Error("the catalog service is missing owner-scoped Registry or source reads")
	}
	if app.PackagingSvc.AppliedSuggestions == nil || app.PackagingSvc.SourceLineage == nil {
		t.Error("the packaging service is missing manifest provenance owner reads")
	}
	if app.PackagingSvc.ReadSkill == nil || app.PackagingSvc.ReadVersion == nil ||
		app.PackagingSvc.ReadCompatibility == nil || app.PackagingSvc.ReadPrevious == nil ||
		app.PackagingSvc.ReadLineage == nil || app.PackagingSvc.ReadOldest == nil {
		t.Error("the packaging service is missing Registry owner reads")
	}
	if app.RunSvc.ActiveArtifactReferences == nil {
		t.Error("the run service is missing packaging's artifact reference counter")
	}

	if app.Deps.Runs.RunVerdicts == nil {
		t.Error("the run handler is missing eval's standing verdict read")
	}
	if app.RunSvc.WorkspaceCreatedAt == nil {
		t.Error("the run service is missing identity's workspace creation reader")
	}
	if app.RunSvc.ReadSkill == nil || app.RunSvc.ReadVersion == nil {
		t.Error("the run service is missing Registry owner reads")
	}
	if app.Deps.TestLab.Svc.ReadSkill == nil {
		t.Error("the test lab service is missing Registry's skill reader")
	}
	if testlabSvc := app.Deps.TestLab.Svc; app.RunSvc.TestLab != testlabSvc ||
		app.EvalSvc.TestLab != testlabSvc || app.PackagingSvc.TestLab != testlabSvc {
		t.Error("run, evaluation and packaging were not wired to the shared Test Lab service")
	}
	if app.Deps.Analytics.Svc.RunBelongsToWorkspace == nil {
		t.Error("the analytics service is missing Run's workspace membership reader")
	}
	if app.TraceSvc.ReadRunState == nil || app.TraceSvc.ReadIngestRunState == nil ||
		app.TraceSvc.ReadRunTransitions == nil {
		t.Error("the trace service is missing a Run-owned fact reader")
	}
	if app.RunSvc.Trace != app.TraceSvc {
		t.Error("the run service is missing the shared Trace owner service")
	}
	if app.EvalSvc.ReadRunFacts == nil || app.EvalSvc.ReadEvaluationInput == nil {
		t.Error("the evaluation service is missing Run-owned fact readers")
	}
	if app.EvalSvc.ReadVersion == nil || app.EvalSvc.ReadLatestVersion == nil ||
		app.EvalSvc.ReadSkill == nil || app.EvalSvc.ReadRuntimeCompatibility == nil {
		t.Error("the evaluation service is missing Registry-owned fact readers")
	}

	purgeSteps := reflect.ValueOf(*app.Auth.Service)
	for i := range purgeSteps.NumField() {
		field := purgeSteps.Field(i)
		if (field.Type() == reflect.TypeFor[identity.WorkspacePurge]() ||
			field.Type() == reflect.TypeFor[identity.WorkspaceObjectKeys]()) && field.IsNil() {
			t.Errorf("identity.Service.%s is nil: the account purge would refuse to run",
				purgeSteps.Type().Field(i).Name)
		}
	}
	app.Auth.Service.DownloadArtifactObjectKeys = nil
	if _, err := app.Auth.Service.PurgeExpiredAccounts(context.Background(), nil, 0, 1); err == nil ||
		!strings.Contains(err.Error(), "packaging (object keys)") {
		t.Errorf("missing packaging object-key reader did not fail closed: %v", err)
	}

	if llm := app.Deps.TestLab.Svc.LLM; llm != nil {
		t.Errorf("no LLM service was configured, yet the test lab holds a suggester (%T)", llm)
	}
}
