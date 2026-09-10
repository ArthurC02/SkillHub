package testlab

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type DatasetRef struct {
	DatasetID   string `json:"dataset_id"`
	FileName    string `json:"file_name"`
	ContentHash string `json:"content_hash"`
	SizeBytes   int64  `json:"size_bytes"`
}

type Snapshot struct {
	ID                 pgtype.UUID
	WorkspaceID        pgtype.UUID
	TestCaseID         pgtype.UUID
	UserPrompt         string
	AcceptanceCriteria []byte
	DatasetRefs        []byte
	ContentHash        string
	CreatedAt          pgtype.Timestamptz
	Rubric             []byte
}

func snapshotDTO(row gen.TestCaseSnapshot) Snapshot {
	return Snapshot{
		ID: row.ID, WorkspaceID: row.WorkspaceID, TestCaseID: row.TestCaseID,
		UserPrompt: row.UserPrompt, AcceptanceCriteria: row.AcceptanceCriteria,
		DatasetRefs: row.DatasetRefs, ContentHash: row.ContentHash,
		CreatedAt: row.CreatedAt, Rubric: row.Rubric,
	}
}

type snapshotContent struct {
	UserPrompt         string       `json:"user_prompt"`
	AcceptanceCriteria []Criterion  `json:"acceptance_criteria"`
	DatasetRefs        []DatasetRef `json:"dataset_refs"`

	Rubric *Rubric `json:"rubric,omitempty"`
}

func (*Service) LockDraft(ctx context.Context, tx pgx.Tx, workspaceID, testCaseID pgtype.UUID) (Draft, error) {
	if tx == nil {
		return Draft{}, errPersistenceNotConfigured
	}
	q := gen.New(tx)
	tc, err := lockDraftRow(ctx, q, workspaceID, testCaseID)
	if err != nil {
		return Draft{}, err
	}
	return draftFromRow(ctx, q, workspaceID, tc)
}

func lockDraftRow(ctx context.Context, q *gen.Queries, workspaceID, testCaseID pgtype.UUID) (gen.TestCase, error) {
	tc, err := q.LockTestCase(ctx, gen.LockTestCaseParams{ID: testCaseID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.TestCase{}, ErrNotFound
	}
	return tc, err
}

func (s *Service) ReadSnapshot(ctx context.Context, workspaceID, snapshotID pgtype.UUID) (Snapshot, error) {
	if s == nil || s.Pool == nil {
		return Snapshot{}, errPersistenceNotConfigured
	}
	snap, err := gen.New(s.Pool).GetTestCaseSnapshot(ctx, gen.GetTestCaseSnapshotParams{
		ID: snapshotID, WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Snapshot{}, ErrNotFound
	}
	if err != nil {
		return Snapshot{}, err
	}
	return snapshotDTO(snap), nil
}

func (s *Service) CreateSnapshot(ctx context.Context, tx pgx.Tx, workspaceID, testCaseID pgtype.UUID) (Snapshot, error) {
	if tx == nil {
		return Snapshot{}, errPersistenceNotConfigured
	}
	q := gen.New(tx)
	tc, err := lockDraftRow(ctx, q, workspaceID, testCaseID)
	if err != nil {
		return Snapshot{}, err
	}

	criteria, err := DecodeCriteria(tc.AcceptanceCriteria)
	if err != nil {
		return Snapshot{}, err
	}
	rows, err := q.ListDatasets(ctx, gen.ListDatasetsParams{
		TestCaseID: tc.ID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return Snapshot{}, err
	}

	refs := make([]DatasetRef, 0, len(rows))
	for _, d := range rows {
		refs = append(refs, DatasetRef{
			DatasetID:   pgconv.UUIDString(d.ID),
			FileName:    d.FileName,
			ContentHash: d.ContentHash,
			SizeBytes:   d.SizeBytes,
		})
	}

	rubric, err := DecodeRubric(tc.Rubric)
	if err != nil {
		return Snapshot{}, err
	}

	content := snapshotContent{
		UserPrompt:         tc.UserPrompt,
		AcceptanceCriteria: criteria,
		DatasetRefs:        refs,
		Rubric:             rubric,
	}
	body, err := json.Marshal(content)
	if err != nil {
		return Snapshot{}, err
	}
	sum := sha256.Sum256(body)

	encodedCriteria, err := json.Marshal(criteria)
	if err != nil {
		return Snapshot{}, err
	}
	encodedRefs, err := json.Marshal(refs)
	if err != nil {
		return Snapshot{}, err
	}
	row, err := q.CreateTestCaseSnapshot(ctx, gen.CreateTestCaseSnapshotParams{
		WorkspaceID:        workspaceID,
		TestCaseID:         tc.ID,
		UserPrompt:         tc.UserPrompt,
		AcceptanceCriteria: encodedCriteria,
		DatasetRefs:        encodedRefs,
		ContentHash:        hex.EncodeToString(sum[:]),

		Rubric: tc.Rubric,
	})
	if err != nil {
		return Snapshot{}, err
	}
	return snapshotDTO(row), nil
}

func DecodeDatasetRefs(raw []byte) ([]DatasetRef, error) {
	refs := []DatasetRef{}
	if len(raw) == 0 {
		return refs, nil
	}
	if err := json.Unmarshal(raw, &refs); err != nil {
		return nil, fmt.Errorf("decode dataset refs: %w", err)
	}
	return refs, nil
}
