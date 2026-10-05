package apiserver_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/apiserver"
)

type unreachableStore struct{}

func (unreachableStore) Get(context.Context, string) ([]byte, error) {
	return nil, errors.New("object store unreachable")
}

func TestSkillDetailAnswersAStoreOutageWith503ButAMissingObjectWith200(t *testing.T) {
	pool := requireDB(t)
	a := newAPITuned(t, pool, "", func(d *apiserver.Deps) { d.Search.Svc.Store = unreachableStore{} })
	owner := a.login(t, "owner-detail-outage")
	skillID := seedSkill(t, pool, owner.workspaceID, "detail-outage")
	seedSkillVersion(t, pool, owner.workspaceID, skillID)

	if code := getJSON(t, owner.Client, a.URL+"/api/skills/"+skillID, nil); code != http.StatusServiceUnavailable {
		t.Fatalf("detail while the object store is down = %d, want 503", code)
	}

	healthy := newAPI(t, pool)
	owner = healthy.login(t, "owner-detail-missing")
	skillID = seedSkill(t, pool, owner.workspaceID, "detail-missing")
	seedSkillVersion(t, pool, owner.workspaceID, skillID)
	var got detail
	if code := getJSON(t, owner.Client, healthy.URL+"/api/skills/"+skillID, &got); code != http.StatusOK || got.Risk.ScanStatus != "unavailable" {
		t.Fatalf("detail of a package that was never stored = %d scan_status=%q, want 200 unavailable", code, got.Risk.ScanStatus)
	}
}
