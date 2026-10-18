package registry

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
)

type unreachableStore struct{}

func (unreachableStore) Get(context.Context, string) ([]byte, error) {
	return nil, errors.New("the object store was reached")
}

func TestAPackageOfATakenDownSkillIsNotCurrent(t *testing.T) {
	pool := requireRegistryDB(t)
	ctx := context.Background()
	ws, skillID := seedSkill(t, pool, "package-lock-takedown")
	t.Cleanup(func() { cleanupSeededSkillWorkspace(t, ctx, pool, ws) })
	current, err := commitVersion(t, pool, ws.ID, skillID, NewVersion{
		ContentHash: "taken", PackageObjectKey: "packages/taken", Report: passingReport("package-lock-takedown"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE skills SET takedown_at = now(), takedown_reason = 'withdrawn' WHERE id = $1`, skillID); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	ok, err := LockCurrentPackage(ctx, tx, current)
	if err != nil || ok {
		t.Fatalf("current=%v err=%v for a taken-down skill, want false/nil", ok, err)
	}
}

func TestADiffRefusesAVersionOfAnotherSkill(t *testing.T) {
	pool := requireRegistryDB(t)
	ctx := context.Background()
	ws, skillID := seedSkill(t, pool, "diff-own-skill")
	t.Cleanup(func() { cleanupSeededSkillWorkspace(t, ctx, pool, ws) })
	var otherID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO skills (workspace_id, name) VALUES ($1, 'diff-other-skill') RETURNING id`, ws.ID).Scan(&otherID); err != nil {
		t.Fatal(err)
	}
	own, err := commitVersion(t, pool, ws.ID, skillID, NewVersion{
		ContentHash: "own", PackageObjectKey: "packages/own", Report: passingReport("diff-own-skill"),
	})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := commitVersion(t, pool, ws.ID, otherID, NewVersion{
		ContentHash: "foreign", PackageObjectKey: "packages/foreign", Report: passingReport("diff-other-skill"),
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{Pool: pool, Store: unreachableStore{}}

	_, err = s.DiffVersions(ctx, identity.Workspace{ID: ws.ID}, skillID, VersionRange{From: foreign.ID, To: own.ID})

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("diffing another skill's version: err = %v, want ErrNotFound", err)
	}
}

func TestATextDiffStopsAtOneMiBPerFile(t *testing.T) {
	atCap := strings.Repeat("a", maxDiffFileBytes)
	if textDiff("big.txt", []byte(atCap), []byte(atCap[:maxDiffFileBytes-1]+"b")) == "" {
		t.Error("two files of exactly 1 MiB got no diff")
	}
	if textDiff("big.txt", []byte(atCap), []byte(atCap+"b")) != "" {
		t.Error("a file one byte over 1 MiB was diffed")
	}
}
