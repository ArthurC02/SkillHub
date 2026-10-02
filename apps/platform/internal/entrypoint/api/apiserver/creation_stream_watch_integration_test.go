package apiserver_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func seedCreationSessionExpiringIn(t *testing.T, pool *pgxpool.Pool, workspaceID pgtype.UUID, interval string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO creation_sessions (id, workspace_id, state, revision, snapshot, expires_at)
		VALUES (gen_random_uuid(), $1, 'working', 1, '{"snapshot":{}}', now() + $2::interval)
		RETURNING id`, workspaceID, interval).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestTheStreamWatchReadsAnExpiredSessionAsGone(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "stream-watch-expired")
	workspace := mustUUID(t, alice.workspaceID)
	live := seedCreationSessionExpiringIn(t, pool, workspace, "1 day")
	expired := seedCreationSessionExpiringIn(t, pool, workspace, "-1 second")

	rows, err := gen.New(pool).CreationSessionRevisions(context.Background(), gen.CreationSessionRevisionsParams{
		SessionIds: []pgtype.UUID{live, expired}, WorkspaceIds: []pgtype.UUID{workspace, workspace},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != live {
		t.Errorf("the watch read %d sessions, want only the live one: an expired session that still answers "+
			"with its revision never tells its open streams to look, so they learn it ended only at the next keep-alive",
			len(rows))
	}
}
