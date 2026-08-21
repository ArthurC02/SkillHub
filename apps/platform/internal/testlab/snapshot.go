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

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/pgconv"
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
	// Part of the hashed body: two runs judged against different rubrics did not
	// execute the same input, whatever else matched. `omitempty` on purpose — a
	// test case with no rubric hashes exactly as it did before 0026, so the
	// column's arrival does not make every M2 snapshot look like a different one.
	Rubric *Rubric `json:"rubric,omitempty"`
}

// LockDraft takes the row lock that serialises every change to one test case -
// dataset upload and delete, criteria and rubric edits - and returns the locked
// row, workspace scoped.
//
// It is exported because a lock and the invariants it protects have to be owned
// by the same context (ADR-035 B 組, DDD-031). Until then internal/run called
// the gen query directly and this package trusted it to have done so, which
// meant the owner of the invariant could not tell whether the caller had locked
// the right row, or any row at all. A caller that needs the critical section to
// begin before its own checks - run's permission confirmation is the only one -
// asks for it here instead of reaching for the query.
//
// q is taken rather than the pool for the same reason [CreateSnapshot] takes
// one: a lock is worth something only while the transaction that took it is
// open, and a transaction begun here would end at the wrong moment.
//
// Deliberately not a Service method and deliberately not narrowed to the one
// field run reads: a method would need a Service the caller does not have inside
// its own transaction, and the row is what the query already returns. Callers
// that only want to read a draft want [ReadDraft], which does not lock - this is
// the same read with the lock, and the names are a pair on purpose.
//
// Not named after the query it wraps, for a duller reason: devctl's
// automation-check finds call sites by matching `.LockTestCase(` in any file
// that imports db/gen, so a wrapper with that name would be indistinguishable
// from the query at every call site and would re-raise the violation this
// function exists to clear (ADR-035「呼叫點判定仍是文字比對」).
//
// A test case outside workspaceID, or soft-deleted, answers ErrNotFound - the
// same answer as one that does not exist (WS-006).
func LockDraft(ctx context.Context, q *gen.Queries, workspaceID, testCaseID pgtype.UUID) (gen.TestCase, error) {
	tc, err := q.LockTestCase(ctx, gen.LockTestCaseParams{ID: testCaseID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.TestCase{}, ErrNotFound
	}
	return tc, err
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
// together, so two runs that hash the same executed the same input. This is the
// only place a snapshot is written, which is what makes that comparison mean
// anything: a second freezer hashing different bytes would give the same test
// case two hashes depending on which path froze it.
//
// 2026-08-21 (DDD-031, ADR-035 B 組): it takes the parent row lock itself, via
// [LockDraft], rather than reading unlocked and trusting the caller to have
// locked first. The promise in the paragraph above is this package's, and it
// held only for as long as every caller remembered a lock this package could
// not see; internal/run remembering it was never the same thing as this package
// guaranteeing it. Re-locking a row the caller's transaction already holds is a
// no-op in Postgres, so the callers that did remember are unaffected.
func CreateSnapshot(ctx context.Context, q *gen.Queries, workspaceID, testCaseID pgtype.UUID) (gen.TestCaseSnapshot, error) {
	tc, err := LockDraft(ctx, q, workspaceID, testCaseID)
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
			DatasetID:   pgconv.UUIDString(d.ID),
			FileName:    d.FileName,
			ContentHash: d.ContentHash,
			SizeBytes:   d.SizeBytes,
		})
	}

	// The rubric is frozen with the criteria it strengthens (CONTENT-007, iron
	// rule 4). Editing the draft's rubric afterwards changes the *next* run and
	// nothing about the standard this one was judged against.
	rubric, err := DecodeRubric(tc.Rubric)
	if err != nil {
		return gen.TestCaseSnapshot{}, err
	}

	content := snapshotContent{
		UserPrompt:         tc.UserPrompt,
		AcceptanceCriteria: criteria,
		DatasetRefs:        refs,
		Rubric:             rubric,
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
		// Copied, not re-encoded: the frozen bytes are the ones that were stored.
		Rubric: tc.Rubric,
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
