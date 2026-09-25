package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

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
	current, err := LockCurrentPackage(ctx, tx, ws.ID, skillID, first.ID, "packages/first")
	if err != nil || !current {
		t.Fatalf("current=%v err=%v, want true/nil", current, err)
	}
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
	current, err = LockCurrentPackage(ctx, check, ws.ID, skillID, first.ID, "packages/first")
	if err != nil || current {
		t.Fatalf("superseded package: current=%v err=%v, want false/nil", current, err)
	}
	current, err = LockCurrentPackage(ctx, check, ws.ID, skillID, second.ID, "packages/second")
	if err != nil || !current {
		t.Fatalf("replacement package: current=%v err=%v, want true/nil", current, err)
	}
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
	current, err := LockCurrentPackage(ctx, tx, ws.ID, skillID, skillID, "packages/unknown")
	if current || !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatalf("current=%v err=%v, want false/ErrTxClosed", current, err)
	}
}
