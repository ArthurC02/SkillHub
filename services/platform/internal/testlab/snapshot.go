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

	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
)

// DatasetRef is one file as a run saw it. The content hash is the part that
// outlives the file: after the user deletes a dataset the run is no longer
// reproducible, but it is still traceable to exactly which bytes it read
// (ADR-003 刪除與可追溯性).
type DatasetRef struct {
	DatasetID   string `json:"dataset_id"`
	FileName    string `json:"file_name"`
	ContentHash string `json:"content_hash"`
	SizeBytes   int64  `json:"size_bytes"`
}

// snapshotContent is the hashed body of a snapshot. A struct rather than a map
// so field order — and therefore the hash — is fixed by the type, not by the
// encoder's iteration.
type snapshotContent struct {
	UserPrompt         string       `json:"user_prompt"`
	AcceptanceCriteria []Criterion  `json:"acceptance_criteria"`
	DatasetRefs        []DatasetRef `json:"dataset_refs"`
}

// CreateSnapshot freezes a test case into the row a run executes (TEST-010).
//
// Contract with the run domain (RUN-001/004):
//
//   - q MUST be transaction-bound (gen.New(tx) or Queries.WithTx(tx)) and MUST be
//     the same transaction that inserts the runs row. The snapshot and the run
//     that points at it commit together or not at all; a run referencing a
//     snapshot that was rolled back, or a snapshot with no run, is the failure
//     this signature exists to make impossible (iron rule 9).
//   - workspaceID MUST come from the session, never from the request (iron rule
//     3). A test case outside it answers ErrNotFound, so a cross-workspace run
//     cannot be started by guessing an id.
//   - Call it once per run. It is deliberately NOT idempotent: a retried attempt
//     of the same run reuses the existing runs.test_case_snapshot_id rather than
//     re-freezing, because re-freezing would capture edits made since the first
//     attempt and silently change what "the same run" means.
//   - The returned row is immutable (0005 trigger). Nothing may update it,
//     including this package.
//
// The returned ContentHash covers prompt, criteria and dataset references
// together, so two runs that hash the same executed the same input.
//
// TODO(M2 第一批合併): db/queries/runs.sql grew a second freezer,
// CreateTestCaseSnapshotFromTestCase, in parallel with this one, and internal/run
// calls that instead. Two implementations of one snapshot is one too many, and
// worse, they hash different bytes — the same test case would get a different
// content_hash depending on which path froze it, which is exactly the comparison
// the hash exists to support. One must go before the batch merges. The SQL one
// also predates 0017 and does not exclude a soft-deleted test case.
func CreateSnapshot(ctx context.Context, q *gen.Queries, workspaceID, testCaseID pgtype.UUID) (gen.TestCaseSnapshot, error) {
	tc, err := q.GetTestCase(ctx, gen.GetTestCaseParams{ID: testCaseID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.TestCaseSnapshot{}, ErrNotFound
	}
	if err != nil {
		return gen.TestCaseSnapshot{}, err
	}

	criteria, err := DecodeCriteria(tc.AcceptanceCriteria)
	if err != nil {
		return gen.TestCaseSnapshot{}, err
	}
	rows, err := q.ListDatasets(ctx, gen.ListDatasetsParams{
		TestCaseID: tc.ID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return gen.TestCaseSnapshot{}, err
	}
	// ListDatasets orders by created_at, so the reference list — and the hash
	// over it — does not depend on how the rows happened to come back.
	refs := make([]DatasetRef, 0, len(rows))
	for _, d := range rows {
		refs = append(refs, DatasetRef{
			DatasetID:   uuidString(d.ID),
			FileName:    d.FileName,
			ContentHash: d.ContentHash,
			SizeBytes:   d.SizeBytes,
		})
	}

	content := snapshotContent{
		UserPrompt:         tc.UserPrompt,
		AcceptanceCriteria: criteria,
		DatasetRefs:        refs,
	}
	body, err := json.Marshal(content)
	if err != nil {
		return gen.TestCaseSnapshot{}, err
	}
	sum := sha256.Sum256(body)

	encodedCriteria, err := json.Marshal(criteria)
	if err != nil {
		return gen.TestCaseSnapshot{}, err
	}
	encodedRefs, err := json.Marshal(refs)
	if err != nil {
		return gen.TestCaseSnapshot{}, err
	}
	return q.CreateTestCaseSnapshot(ctx, gen.CreateTestCaseSnapshotParams{
		WorkspaceID:        workspaceID,
		TestCaseID:         tc.ID,
		UserPrompt:         tc.UserPrompt,
		AcceptanceCriteria: encodedCriteria,
		DatasetRefs:        encodedRefs,
		ContentHash:        hex.EncodeToString(sum[:]),
	})
}

// DecodeDatasetRefs reads a snapshot's dataset_refs column. Exported for the run
// and evaluation domains, so the shape written above is never re-guessed.
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
