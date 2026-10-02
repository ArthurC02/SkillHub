package apiserver_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
)

func TestRebuildIndexRefreshesTheSkillsAfterOneThatFails(t *testing.T) {
	pool := requireDB(t)
	owner := newAPI(t, pool).login(t, "owner-rebuild-continues")
	svc := wiring.NewCatalogService(pool)
	var live []catalog.IndexSkillFacts
	var ids []string
	for i := 0; i < 3; i++ {
		id := seedSkill(t, pool, owner.workspaceID, fmt.Sprintf("rebuild-continues-%d", i))
		ids = append(ids, id)
		live = append(live, catalog.IndexSkillFacts{ID: mustUUID(t, id), WorkspaceID: mustUUID(t, owner.workspaceID), Name: "rebuild-continues", Summary: "s"})
	}
	svc.ReadLiveSkills = func(context.Context, gen.DBTX) ([]catalog.IndexSkillFacts, error) { return live, nil }
	read := svc.ReadLiveListingFacts
	boom := errors.New("listing facts unreadable")
	refreshed := map[pgtype.UUID]bool{}
	svc.ReadLiveListingFacts = func(ctx context.Context, db gen.DBTX, id pgtype.UUID) (catalog.ListingFacts, bool, error) {
		if id == live[0].ID {
			return catalog.ListingFacts{}, false, boom
		}
		refreshed[id] = true
		return read(ctx, db, id)
	}

	indexed, _, err := svc.RebuildIndex(context.Background())

	if !errors.Is(err, boom) || indexed != 3 {
		t.Fatalf("RebuildIndex indexed=%d err=%v, want 3 indexed and the first failure reported", indexed, err)
	}
	if !refreshed[live[1].ID] || !refreshed[live[2].ID] {
		t.Fatalf("skills refreshed after the failing one = %v, want %s and %s", refreshed, ids[1], ids[2])
	}
}
