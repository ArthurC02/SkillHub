package apiserver_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func TestTheContentSourceAdapterReadsCurationFromTheOwningContexts(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	read := a.runs.ReadContentSource
	if read == nil {

		t.Fatal("NewApp left run.ReadContentSource unwired; the clean-mode content gate would refuse every run")
	}

	owner := a.login(t, "b1-clean-gate-owner")
	workspace := mustUUID(t, owner.workspaceID)
	skillID := importPackage(t, pool, a.packages, owner, "portgate-outgrabe-notes", false)
	first := mustUUID(t, newestVersion(t, pool, skillID))

	assertContentSource(t, read, workspace, first, false, "indexed", false)

	curate(t, pool, skillID, newestVersion(t, pool, skillID))
	assertContentSource(t, read, workspace, first, false, "curated", true)

	secondID := seedSkillVersion(t, pool, owner.workspaceID, skillID)
	second := mustUUID(t, secondID)
	if secondID == newestVersion(t, pool, skillID) && second == first {
		t.Fatal("the second version is the first one; the rest of this test proves nothing")
	}
	assertContentSource(t, read, workspace, second, false, "curated", false)
	assertContentSource(t, read, workspace, first, false, "curated", true)

	curator := a.login(t, "b1-clean-gate-curator")
	markCatalog(t, pool, curator.workspaceID)
	catalogueSkill := importPackage(t, pool, a.packages, curator, "portgate-catalogue-entry", false)
	assertContentSource(t, read,
		mustUUID(t, curator.workspaceID), mustUUID(t, newestVersion(t, pool, catalogueSkill)),
		true, "indexed", false)

	if _, found, err := read(t.Context(), mustUUID(t, curator.workspaceID), first); err != nil || found {
		t.Errorf("reading another workspace's version returned found=%v err=%v, want false and no error", found, err)
	}
}

func assertContentSource(
	t *testing.T,
	read func(ctx context.Context, workspaceID, versionID pgtype.UUID) (run.ContentSource, bool, error),
	workspaceID, versionID pgtype.UUID,
	wantCatalog bool, wantTier string, wantSameVersion bool,
) {
	t.Helper()
	got, found, err := read(t.Context(), workspaceID, versionID)
	if err != nil {
		t.Fatalf("reading the content source failed: %v", err)
	}
	if !found {
		t.Fatalf("version %v was not found in workspace %v", versionID, workspaceID)
	}
	if got.WorkspaceIsCatalog != wantCatalog || got.CurationTier != wantTier ||
		got.CuratedVersionIsThisOne != wantSameVersion {
		t.Errorf("content source = %+v, want catalogue=%v tier=%q thisVersion=%v",
			got, wantCatalog, wantTier, wantSameVersion)
	}
}

func TestTheCleanTestModeRefusesUncuratedMaterialBeforeItReachesAnySandbox(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	fake, svc := haltHarness(t, a, pool)
	f := newFixture(t, a, pool, "b1-clean-dispatch")
	ctx := context.Background()
	ws := mustUUID(t, f.workspaceID)

	refused := f.start(t)

	t.Setenv("SKILLHUB_CLEAN_MODE", "1")
	if err := svc.Drive(ctx, ws, mustUUID(t, refused.RunID)); err != nil {
		t.Fatalf("driving a run in the clean test mode returned an error: %v", err)
	}
	code, view := f.getRun(t, refused.RunID)
	if code != http.StatusOK {
		t.Fatalf("GET run: %d", code)
	}
	if view.Status != string(gen.RunStatusFailed) {
		t.Fatalf("status = %q (%s), want failed: an ordinary imported skill ran on a driver with no boundary",
			view.Status, view.StatusReason)
	}

	for _, want := range []string{"curated", "catalogue", "indexed"} {
		if !strings.Contains(view.StatusReason, want) {
			t.Errorf("status reason = %q, want it to mention %q", view.StatusReason, want)
		}
	}
	if fake.Dispatches() != 0 {
		t.Fatalf("dispatches = %d; uncurated material reached the fleet", fake.Dispatches())
	}

	curate(t, pool, f.skillID, f.versionID)
	accepted := f.start(t)
	if err := svc.Drive(ctx, ws, mustUUID(t, accepted.RunID)); err != nil {
		t.Fatalf("driving curated material in the clean test mode: %v", err)
	}
	if _, view := f.getRun(t, accepted.RunID); view.Status != string(gen.RunStatusSucceeded) {
		t.Fatalf("curated material ended as %q (%s), want succeeded", view.Status, view.StatusReason)
	}
	if fake.Dispatches() != 1 {
		t.Errorf("dispatches = %d, want 1: curated material never reached the fleet", fake.Dispatches())
	}
}
