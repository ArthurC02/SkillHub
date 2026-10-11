package eval

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

const (
	MinBatchTrials       = 1
	MaxBatchTrials       = 10
	MaxBatchTestCases    = 50
	BatchAdvanceInterval = time.Minute
)

const (
	BatchLaunching BatchStatus = "launching"
	BatchLaunched  BatchStatus = "launched"
	BatchStopped   BatchStatus = "stopped"
)

type BatchStatus string

func AllBatchStatuses() []BatchStatus {
	return []BatchStatus{BatchLaunching, BatchLaunched, BatchStopped}
}

const stopReasonCreatorGone = "creator_gone"

var (
	ErrBatchShape  = errors.New("an evaluation batch needs 1 to 50 distinct test cases and 1 to 10 trials")
	ErrRunMustWait = errors.New("the workspace cannot start a run right now; the batch waits for its turn")
	errBatchPorts  = errors.New("eval: batch run ports are not injected")
	errItemTaken   = errors.New("eval: another advance already started this trial")
)

type UnconfirmedTestCasesError struct{ TestCaseIDs []pgtype.UUID }

func (e UnconfirmedTestCasesError) Error() string {
	return fmt.Sprintf("%d test cases need their run permissions confirmed first", len(e.TestCaseIDs))
}

type RunRefusedError struct {
	Reason string
	Err    error
}

func (e RunRefusedError) Error() string { return e.Err.Error() }
func (e RunRefusedError) Unwrap() error { return e.Err }

type BatchRun struct {
	WorkspaceID pgtype.UUID
	Actor       pgtype.UUID
	SkillID     pgtype.UUID
	VersionID   pgtype.UUID
	TestCaseID  pgtype.UUID
	SummaryHash string
	Link        func(ctx context.Context, tx pgx.Tx, runID pgtype.UUID) error
}

type ChainEnd struct {
	StartSnapshotID pgtype.UUID
	RunID           pgtype.UUID
	Status          string
	Terminal        bool
}

type BatchPorts struct {
	StartRun              func(ctx context.Context, run BatchRun) (pgtype.UUID, error)
	ConfirmedSummaryHash  func(ctx context.Context, workspaceID, skillID, versionID, testCaseID pgtype.UUID) (string, bool, error)
	ReadChainEnds         func(ctx context.Context, workspaceID pgtype.UUID, runIDs []pgtype.UUID) (map[pgtype.UUID]ChainEnd, error)
	ReadSnapshotContents  func(ctx context.Context, workspaceID pgtype.UUID, snapshotIDs []pgtype.UUID) (map[pgtype.UUID]string, error)
	ReadEndedWithQuestion func(ctx context.Context, workspaceID, runID pgtype.UUID) (bool, error)
}

func (p BatchPorts) ready() error {
	if p.StartRun == nil || p.ConfirmedSummaryHash == nil || p.ReadChainEnds == nil ||
		p.ReadSnapshotContents == nil || p.ReadEndedWithQuestion == nil {
		return errBatchPorts
	}
	return nil
}

type BatchService struct {
	Pool  *pgxpool.Pool
	Ports BatchPorts
}

func (s *BatchService) queries() *gen.Queries { return gen.New(s.Pool) }

type Batch struct {
	ID         pgtype.UUID
	SkillID    pgtype.UUID
	VersionID  pgtype.UUID
	Trials     int
	Status     BatchStatus
	StopReason string
	CreatedAt  time.Time
}

type NewBatch struct {
	WorkspaceID pgtype.UUID
	Actor       pgtype.UUID
	SkillID     pgtype.UUID
	VersionID   pgtype.UUID
	TestCaseIDs []pgtype.UUID
	Trials      int
}

func (n NewBatch) validShape() bool {
	if n.Trials < MinBatchTrials || n.Trials > MaxBatchTrials {
		return false
	}
	if len(n.TestCaseIDs) == 0 || len(n.TestCaseIDs) > MaxBatchTestCases {
		return false
	}
	seen := make(map[pgtype.UUID]bool, len(n.TestCaseIDs))
	for _, id := range n.TestCaseIDs {
		if seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

func (s *BatchService) CreateBatch(ctx context.Context, n NewBatch) (Batch, error) {
	if err := s.Ports.ready(); err != nil {
		return Batch{}, err
	}
	if !n.validShape() {
		return Batch{}, ErrBatchShape
	}
	hashes, err := s.confirmedHashes(ctx, n)
	if err != nil {
		return Batch{}, err
	}
	var created gen.EvaluationBatch
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := gen.New(tx)
		created, err = q.InsertEvaluationBatch(ctx, gen.InsertEvaluationBatchParams{
			WorkspaceID: n.WorkspaceID, SkillID: n.SkillID, SkillVersionID: n.VersionID,
			Trials: int32(n.Trials), CreatedBy: n.Actor,
		})
		if err != nil {
			return err
		}
		for position, testCaseID := range n.TestCaseIDs {
			if err := q.InsertEvaluationBatchTrials(ctx, gen.InsertEvaluationBatchTrialsParams{
				BatchID: created.ID, WorkspaceID: n.WorkspaceID, TestCaseID: testCaseID,
				Position: int32(position), SummaryHash: hashes[position], Trials: int32(n.Trials),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Batch{}, err
	}
	if err := s.AdvanceBatches(ctx, n.WorkspaceID); err != nil {
		slog.Warn("the batch was saved but its first run could not start yet; the periodic advance retries it",
			"batch_id", pgconv.UUIDString(created.ID), "error", err)
	}
	return s.batch(ctx, n.WorkspaceID, created.ID)
}

func (s *BatchService) confirmedHashes(ctx context.Context, n NewBatch) ([]string, error) {
	hashes := make([]string, len(n.TestCaseIDs))
	var unconfirmed []pgtype.UUID
	for i, testCaseID := range n.TestCaseIDs {
		hash, confirmed, err := s.Ports.ConfirmedSummaryHash(ctx, n.WorkspaceID, n.SkillID, n.VersionID, testCaseID)
		if err != nil {
			return nil, err
		}
		if !confirmed {
			unconfirmed = append(unconfirmed, testCaseID)
		}
		hashes[i] = hash
	}
	if len(unconfirmed) > 0 {
		return nil, UnconfirmedTestCasesError{TestCaseIDs: unconfirmed}
	}
	return hashes, nil
}

func (s *BatchService) HasLaunchingBatch(ctx context.Context, workspaceID pgtype.UUID) (bool, error) {
	return s.queries().HasLaunchingBatch(ctx, workspaceID)
}

func (s *BatchService) AdvanceAllBatches(ctx context.Context) error {
	workspaces, err := s.queries().ListWorkspacesWithLaunchingBatches(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, workspaceID := range workspaces {
		errs = append(errs, s.AdvanceBatches(ctx, workspaceID))
	}
	return errors.Join(errs...)
}

func (s *BatchService) AdvanceBatches(ctx context.Context, workspaceID pgtype.UUID) error {
	if err := s.Ports.ready(); err != nil {
		return err
	}
	for {
		started, err := s.startNextTrial(ctx, workspaceID)
		if err != nil || !started {
			return err
		}
	}
}

func (s *BatchService) startNextTrial(ctx context.Context, workspaceID pgtype.UUID) (bool, error) {
	q := s.queries()
	next, err := q.ClaimNextBatchItem(ctx, workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = q.MarkBatchesLaunched(ctx, workspaceID)
		return false, err
	}
	if err != nil {
		return false, err
	}
	if !next.CreatedBy.Valid {
		return true, s.stopBatch(ctx, q, workspaceID, next.BatchID, stopReasonCreatorGone)
	}
	_, err = s.Ports.StartRun(ctx, BatchRun{
		WorkspaceID: workspaceID, Actor: next.CreatedBy, SkillID: next.SkillID,
		VersionID: next.SkillVersionID, TestCaseID: next.TestCaseID, SummaryHash: next.SummaryHash,
		Link: linkTrial(workspaceID, next),
	})
	if errors.Is(err, ErrRunMustWait) {
		return false, nil
	}
	if refused, isRefused := errors.AsType[RunRefusedError](err); isRefused {
		return true, s.stopBatch(ctx, q, workspaceID, next.BatchID, refused.Reason)
	}
	if errors.Is(err, errItemTaken) {
		return true, nil
	}
	return err == nil, err
}

func linkTrial(workspaceID pgtype.UUID, item gen.ClaimNextBatchItemRow) func(context.Context, pgx.Tx, pgtype.UUID) error {
	return func(ctx context.Context, tx pgx.Tx, runID pgtype.UUID) error {
		linked, err := gen.New(tx).LinkBatchItemRun(ctx, gen.LinkBatchItemRunParams{
			RunID: runID, BatchID: item.BatchID, TestCaseID: item.TestCaseID,
			Trial: item.Trial, WorkspaceID: workspaceID,
		})
		if err == nil && linked == 0 {
			return errItemTaken
		}
		return err
	}
}

func (s *BatchService) stopBatch(ctx context.Context, q *gen.Queries, workspaceID, batchID pgtype.UUID, reason string) error {
	_, err := q.StopEvaluationBatch(ctx, gen.StopEvaluationBatchParams{
		ID: batchID, WorkspaceID: workspaceID, StopReason: &reason,
	})
	return err
}

func (s *BatchService) ListBatches(ctx context.Context, workspaceID, skillID pgtype.UUID) ([]Batch, error) {
	rows, err := s.queries().ListEvaluationBatches(ctx, gen.ListEvaluationBatchesParams{
		WorkspaceID: workspaceID, SkillID: skillID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Batch, len(rows))
	for i, row := range rows {
		out[i] = batchOf(row)
	}
	return out, nil
}

func (s *BatchService) batch(ctx context.Context, workspaceID, batchID pgtype.UUID) (Batch, error) {
	row, err := s.queries().GetEvaluationBatch(ctx, gen.GetEvaluationBatchParams{ID: batchID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Batch{}, ErrNotFound
	}
	if err != nil {
		return Batch{}, err
	}
	return batchOf(row), nil
}

func batchOf(row gen.EvaluationBatch) Batch {
	b := Batch{
		ID: row.ID, SkillID: row.SkillID, VersionID: row.SkillVersionID,
		Trials: int(row.Trials), Status: BatchStatus(row.Status), CreatedAt: row.CreatedAt.Time,
	}
	if row.StopReason != nil {
		b.StopReason = *row.StopReason
	}
	return b
}
