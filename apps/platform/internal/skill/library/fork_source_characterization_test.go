package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func rolledBackTx(t *testing.T) pgx.Tx {
	t.Helper()
	pool := requireRegistryDB(t)
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

func seedSkillInTx(t *testing.T, tx pgx.Tx, name string) (identity.Workspace, pgtype.UUID) {
	t.Helper()
	ctx := context.Background()
	var ws identity.Workspace
	if err := tx.QueryRow(ctx,
		`WITH u AS (INSERT INTO users (email, display_name) VALUES ($1, $1) RETURNING id)
		 INSERT INTO workspaces (owner_user_id, name) SELECT id, $2 FROM u RETURNING id, owner_user_id`,
		name+"@example.test", name).Scan(&ws.ID, &ws.OwnerUserID); err != nil {
		t.Fatal(err)
	}
	var skillID pgtype.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO skills (workspace_id, name) VALUES ($1, $2) RETURNING id`,
		ws.ID, name).Scan(&skillID); err != nil {
		t.Fatal(err)
	}
	return ws, skillID
}

func addVersionInTx(t *testing.T, tx pgx.Tx, ws identity.Workspace, skillID pgtype.UUID, hash string) Version {
	t.Helper()
	ctx := context.Background()
	root, err := LoadSkill(ctx, tx, ws.ID, skillID)
	if err != nil {
		t.Fatal(err)
	}
	content, err := ContentFromPackage(NewVersion{ContentHash: hash, PackageObjectKey: "packages/" + hash, Report: passingReport("fork-source")}, false)
	if err != nil {
		t.Fatal(err)
	}
	root.AddVersion(content)
	if err := SaveSkill(ctx, tx, root); err != nil {
		t.Fatal(err)
	}
	return root.AddedVersion()
}

func TestAForkReadsTheNewestVersionOfAVisibleLiveSkill(t *testing.T) {
	ctx := context.Background()
	t.Run("own skill with a version", func(t *testing.T) {
		tx := rolledBackTx(t)
		ws, skillID := seedSkillInTx(t, tx, "fork-own")
		version := addVersionInTx(t, tx, ws, skillID, "fork-own-bytes")
		src, srcVer, err := (&Service{}).forkSource(ctx, tx, ws, skillID)
		if err != nil || src.ID != skillID || srcVer.ID != version.ID {
			t.Fatalf("forkSource = %v / %v, %v; want the skill and its version %v", src.ID, srcVer.ID, err, version.ID)
		}
	})
	t.Run("own skill that was taken down", func(t *testing.T) {
		tx := rolledBackTx(t)
		ws, skillID := seedSkillInTx(t, tx, "fork-taken-down")
		addVersionInTx(t, tx, ws, skillID, "fork-taken-down-bytes")
		if _, err := tx.Exec(ctx, `UPDATE skills SET takedown_at = now(), takedown_reason = 'withdrawn' WHERE id = $1`, skillID); err != nil {
			t.Fatal(err)
		}
		if _, _, err := (&Service{}).forkSource(ctx, tx, ws, skillID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
	t.Run("own skill without any version", func(t *testing.T) {
		tx := rolledBackTx(t)
		ws, skillID := seedSkillInTx(t, tx, "fork-empty")
		if _, _, err := (&Service{}).forkSource(ctx, tx, ws, skillID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
	t.Run("another workspace's skill outside the catalogue", func(t *testing.T) {
		tx := rolledBackTx(t)
		owner, skillID := seedSkillInTx(t, tx, "fork-private")
		addVersionInTx(t, tx, owner, skillID, "fork-private-bytes")
		reader, _ := seedSkillInTx(t, tx, "fork-reader-a")
		svc := &Service{CatalogWorkspaces: func(context.Context, gen.DBTX) ([]pgtype.UUID, error) { return nil, nil }}
		if _, _, err := svc.forkSource(ctx, tx, reader, skillID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
	t.Run("a catalogue skill from another workspace", func(t *testing.T) {
		tx := rolledBackTx(t)
		catalogue, skillID := seedSkillInTx(t, tx, "fork-catalogue")
		version := addVersionInTx(t, tx, catalogue, skillID, "fork-catalogue-bytes")
		reader, _ := seedSkillInTx(t, tx, "fork-reader-b")
		svc := &Service{CatalogWorkspaces: func(context.Context, gen.DBTX) ([]pgtype.UUID, error) {
			return []pgtype.UUID{catalogue.ID}, nil
		}}
		src, srcVer, err := svc.forkSource(ctx, tx, reader, skillID)
		if err != nil || src.WorkspaceID != catalogue.ID || srcVer.ID != version.ID {
			t.Fatalf("forkSource = %v / %v, %v; want the catalogue skill's version %v", src.WorkspaceID, srcVer.ID, err, version.ID)
		}
	})
}
