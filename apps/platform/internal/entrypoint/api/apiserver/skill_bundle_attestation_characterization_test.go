package apiserver_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func purgeWorkspaceRows(t *testing.T, pool *pgxpool.Pool, workspaceID, userID string) {
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Errorf("cleanup: %v", err)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SET LOCAL skillhub.purge = 'on'`); err != nil {
		t.Errorf("cleanup: %v", err)
		return
	}
	targets, ok := ownerColumns(t, tx)
	if !ok {
		return
	}
	for pass := 0; len(targets) > 0 && pass < len(targets)+1; pass++ {
		if targets, ok = deleteOwnedRowsOnce(t, tx, targets, purgeOwner{workspaceID: workspaceID, userID: userID}); !ok {
			return
		}
	}
	for _, stmt := range []string{`DELETE FROM workspaces WHERE id = $1`, `DELETE FROM users WHERE id = $1`} {
		id := workspaceID
		if strings.Contains(stmt, "users") {
			id = userID
		}
		if _, err := tx.Exec(ctx, stmt, id); err != nil {
			t.Errorf("cleanup %q: %v (still blocked: %v)", stmt, err, targets)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Errorf("cleanup: %v", err)
	}
}

type purgeOwner struct {
	workspaceID, userID string
}

func ownerColumns(t *testing.T, tx pgx.Tx) ([][2]string, bool) {
	rows, err := tx.Query(context.Background(), `SELECT table_name, column_name FROM information_schema.columns
		WHERE table_schema = 'public' AND column_name IN ('workspace_id', 'user_id', 'actor_user_id', 'owner_user_id')`)
	if err != nil {
		t.Errorf("cleanup: %v", err)
		return nil, false
	}
	var targets [][2]string
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			t.Errorf("cleanup: %v", err)
			return nil, false
		}
		targets = append(targets, [2]string{table, column})
	}
	rows.Close()
	return targets, true
}

func deleteOwnedRowsOnce(t *testing.T, tx pgx.Tx, targets [][2]string, owner purgeOwner) ([][2]string, bool) {
	ctx := context.Background()
	var blocked [][2]string
	for _, target := range targets {
		id := owner.workspaceID
		if target[1] != "workspace_id" {
			id = owner.userID
		}
		stmt := fmt.Sprintf(`DELETE FROM %s WHERE %s = $1`, pgx.Identifier{target[0]}.Sanitize(), pgx.Identifier{target[1]}.Sanitize())
		if _, err := tx.Exec(ctx, "SAVEPOINT purge_step"); err != nil {
			t.Errorf("cleanup: %v", err)
			return blocked, false
		}
		if _, err := tx.Exec(ctx, stmt, id); err != nil {
			blocked = append(blocked, target)
			_, _ = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT purge_step")
		}
	}
	return blocked, true
}

func TestABundleOfTheAuthorsOwnSkillsIsNotPublishedWithoutTheirStatement(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, freshName("unattested-alice"))
	t.Cleanup(func() { purgeWorkspaceRows(t, pool, alice.workspaceID, alice.userID) })
	registerPublisher(t, alice, freshName("unattested"))
	bundle, _, skillNames := bundleOfTwo(t, alice, "unattested")

	code, body := postJSON(t, alice, "/me/bundles/"+bundle+"/publication", `{"rights_attested":false}`)

	message, _ := body["error"].(string)
	if code != http.StatusUnprocessableEntity || body["reason"] != "rights_not_attested" || !strings.HasPrefix(message, "成員 "+skillNames[0]) {
		t.Fatalf("publishing without the statement: %d %v, want 422 rights_not_attested naming %s", code, body, skillNames[0])
	}
}
