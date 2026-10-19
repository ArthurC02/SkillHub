package batch

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
	BatchPaused    BatchStatus = "paused"
	BatchStopped   BatchStatus = "stopped"
)

type BatchStatus string

func AllBatchStatuses() []BatchStatus {
	return []BatchStatus{BatchLaunching, BatchLaunched, BatchPaused, BatchStopped}
}

const (
	stopReasonCreatorGone   = "creator_gone"
	stopReasonTargetGone    = "target_gone"
	stopReasonPausedTooLong = "paused_too_long"
)

const PausedBatchLimit = 30 * 24 * time.Hour

var (
	ErrNotFound       = errors.New("evaluation not found")
	ErrBatchShape     = errors.New("an evaluation batch needs 1 to 50 distinct test cases and 1 to 10 trials")
	ErrRunMustWait    = errors.New("the workspace cannot start a run right now; the batch waits for its turn")
	ErrNoFurtherRound = errors.New("this trial's conversation cannot take another answer")
	ErrTargetGone     = errors.New("the skill, version or test case this trial runs is gone")
	ErrBatchNotPaused = errors.New("only a paused batch can be resumed")
	errBatchPorts     = errors.New("batch: run ports are not injected")
)

type UnconfirmedTestCasesError struct{ TestCaseIDs []pgtype.UUID }

func (e UnconfirmedTestCasesError) Error() string {
	return fmt.Sprintf("%d test cases need their run permissions confirmed first", len(e.TestCaseIDs))
}

type RunRefusedError struct {
	Reason      string
	AwaitsTopUp bool
	Err         error
}

func (e RunRefusedError) Error() string { return e.Err.Error() }
func (e RunRefusedError) Unwrap() error { return e.Err }

type BatchRun struct {
	WorkspaceID   pgtype.UUID
	Actor         pgtype.UUID
	SkillID       pgtype.UUID
	VersionID     pgtype.UUID
	TestCaseID    pgtype.UUID
	SummaryHash   string
	WithholdSkill bool
	OriginKey     string
}

type ChainEnd struct {
	StartSnapshotID pgtype.UUID
	RunID           pgtype.UUID
	Terminal        bool
	Succeeded       bool
	Failed          bool
}

type BatchPorts struct {
	StartRun             func(ctx context.Context, run BatchRun) (pgtype.UUID, error)
	ConfirmedSummaryHash func(ctx context.Context, workspaceID, skillID, versionID, testCaseID pgtype.UUID) (string, bool, error)
	ReadChainEnds        func(ctx context.Context, workspaceID pgtype.UUID, runIDs []pgtype.UUID) (map[pgtype.UUID]ChainEnd, error)
	ReadSnapshotContents func(ctx context.Context, workspaceID pgtype.UUID, snapshotIDs []pgtype.UUID) (map[pgtype.UUID]string, error)
	ReadQuestionCount    func(ctx context.Context, workspaceID, runID pgtype.UUID) (int, error)
	ReadSimulatedUser    func(ctx context.Context, workspaceID pgtype.UUID, snapshotIDs []pgtype.UUID) (map[pgtype.UUID][]string, error)
	AnswerRun            func(ctx context.Context, answer BatchAnswer) error
	ReadVerdicts         func(ctx context.Context, workspaceID pgtype.UUID, runIDs []pgtype.UUID) (map[pgtype.UUID]Verdict, error)
}

func (p BatchPorts) ready() error {
	if p.StartRun == nil || p.ConfirmedSummaryHash == nil || p.ReadChainEnds == nil ||
		p.ReadSnapshotContents == nil || p.ReadQuestionCount == nil ||
		p.ReadSimulatedUser == nil || p.AnswerRun == nil || p.ReadVerdicts == nil {
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
	ID                pgtype.UUID
	SkillID           pgtype.UUID
	VersionID         pgtype.UUID
	Trials            int
	WithSkillWithheld bool
	Status            BatchStatus
	StopReason        string
	CreatedAt         time.Time
}

type NewBatch struct {
	WorkspaceID       pgtype.UUID
	Actor             pgtype.UUID
	SkillID           pgtype.UUID
	VersionID         pgtype.UUID
	TestCaseIDs       []pgtype.UUID
	Trials            int
	WithSkillWithheld bool
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
			Trials: int32(n.Trials), WithBaseline: n.WithSkillWithheld, CreatedBy: n.Actor,
		})
		if err != nil {
			return err
		}
		for position, testCaseID := range n.TestCaseIDs {
			if err := q.InsertEvaluationBatchTrials(ctx, gen.InsertEvaluationBatchTrialsParams{
				BatchID: created.ID, WorkspaceID: n.WorkspaceID, TestCaseID: testCaseID,
				Position: int32(position), SummaryHash: hashes[position], Trials: int32(n.Trials),
				WithBaseline: n.WithSkillWithheld,
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

func (s *BatchService) HasUnsettledBatch(ctx context.Context, workspaceID pgtype.UUID) (bool, error) {
	return s.queries().HasUnsettledBatch(ctx, workspaceID)
}

func (s *BatchService) AdvanceAllBatches(ctx context.Context) error {
	reason := stopReasonPausedTooLong
	if _, err := s.queries().StopLongPausedBatches(ctx, gen.StopLongPausedBatchesParams{
		StopReason: &reason, PausedFor: pgconv.Interval(PausedBatchLimit),
	}); err != nil {
		return err
	}
	workspaces, err := s.queries().ListWorkspacesWithUnsettledBatches(ctx)
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
		if err != nil {
			return err
		}
		if !started {
			return s.settleBatches(ctx, workspaceID)
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
		return true, stopBatch(ctx, q, workspaceID, next.BatchID, stopReasonCreatorGone)
	}
	runID, err := s.Ports.StartRun(ctx, BatchRun{
		WorkspaceID: workspaceID, Actor: next.CreatedBy, SkillID: next.SkillID,
		VersionID: next.SkillVersionID, TestCaseID: next.TestCaseID, SummaryHash: next.SummaryHash,
		WithholdSkill: next.SkillWithheld, OriginKey: trialOriginKey(next),
	})
	if errors.Is(err, ErrRunMustWait) {
		return false, nil
	}
	if errors.Is(err, ErrTargetGone) {
		return true, stopBatch(ctx, q, workspaceID, next.BatchID, stopReasonTargetGone)
	}
	if refused, isRefused := errors.AsType[RunRefusedError](err); isRefused {
		if refused.AwaitsTopUp {
			return true, pauseBatch(ctx, q, workspaceID, next.BatchID, refused.Reason)
		}
		return true, stopBatch(ctx, q, workspaceID, next.BatchID, refused.Reason)
	}
	if err != nil {
		return false, err
	}
	_, err = q.LinkBatchItemRun(ctx, gen.LinkBatchItemRunParams{
		RunID: runID, BatchID: next.BatchID, TestCaseID: next.TestCaseID,
		Trial: next.Trial, SkillWithheld: next.SkillWithheld, WorkspaceID: workspaceID,
	})
	return err == nil, err
}

func trialOriginKey(item gen.ClaimNextBatchItemRow) string {
	return fmt.Sprintf("evaluation-batch/%s/%s/%d/withheld=%t",
		pgconv.UUIDString(item.BatchID), pgconv.UUIDString(item.TestCaseID), item.Trial, item.SkillWithheld)
}

func stopBatch(ctx context.Context, q *gen.Queries, workspaceID, batchID pgtype.UUID, reason string) error {
	_, err := q.StopEvaluationBatch(ctx, gen.StopEvaluationBatchParams{
		ID: batchID, WorkspaceID: workspaceID, StopReason: &reason,
	})
	return err
}

func pauseBatch(ctx context.Context, q *gen.Queries, workspaceID, batchID pgtype.UUID, reason string) error {
	_, err := q.PauseEvaluationBatch(ctx, gen.PauseEvaluationBatchParams{
		ID: batchID, WorkspaceID: workspaceID, PauseReason: &reason,
	})
	return err
}

func (s *BatchService) ResumeBatch(ctx context.Context, workspaceID, skillID, batchID pgtype.UUID) error {
	q := s.queries()
	resumed, err := q.ResumeEvaluationBatch(ctx, gen.ResumeEvaluationBatchParams{
		ID: batchID, WorkspaceID: workspaceID, SkillID: skillID,
	})
	if err != nil || resumed == 1 {
		return err
	}
	row, err := q.GetEvaluationBatch(ctx, gen.GetEvaluationBatchParams{ID: batchID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.SkillID != skillID) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return ErrBatchNotPaused
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
		Trials: int(row.Trials), WithSkillWithheld: row.WithBaseline, Status: BatchStatus(row.Status), CreatedAt: row.CreatedAt.Time,
	}
	if row.StopReason != nil {
		b.StopReason = *row.StopReason
	}
	return b
}
