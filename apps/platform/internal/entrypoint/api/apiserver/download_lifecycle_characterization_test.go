package apiserver_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestAWorkspaceThatMayNotStoreObjectsGetsNoPackage(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, uniqueWorklistLabel("package-may-not-store"))
	skillID, versionID := packagedSkill(t, a, pool, c, uniqueWorklistLabel("package-may-not-store-skill"))
	a.packaging.MayStoreObjects = func(context.Context, gen.DBTX, pgtype.UUID) (bool, error) { return false, nil }

	if code, _ := postJSON(t, c, packagingPath(skillID, versionID), `{"target":"standard"}`); code != http.StatusNotFound {
		t.Fatalf("packaging for a workspace that may not store objects: got %d, want 404", code)
	}
	if keys := workspaceDownloadObjects(a, c.workspaceID); len(keys) != 0 {
		t.Errorf("objects %v were written for a workspace that may not store any", keys)
	}
	if n := downloadIntentsIn(t, pool, c.workspaceID); n != 0 {
		t.Errorf("%d cleanup intents recorded, want 0", n)
	}
	if n := advisoryLocksHeldByTheProduct(t, pool); n != 0 {
		t.Errorf("the refused packaging kept the workspace object lock: %d advisory locks held", n)
	}
}

func TestPackagingWithoutALifecycleReadIsRefusedBeforeAnyObject(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, uniqueWorklistLabel("package-no-lifecycle"))
	skillID, versionID := packagedSkill(t, a, pool, c, uniqueWorklistLabel("package-no-lifecycle-skill"))
	a.packaging.MayStoreObjects = nil

	if code, _ := postJSON(t, c, packagingPath(skillID, versionID), `{"target":"standard"}`); code < 500 {
		t.Fatalf("packaging without a lifecycle read: got %d, want a server error", code)
	}
	if keys := workspaceDownloadObjects(a, c.workspaceID); len(keys) != 0 {
		t.Errorf("objects %v were written without asking whether the workspace may store them", keys)
	}
	if n := advisoryLocksHeldByTheProduct(t, pool); n != 0 {
		t.Errorf("the refused packaging kept the workspace object lock: %d advisory locks held", n)
	}
}
