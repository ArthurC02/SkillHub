package apiserver_test

// 02:PORT-010 / 04 丙-85: the clean test mode's content-source gate, from the
// composition root's side.
//
// The gate itself is a unit-tested decision in trial/execution (schedule_test.go).
// What only a database can answer is the half underneath it: `curation_tier`
// lives on `skills`, `is_catalog` lives on `workspaces`, they are owned by two
// different contexts, and run may not read either (ADR-033). So the facts arrive
// through an adapter assembled here, and an adapter that quietly returns the
// zero value would make every clean-mode run refuse for a reason nobody could
// diagnose — a red light nobody can read is as useless as a green one.
//
// Needs SKILLHUB_TEST_DATABASE_URL like the rest of this package; skips without.

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
		// Fail rather than skip: nil is the fail-closed branch of the gate, so a
		// root that forgot to wire it would make the clean test mode refuse
		// everything while every other test in this package stayed green.
		t.Fatal("NewApp left run.ReadContentSource unwired; the clean-mode content gate would refuse every run")
	}

	owner := a.login(t, "b1-clean-gate-owner")
	workspace := mustUUID(t, owner.workspaceID)
	skillID := importPackage(t, pool, a.packages, owner, "portgate-outgrabe-notes", false)
	first := mustUUID(t, newestVersion(t, pool, skillID))

	// An ordinary import into an ordinary workspace: nobody has reviewed it and
	// it is not in the public catalogue. This is the case the gate exists for.
	assertContentSource(t, read, workspace, first, false, "indexed", false)

	// PDM-002 recorded against these exact bytes.
	curate(t, pool, skillID, newestVersion(t, pool, skillID))
	assertContentSource(t, read, workspace, first, false, "curated", true)

	// The content moves on and nothing else changes: the column still says
	// curated and it is still about `first`. A tier-only check would hand the
	// unreviewed bytes to a driver with no isolation boundary, which is the one
	// mistake this pair of columns exists to prevent (0042).
	secondID := seedSkillVersion(t, pool, owner.workspaceID, skillID)
	second := mustUUID(t, secondID)
	if secondID == newestVersion(t, pool, skillID) && second == first {
		t.Fatal("the second version is the first one; the rest of this test proves nothing")
	}
	assertContentSource(t, read, workspace, second, false, "curated", false)
	assertContentSource(t, read, workspace, first, false, "curated", true)

	// A catalogue workspace passes on the other clause, whatever its tier is:
	// 02:PORT-007's curated material is the catalogue, and the tier is the
	// finer-grained verdict inside it.
	curator := a.login(t, "b1-clean-gate-curator")
	markCatalog(t, pool, curator.workspaceID)
	catalogueSkill := importPackage(t, pool, a.packages, curator, "portgate-catalogue-entry", false)
	assertContentSource(t, read,
		mustUUID(t, curator.workspaceID), mustUUID(t, newestVersion(t, pool, catalogueSkill)),
		true, "indexed", false)

	// Another workspace's version is not found rather than refused, which the
	// gate turns into a refusal anyway (WS-006: existence is private).
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

// The gate where it actually stands: in dispatch, on the way to a sandbox.
//
// The unit tests prove requireCuratedContent decides correctly; this one proves
// dispatch asks it at all. Deleting the call in job.go leaves every unit test
// green — the same shape as the three toothless fixes AGENTS.md records — so the
// assertion that matters here is fake.Dispatches().
//
// No worker is started: Drive is called by hand, so "was not dispatched" is a
// call that returned rather than a timeout that expired.
func TestTheCleanTestModeRefusesUncuratedMaterialBeforeItReachesAnySandbox(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	fake, svc := haltHarness(t, a, pool)
	f := newFixture(t, a, pool, "b1-clean-dispatch")
	ctx := context.Background()
	ws := mustUUID(t, f.workspaceID)

	// Created before the mode is on, so this is a test of the dispatch gate and
	// not of anything at creation time.
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
	// The reason has to be usable by the operator who set the variable: what was
	// refused, and what would have passed.
	for _, want := range []string{"curated", "catalogue", "indexed"} {
		if !strings.Contains(view.StatusReason, want) {
			t.Errorf("status reason = %q, want it to mention %q", view.StatusReason, want)
		}
	}
	if fake.Dispatches() != 0 {
		t.Fatalf("dispatches = %d; uncurated material reached the fleet", fake.Dispatches())
	}

	// And the gate is not simply "refuse everything in clean mode", which would
	// pass every assertion above while making the mode useless. Same fixture,
	// same driver, one recorded PDM-002 verdict apart.
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
