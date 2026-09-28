package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func assertLockCurrentPackage(t *testing.T, ctx context.Context, tx pgx.Tx, v Version, want bool, label string) {
	t.Helper()
	current, err := LockCurrentPackage(ctx, tx, v)
	if err != nil || current != want {
		t.Fatalf("%s: current=%v err=%v, want %v/nil", label, current, err, want)
	}
}

func assertCompetingWriteIsBlockedByTheLock(t *testing.T, ctx context.Context, pool *pgxpool.Pool, skillID pgtype.UUID) {
	t.Helper()
	competing, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = competing.Rollback(ctx) }()
	_, err = competing.Exec(ctx, "SELECT id FROM skills WHERE id = $1 FOR UPDATE NOWAIT", skillID)
	var locked *pgconn.PgError
	if !errors.As(err, &locked) || locked.Code != "55P03" {
		t.Fatalf("competing write lock: %v, want lock_not_available", err)
	}
	if err := competing.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestCurrentPackageRemainsLockedUntilProjectionTransactionEnds(t *testing.T) {
	pool := requireRegistryDB(t)
	ctx := context.Background()
	ws, skillID := seedSkill(t, pool, "package-lock")
	first, err := commitVersion(t, pool, ws.ID, skillID, NewVersion{
		ContentHash: "first", PackageObjectKey: "packages/first", Report: passingReport("package-lock"),
	})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	assertLockCurrentPackage(t, ctx, tx, Version{WorkspaceID: ws.ID, SkillID: skillID, ID: first.ID, PackageObjectKey: "packages/first"}, true, "current package")

	assertCompetingWriteIsBlockedByTheLock(t, ctx, pool, skillID)

	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	second, err := commitVersion(t, pool, ws.ID, skillID, NewVersion{
		ContentHash: "second", PackageObjectKey: "packages/second", Report: passingReport("package-lock"),
	})
	if err != nil {
		t.Fatal(err)
	}
	check, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = check.Rollback(ctx) }()
	assertLockCurrentPackage(t, ctx, check, Version{WorkspaceID: ws.ID, SkillID: skillID, ID: first.ID, PackageObjectKey: "packages/first"}, false, "superseded package")
	assertLockCurrentPackage(t, ctx, check, Version{WorkspaceID: ws.ID, SkillID: skillID, ID: second.ID, PackageObjectKey: "packages/second"}, true, "replacement package")
}

func TestCurrentPackageReadFailureIsNotAContentDecision(t *testing.T) {
	pool := requireRegistryDB(t)
	ctx := context.Background()
	ws, skillID := seedSkill(t, pool, "package-read-error")
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	current, err := LockCurrentPackage(ctx, tx, Version{WorkspaceID: ws.ID, SkillID: skillID, ID: skillID, PackageObjectKey: "packages/unknown"})
	if current || !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatalf("current=%v err=%v, want false/ErrTxClosed", current, err)
	}
}
