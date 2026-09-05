package testlab

// TestCreateTestCaseWithCriteria (05 R-46 (b)): the creation flow confirms a
// brief and its acceptance criteria together, and materialize is expected to
// turn the confirmed words into a real Test Case in the same transaction.
//
// Needs PostgreSQL like the rest of this file's siblings; TestMain in
// dataset_ownership_integration_test.go provides testLabPool and skips this
// file's tests when SKILLHUB_TEST_DATABASE_URL is unset.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
)

func seedWorkspaceWithSkill(t *testing.T) (ws identity.Workspace, skillID pgtype.UUID) {
	t.Helper()
	pool := requireTestLabDB(t)
	ctx := context.Background()
	tag := fmt.Sprintf("%s-%d", strings.ReplaceAll(t.Name(), "/", "-"), time.Now().UnixNano())
	err := pool.QueryRow(ctx, `
		WITH u AS (
			INSERT INTO users (email, display_name) VALUES ($1 || '@example.test', $1)
			RETURNING id
		), w AS (
			INSERT INTO workspaces (owner_user_id, name) SELECT id, $1 FROM u
			RETURNING id, owner_user_id
		), s AS (
			INSERT INTO skills (workspace_id, name) SELECT id, $1 FROM w
			RETURNING id, workspace_id
		)
		SELECT w.id, w.owner_user_id, s.id FROM w, s`, tag).
		Scan(&ws.ID, &ws.OwnerUserID, &skillID)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	return ws, skillID
}

// testCaseServiceWithSkill wires ReadSkill to the given workspace/skill pair
// only — any other (workspace, skill) reads as not found, the same as a real
// Registry-backed ReadSkill would answer for a skill outside scope (WS-006).
func testCaseServiceWithSkill(pool *pgxpool.Pool, ws identity.Workspace, skillID pgtype.UUID) *Service {
	return &Service{
		Pool: pool,
		ReadSkill: func(_ context.Context, workspaceID, id pgtype.UUID) (SkillFacts, bool, error) {
			if workspaceID != ws.ID || id != skillID {
				return SkillFacts{}, false, nil
			}
			return SkillFacts{Name: "test-skill"}, true, nil
		},
	}
}

func TestCreateTestCaseWithCriteriaMintsConfirmedUserCriteria(t *testing.T) {
	pool := requireTestLabDB(t)
	ws, skillID := seedWorkspaceWithSkill(t)
	svc := testCaseServiceWithSkill(pool, ws, skillID)

	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()

	tc, err := svc.CreateTestCaseWithCriteria(t.Context(), tx, ws, skillID,
		"acceptance test", "run the thing", []string{"輸出摘要含所有輸入重點"})
	if err != nil {
		t.Fatalf("CreateTestCaseWithCriteria: %v", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}

	criteria, err := DecodeCriteria(tc.AcceptanceCriteria)
	if err != nil {
		t.Fatal(err)
	}
	if len(criteria) != 1 {
		t.Fatalf("got %d criteria, want 1", len(criteria))
	}
	c := criteria[0]
	if c.Text != "輸出摘要含所有輸入重點" || c.Source != SourceUser || c.ID == "" {
		t.Fatalf("criterion mis-recorded: %+v", c)
	}
	if c.ConfirmedAt == nil {
		t.Fatal("confirmed_at is nil, want the confirmation timestamp")
	}
}

func TestCreateTestCaseWithCriteriaRejectsTooManyOrBlank(t *testing.T) {
	pool := requireTestLabDB(t)
	ws, skillID := seedWorkspaceWithSkill(t)
	svc := testCaseServiceWithSkill(pool, ws, skillID)
	ctx := t.Context()

	run := func(criteria []string) error {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		_, err = svc.CreateTestCaseWithCriteria(ctx, tx, ws, skillID, "n", "p", criteria)
		return err
	}

	tooMany := make([]string, 13)
	for i := range tooMany {
		tooMany[i] = "c"
	}
	if err := run(tooMany); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("13 criteria returned %v, want ErrLimitExceeded", err)
	}
	if err := run([]string{"  "}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("blank criterion returned %v, want ErrInvalid", err)
	}
}
