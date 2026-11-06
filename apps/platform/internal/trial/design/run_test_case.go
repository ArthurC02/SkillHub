package testlab

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type RunOrigin struct {
	SkillID    pgtype.UUID
	SnapshotID pgtype.UUID
}

var errRunOriginNotConfigured = errors.New("testlab: run origin reader not injected")

type RunTestCases struct {
	Lab    *Service
	Origin func(ctx context.Context, workspaceID, runID pgtype.UUID) (RunOrigin, error)
}

func (r *RunTestCases) Save(ctx context.Context, ws identity.Workspace, runID pgtype.UUID, name string) (TestCase, error) {
	if r == nil || r.Origin == nil || r.Lab == nil || r.Lab.LockLiveSkillForCreate == nil {
		return TestCase{}, errRunOriginNotConfigured
	}
	origin, err := r.Origin(ctx, ws.ID, runID)
	if err != nil {
		return TestCase{}, err
	}
	snap, err := r.Lab.ReadSnapshot(ctx, ws.ID, origin.SnapshotID)
	if err != nil {
		return TestCase{}, err
	}
	name, prompt, err := validateDraft(name, snap.UserPrompt)
	if err != nil {
		return TestCase{}, err
	}
	facts, err := r.Lab.SimulatedUserFacts(ctx, ws.ID, []pgtype.UUID{snap.ID})
	if err != nil {
		return TestCase{}, err
	}

	return r.createFromSnapshot(ctx, ws.ID, snapshotDraft{
		skillID: origin.SkillID, name: name, prompt: prompt, snap: snap, userFacts: facts[snap.ID],
	})
}

type snapshotDraft struct {
	skillID      pgtype.UUID
	name, prompt string
	snap         Snapshot
	userFacts    []string
}

func (r *RunTestCases) createFromSnapshot(ctx context.Context, workspaceID pgtype.UUID, d snapshotDraft) (TestCase, error) {
	skillID, name, prompt, snap, userFacts := d.skillID, d.name, d.prompt, d.snap, d.userFacts
	tx, err := r.Lab.Pool.Begin(ctx)
	if err != nil {
		return TestCase{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	found, err := r.Lab.LockLiveSkillForCreate(ctx, tx, workspaceID, skillID)
	if !found && err == nil {
		return TestCase{}, ErrNotFound
	}
	if err != nil {
		return TestCase{}, err
	}
	q := gen.New(tx)
	row, err := q.CreateTestCase(ctx, gen.CreateTestCaseParams{
		WorkspaceID: workspaceID, SkillID: skillID, Name: name, UserPrompt: prompt,
		AcceptanceCriteria: snap.AcceptanceCriteria,
	})
	if err != nil {
		return TestCase{}, err
	}
	if len(snap.Rubric) > 0 {
		if row, err = q.UpdateTestCaseRubric(ctx, gen.UpdateTestCaseRubricParams{
			ID: row.ID, WorkspaceID: workspaceID, Rubric: snap.Rubric,
		}); err != nil {
			return TestCase{}, err
		}
	}
	if len(userFacts) > 0 {
		if row, err = q.UpdateTestCaseSimulatedUserFacts(ctx, gen.UpdateTestCaseSimulatedUserFactsParams{
			ID: row.ID, WorkspaceID: workspaceID, SimulatedUserFacts: userFacts,
		}); err != nil {
			return TestCase{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return TestCase{}, err
	}
	return testCaseOf(row), nil
}
