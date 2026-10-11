package apiserver_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	registry "github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/publishing"
)

func (w exposureWorld) docketEntry(t *testing.T) (publishing.DocketEntry, bool) {
	t.Helper()
	catalogWorkspaces := (&identity.Service{Pool: w.pool}).CatalogWorkspaceIDs
	docket := wiring.NewExposureDocket(w.pool,
		&registry.Service{Pool: w.pool, CatalogWorkspaces: catalogWorkspaces},
		&catalog.Service{Pool: w.pool, CatalogWorkspaces: catalogWorkspaces})
	entries, err := docket(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.State.SkillID == mustUUID(t, w.skillID) {
			return entry, true
		}
	}
	return publishing.DocketEntry{}, false
}

func TestTheExposureDocketShowsAWaitingReleasesSearchTextAndScanUntilItIsApproved(t *testing.T) {
	w := newExposureWorld(t, "docket")
	entry, found := w.docketEntry(t)
	if !found {
		t.Fatal("a published release waiting for review is missing from the docket")
	}
	if entry.Snapshot == nil || entry.Snapshot.Name != w.name || entry.Snapshot.VersionID != entry.State.VersionID {
		t.Errorf("snapshot = %+v, want the searchable text of the released version", entry.Snapshot)
	}
	var raw []byte
	if err := w.pool.QueryRow(context.Background(),
		`SELECT r.findings FROM publication_releases r JOIN publications p ON p.id = r.publication_id
		 WHERE p.skill_id = $1 ORDER BY r.released_at DESC, r.id DESC LIMIT 1`, mustUUID(t, w.skillID)).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var stored skillpkg.CategorizedFindings
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(entry.Findings, stored) {
		t.Errorf("findings = %+v, want the release's stored scan %+v", entry.Findings, stored)
	}

	w.allowRedistribution(t)
	w.approveAndAssertExposedToAnyone(t)
	if _, found := w.docketEntry(t); found {
		t.Error("an approved, exposed release is still on the docket")
	}
}
