package apiserver_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestTheReleaseCommittedLastIsTheLatestEvenIfItsTransactionBeganFirst(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, freshName("release-order-alice"))
	registerPublisher(t, alice, freshName("release-order"))
	skillID := uploadedSkill(t, alice, freshName("ordered"), "Only way.")
	if code, body := publish(t, alice, skillID, `{"rights_attested":true}`); code != http.StatusOK {
		t.Fatalf("publish: %d %v", code, body)
	}
	ctx := context.Background()
	var params gen.InsertPublicationReleaseParams
	if err := pool.QueryRow(ctx, `
		SELECT r.publication_id, pb.workspace_id, r.skill_version_id, r.version_number, r.content_hash, r.released_by
		FROM publication_releases r
		JOIN publications p ON p.id = r.publication_id
		JOIN publishers pb ON pb.id = p.publisher_id
		WHERE p.skill_id = $1`, mustUUID(t, skillID),
	).Scan(&params.PublicationID, &params.WorkspaceID, &params.SkillVersionID, &params.VersionNumber, &params.ContentHash, &params.ReleasedBy); err != nil {
		t.Fatal(err)
	}
	params.Findings = []byte(`{}`)

	began, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = began.Rollback(ctx) }()
	if _, err := began.Exec(ctx, "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	if _, err := gen.New(pool).InsertPublicationRelease(ctx, params); err != nil {
		t.Fatal(err)
	}
	last, err := gen.New(began).InsertPublicationRelease(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if err := began.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	releases, err := gen.New(pool).ListPublicationReleases(ctx, params.PublicationID)
	if err != nil {
		t.Fatal(err)
	}
	if releases[0].ID != last.ID {
		t.Errorf("latest release = %v, want %v: it was inserted and committed last", releases[0].ID, last.ID)
	}
}
