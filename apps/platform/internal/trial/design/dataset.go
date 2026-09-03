package testlab

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

// UploadDataset stores one file against a test case (TEST-004). Every PDM-005
// §5.1 limit is enforced here and only here:
//
//   - ≤ 25 MB per file             — checked against the received bytes
//   - ≤ 100 MB per test case       — checked against the live total, under a lock
//   - ≤ 20 files per test case     — same
//   - allowed types by magic bytes — detectContentType, file name never consulted
//   - 90 day retention             — expires_at written at creation
//
// A workspace advisory lock spans the intent, object write and row commit so an
// account purge cannot take a key snapshot in the middle. Definite failures
// compensate that uniquely keyed object; an ambiguous Commit result retains it
// so a row that actually committed can never point at missing bytes.
func (s *Service) UploadDataset(ctx context.Context, ws identity.Workspace, testCaseID pgtype.UUID, fileName string, data []byte) (gen.Dataset, error) {
	name := sanitizeFileName(fileName)
	if name == "" {
		return gen.Dataset{}, fmt.Errorf("%w: file name is required", ErrInvalid)
	}
	if len(data) == 0 {
		return gen.Dataset{}, fmt.Errorf("%w: file is empty", ErrInvalid)
	}
	if len(data) > MaxFileBytes {
		return gen.Dataset{}, fmt.Errorf("%w: file is larger than %s", ErrLimitExceeded, humanMB(MaxFileBytes))
	}
	contentType, err := detectContentType(data)
	if err != nil {
		return gen.Dataset{}, err
	}

	// Reject a missing or cross-workspace parent before doing external I/O. This
	// is advisory only: LockTestCase below is the authoritative recheck.
	if _, err := s.GetTestCase(ctx, ws, testCaseID); err != nil {
		return gen.Dataset{}, err
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	id := newUUID()
	key := fmt.Sprintf("datasets/%s/%s", pgconv.UUIDString(ws.ID), pgconv.UUIDString(id))
	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return gen.Dataset{}, err
	}
	locked := false
	objectLocked := false
	defer func() {
		if objectLocked {
			unlockCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if _, err := gen.New(conn).UnlockDatasetObjectKeySession(unlockCtx, key); err != nil {
				slog.Error("dataset object lock could not be released; closing connection", "error", err)
				_ = conn.Hijack().Close(context.Background())
				return
			}
		}
		if !locked {
			conn.Release()
			return
		}
		unlockCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := gen.New(conn).UnlockDatasetWorkspaceObjects(unlockCtx, ws.ID); err != nil {
			slog.Error("dataset workspace lock could not be released; closing connection", "error", err)
			_ = conn.Hijack().Close(context.Background())
			return
		}
		conn.Release()
	}()
	if err := gen.New(conn).LockDatasetWorkspaceObjects(ctx, ws.ID); err != nil {
		return gen.Dataset{}, err
	}
	locked = true
	if s.MayStoreObjects == nil {
		return gen.Dataset{}, errors.New("testlab: identity lifecycle read is not configured")
	}
	allowed, err := s.MayStoreObjects(ctx, conn, ws.ID)
	if err != nil {
		return gen.Dataset{}, err
	}
	if !allowed {
		return gen.Dataset{}, ErrNotFound
	}
	if err := gen.New(conn).LockDatasetObjectKeySession(ctx, key); err != nil {
		return gen.Dataset{}, err
	}
	objectLocked = true
	intent, err := gen.New(conn).CreateDatasetCleanupIntent(ctx, gen.CreateDatasetCleanupIntentParams{
		WorkspaceID: ws.ID, ObjectKey: key,
	})
	if err != nil {
		return gen.Dataset{}, err
	}
	// The cleanup intent exists before object I/O. The transaction that publishes
	// the dataset removes it atomically; a process crash, definite rollback, or
	// failed compensation therefore still leaves a durable key for maintenance.
	commitAttempted := false
	defer func() {
		if commitAttempted {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := s.Store.Remove(cleanupCtx, key); err != nil {
			slog.Error("failed to compensate dataset object", "key", key, "error", err)
			return
		}
		if err := gen.New(conn).DeleteDatasetCleanupIntent(cleanupCtx, gen.DeleteDatasetCleanupIntentParams{
			ID: intent.ID, WorkspaceID: ws.ID,
		}); err != nil {
			slog.Error("failed to clear compensated dataset cleanup intent", "key", key, "error", err)
		}
	}()
	if err := s.Store.Put(ctx, key, data); err != nil {
		return gen.Dataset{}, err
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return gen.Dataset{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	// The lock is what makes the two aggregate limits below hold under
	// concurrency; see LockTestCase in db/queries/test_lab.sql.
	tc, err := q.LockTestCase(ctx, gen.LockTestCaseParams{ID: testCaseID, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Dataset{}, ErrNotFound
	}
	if err != nil {
		return gen.Dataset{}, err
	}
	usage, err := q.SumDatasetUsage(ctx, gen.SumDatasetUsageParams{
		TestCaseID: tc.ID, WorkspaceID: ws.ID,
	})
	if err != nil {
		return gen.Dataset{}, err
	}
	if usage.FileCount+1 > MaxFilesPerTestCase {
		return gen.Dataset{}, fmt.Errorf("%w: a test case may hold at most %d files",
			ErrLimitExceeded, MaxFilesPerTestCase)
	}
	if usage.TotalBytes+int64(len(data)) > MaxTestCaseBytes {
		return gen.Dataset{}, fmt.Errorf("%w: a test case may hold at most %s of files in total",
			ErrLimitExceeded, humanMB(MaxTestCaseBytes))
	}

	// Keyed by dataset id, not by content hash: two test cases uploading the same
	// bytes must be able to delete their own copy independently (TEST-004 "檔案只
	// 授權給指定 Run／Test Case"). The id is minted here so the object exists
	// before the row does.
	ds, err := q.CreateDataset(ctx, gen.CreateDatasetParams{
		WorkspaceID: ws.ID,
		TestCaseID:  tc.ID,
		FileName:    name,
		ContentType: contentType,
		SizeBytes:   int64(len(data)),
		ContentHash: hash,
		ObjectKey:   key,
		ExpiresAt:   pgtype.Timestamptz{Time: time.Now().Add(DatasetRetention), Valid: true},
	})
	if err != nil {
		return gen.Dataset{}, err
	}
	if err := q.DeleteDatasetCleanupIntent(ctx, gen.DeleteDatasetCleanupIntentParams{
		ID: intent.ID, WorkspaceID: ws.ID,
	}); err != nil {
		return gen.Dataset{}, err
	}
	commitAttempted = true
	commitErr := tx.Commit(ctx)
	if shouldCompensateCommit(commitErr) {
		// PostgreSQL definitively rolled this transaction back, so there cannot be
		// a row referring to the object and compensation is safe. Other Commit
		// errors are ambiguous and deliberately retain it.
		commitAttempted = false
	}
	return ds, commitErr
}

func shouldCompensateCommit(err error) bool {
	return errors.Is(err, pgx.ErrTxCommitRollback)
}

// ReadDataset reads one live dataset row, workspace scoped.
//
// Exported for internal/run, which needs the current object key to mint the
// per-run read grant (SBX-008). A snapshot's [DatasetRef] carries the content hash
// and never the key, because the hash is what outlives the file and the key is a
// storage fact - so the key has to be re-read at dispatch time, from here.
//
// A deleted row answers ErrNotFound, which is the answer that matters: it is what
// makes a run whose input is gone fail saying so instead of running without it.
type Dataset struct {
	ID          pgtype.UUID
	WorkspaceID pgtype.UUID
	TestCaseID  pgtype.UUID
	FileName    string
	ContentType string
	SizeBytes   int64
	ContentHash string
	ObjectKey   string
}

func datasetDTO(row gen.Dataset) Dataset {
	return Dataset{
		ID: row.ID, WorkspaceID: row.WorkspaceID, TestCaseID: row.TestCaseID,
		FileName: row.FileName, ContentType: row.ContentType,
		SizeBytes: row.SizeBytes, ContentHash: row.ContentHash, ObjectKey: row.ObjectKey,
	}
}

func (s *Service) ReadDataset(ctx context.Context, workspaceID, datasetID pgtype.UUID) (Dataset, error) {
	if s == nil || s.Pool == nil {
		return Dataset{}, errPersistenceNotConfigured
	}
	ds, err := gen.New(s.Pool).GetDataset(ctx, gen.GetDatasetParams{ID: datasetID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Dataset{}, ErrNotFound
	}
	if err != nil {
		return Dataset{}, err
	}
	return datasetDTO(ds), nil
}

// CaseDatasets lists one test case's live files in created_at order, workspace
// scoped, using Test Lab's own pool.
//
// The same rows [Service.ListDatasets] serves, and deliberately not the same
// function: that one is for a session-scoped HTTP request and reads the parent
// first, so a test case in another workspace answers "not found" rather than an
// empty list. Its caller here - internal/packaging - has just listed the parent
// through [Service.CasesForSkill] and would only be re-reading it.
//
// The ordering is the same guarantee [ReadDraft] states: anything hashed over
// these rows does not depend on how they happened to come back.
func (s *Service) CaseDatasets(ctx context.Context, workspaceID, testCaseID pgtype.UUID) ([]Dataset, error) {
	if s == nil || s.Pool == nil {
		return nil, errPersistenceNotConfigured
	}
	rows, err := gen.New(s.Pool).ListDatasets(ctx, gen.ListDatasetsParams{TestCaseID: testCaseID, WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	out := make([]Dataset, len(rows))
	for i, row := range rows {
		out[i] = datasetDTO(row)
	}
	return out, nil
}

// ListDatasets returns one test case's live files.
func (s *Service) ListDatasets(ctx context.Context, ws identity.Workspace, testCaseID pgtype.UUID) ([]gen.Dataset, error) {
	// Scoped read of the parent first, so a test case in another workspace
	// answers "not found" rather than an empty list.
	if _, err := s.GetTestCase(ctx, ws, testCaseID); err != nil {
		return nil, err
	}
	return gen.New(s.Pool).ListDatasets(ctx, gen.ListDatasetsParams{
		TestCaseID: testCaseID, WorkspaceID: ws.ID,
	})
}

// DeleteDataset removes one file before or after a run (TEST-004 "使用者可在執行
// 前移除檔案，並可在執行後主動刪除"). The row is soft-deleted and the object is
// removed; snapshots that referenced the file keep its name and content hash, so
// a past run stays traceable even though it is no longer reproducible (ADR-003).
func (s *Service) DeleteDataset(ctx context.Context, ws identity.Workspace, testCaseID, datasetID pgtype.UUID) (gen.Dataset, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.Dataset{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	ds, err := q.GetDataset(ctx, gen.GetDatasetParams{ID: datasetID, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Dataset{}, ErrNotFound
	}
	if err != nil {
		return gen.Dataset{}, err
	}
	// The URL names the parent, so the parent has to be the one that owns this
	// row. Without this, DELETE /test-cases/<A>/datasets/<a file of B> deleted
	// B's file and answered 200 — same workspace, so never a tenant crossing, but
	// the path asserted a relationship nobody checked. DeleteCriterion next door
	// has always located its parent by {id}; this is the same answer.
	if ds.TestCaseID != testCaseID {
		return gen.Dataset{}, ErrNotFound
	}
	if _, err := q.LockTestCase(ctx, gen.LockTestCaseParams{ID: testCaseID, WorkspaceID: ws.ID}); errors.Is(err, pgx.ErrNoRows) {
		return gen.Dataset{}, ErrNotFound
	} else if err != nil {
		return gen.Dataset{}, err
	}
	ds, err = q.SoftDeleteDataset(ctx, gen.SoftDeleteDatasetParams{ID: datasetID, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Dataset{}, ErrNotFound
	}
	if err != nil {
		return gen.Dataset{}, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        ws.OwnerUserID,
		Workspace:    ws.ID,
		Action:       audit.ActionDatasetDelete,
		ResourceType: audit.ResourceDataset,
		ResourceID:   ds.ID,
		Metadata:     map[string]any{"test_case_id": pgconv.UUIDString(ds.TestCaseID)},
	}); err != nil {
		return gen.Dataset{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.Dataset{}, err
	}

	s.removeDatasetObject(ctx, ds)
	return ds, nil
}

// removeDatasetObject completes the second half of a soft delete. The request
// may be cancelled immediately after its database commit, so cleanup gets a
// short independent context. purged_at is written only after idempotent object
// removal succeeds; otherwise the retention worklist sees the soft-deleted row
// and retries it durably.
func (s *Service) removeDatasetObject(ctx context.Context, ds gen.Dataset) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := s.Store.Remove(cleanupCtx, ds.ObjectKey); err != nil {
		slog.Warn("dataset object not removed; retention sweep will retry", "error", err)
		return
	}
	if err := gen.New(s.Pool).MarkDatasetPurged(cleanupCtx, ds.ID); err != nil {
		// Remove is idempotent, so a later sweep can safely repeat it before
		// recording completion.
		slog.Warn("dataset object removed but cleanup state was not recorded; retention sweep will retry", "error", err)
	}
}

// sanitizeFileName keeps a display name that cannot be read as a path. The name
// is metadata only — the object key is derived from ids, never from this — but
// it is shown in the UI, in the permission summary and inside snapshots, so a
// traversal-shaped name has no business surviving.
func sanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, `\`, "/")
	name = path.Base(name)
	if name == "." || name == "/" || name == ".." {
		return ""
	}
	// Control characters would corrupt any log or terminal that shows the name.
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	// ToValidUTF8 because the cut is by bytes and the name is not ASCII: a bare
	// slice can halve a rune, and this string goes into a text column, the
	// permission summary and a snapshot manifest.
	if len(name) > MaxNameBytes {
		name = strings.ToValidUTF8(name[:MaxNameBytes], "")
	}
	return name
}

// newUUID mints a v4 UUID. pgtype does the formatting, so this needs no new
// dependency; crypto/rand.Read does not fail.
func newUUID() pgtype.UUID {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return pgtype.UUID{Bytes: b, Valid: true}
}
