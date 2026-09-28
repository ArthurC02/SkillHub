package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestAPackageIsCurrentOnlyForTheVersionThatWasSeen(t *testing.T) {
	pool := requireRegistryDB(t)
	ctx := context.Background()
	ws, skillID := seedSkill(t, pool, "package-lock-identity")
	t.Cleanup(func() {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Errorf("cleanup: %v", err)
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()
		for _, step := range []struct {
			stmt string
			args []any
		}{
			{`SET LOCAL skillhub.purge = 'on'`, nil},
			{`DELETE FROM audit_events WHERE workspace_id = $1 OR actor_user_id = $2`, []any{ws.ID, ws.OwnerUserID}},
			{`DELETE FROM skill_versions WHERE workspace_id = $1`, []any{ws.ID}},
			{`DELETE FROM skills WHERE workspace_id = $1`, []any{ws.ID}},
			{`DELETE FROM skill_sources WHERE workspace_id = $1`, []any{ws.ID}},
			{`DELETE FROM outbox_events WHERE workspace_id = $1`, []any{ws.ID}},
			{`DELETE FROM workspaces WHERE id = $1`, []any{ws.ID}},
			{`DELETE FROM users WHERE id = $1`, []any{ws.OwnerUserID}},
		} {
			if _, err := tx.Exec(ctx, step.stmt, step.args...); err != nil {
				t.Errorf("cleanup %q: %v", step.stmt, err)
				return
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	current, err := commitVersion(t, pool, ws.ID, skillID, NewVersion{
		ContentHash: "only", PackageObjectKey: "packages/only", Report: passingReport("package-lock-identity"),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		seen       Version
		locksSkill bool
	}{
		{"the same key under another version", Version{WorkspaceID: ws.ID, SkillID: skillID, ID: skillID, PackageObjectKey: "packages/only"}, true},
		{"the same key with no version", Version{WorkspaceID: ws.ID, SkillID: skillID, ID: pgtype.UUID{}, PackageObjectKey: "packages/only"}, false},
		{"the version with no key", Version{WorkspaceID: ws.ID, SkillID: skillID, ID: current.ID}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(ctx) }()

			ok, err := LockCurrentPackage(ctx, tx, tc.seen)

			if err != nil || ok {
				t.Fatalf("current=%v err=%v, want false/nil", ok, err)
			}
			competing, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = competing.Rollback(ctx) }()
			_, err = competing.Exec(ctx, "SELECT id FROM skills WHERE id = $1 FOR UPDATE NOWAIT", skillID)
			var locked *pgconn.PgError
			if gotLocked := errors.As(err, &locked) && locked.Code == "55P03"; gotLocked != tc.locksSkill || (!gotLocked && err != nil) {
				t.Fatalf("competing lock err=%v, want the skill locked=%v", err, tc.locksSkill)
			}
		})
	}
}
