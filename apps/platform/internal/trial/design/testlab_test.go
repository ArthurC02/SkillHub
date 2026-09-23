package testlab

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	registry "github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
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

func testCaseServiceWithSkill(pool *pgxpool.Pool, ws identity.Workspace, skillID pgtype.UUID) *Service {
	return &Service{
		Pool: pool,
		ReadSkill: func(_ context.Context, workspaceID, id pgtype.UUID) (SkillFacts, bool, error) {
			if workspaceID != ws.ID || id != skillID {
				return SkillFacts{}, false, nil
			}
			return SkillFacts{Name: "test-skill"}, true, nil
		},
		LockLiveSkillForCreate: func(_ context.Context, _ pgx.Tx, workspaceID, id pgtype.UUID) (bool, error) {
			return workspaceID == ws.ID && id == skillID, nil
		},
	}
}

func TestCreateTestCaseCommitsBeforeConcurrentSkillDeletion(t *testing.T) {
	pool := requireTestLabDB(t)
	ws, skillID := seedWorkspaceWithSkill(t)
	registrySvc := &registry.Service{Pool: pool}
	locked := make(chan struct{})
	release := make(chan struct{})
	svc := &Service{
		Pool: pool,
		LockLiveSkillForCreate: func(ctx context.Context, tx pgx.Tx, workspaceID, id pgtype.UUID) (bool, error) {
			_, found, err := registrySvc.LockLiveWorkspaceSkill(ctx, tx, workspaceID, id)
			if found {
				close(locked)
				select {
				case <-release:
				case <-ctx.Done():
					return false, ctx.Err()
				}
			}
			return found, err
		},
	}

	created := make(chan error, 1)
	go func() {
		_, err := svc.CreateTestCase(t.Context(), ws, skillID, "n", "p")
		created <- err
	}()
	select {
	case <-locked:
	case <-time.After(time.Second):
		t.Fatal("creating a test case did not lock its live skill")
	}

	deleted := make(chan error, 1)
	go func() {
		_, err := pool.Exec(t.Context(), `UPDATE skills SET deleted_at = NOW() WHERE id = $1 AND workspace_id = $2`, skillID, ws.ID)
		deleted <- err
	}()
	select {
	case err := <-deleted:
		t.Fatalf("skill deletion completed before test case creation committed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	if err := <-created; err != nil {
		t.Fatalf("CreateTestCase: %v", err)
	}
	if err := <-deleted; err != nil {
		t.Fatalf("delete skill: %v", err)
	}

	var count int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM test_cases WHERE workspace_id = $1 AND skill_id = $2`, ws.ID, skillID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("test cases after serialized create then delete = %d, want 1", count)
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

func TestAddCriterionAcceptsUpToTheLimitAndRefusesOneOver(t *testing.T) {
	pool := requireTestLabDB(t)
	ws, skillID := seedWorkspaceWithSkill(t)
	svc := testCaseServiceWithSkill(pool, ws, skillID)

	tc, err := svc.CreateTestCase(t.Context(), ws, skillID, "n", "p")
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < MaxCriteria; i++ {
		if _, err := svc.AddCriterion(t.Context(), ws, tc.ID, "c", SourceUser); err != nil {
			t.Fatalf("criterion %d: %v", i+1, err)
		}
	}
	if _, err := svc.AddCriterion(t.Context(), ws, tc.ID, "overflow", SourceUser); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("criterion %d returned %v, want ErrLimitExceeded", MaxCriteria+1, err)
	}
}
