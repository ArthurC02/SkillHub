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

func (s *Service) UploadDataset(ctx context.Context, ws identity.Workspace, testCaseID pgtype.UUID, fileName string, data []byte) (gen.Dataset, error) {
	name := sanitizeFileName(fileName)
	if name == "" {
		return gen.Dataset{}, fmt.Errorf("%w: 檔案需要有檔名", ErrInvalid)
	}
	if len(data) == 0 {
		return gen.Dataset{}, fmt.Errorf("%w: 檔案是空的", ErrInvalid)
	}
	if len(data) > MaxFileBytes {
		return gen.Dataset{}, fmt.Errorf("%w: 檔案超過 %s", ErrLimitExceeded, humanMB(MaxFileBytes))
	}
	contentType, err := detectContentType(data)
	if err != nil {
		return gen.Dataset{}, err
	}

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
		return gen.Dataset{}, fmt.Errorf("%w: 一個 Test Case 最多 %d 個檔案",
			ErrLimitExceeded, MaxFilesPerTestCase)
	}
	if usage.TotalBytes+int64(len(data)) > MaxTestCaseBytes {
		return gen.Dataset{}, fmt.Errorf("%w: 一個 Test Case 的檔案總量最多 %s",
			ErrLimitExceeded, humanMB(MaxTestCaseBytes))
	}

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
		// Commit definitely failed, so no row exists; let the deferred cleanup remove the object.
		commitAttempted = false
	}
	return ds, commitErr
}

// shouldCompensateCommit reports whether tx.Commit definitely failed rather
// than left the outcome ambiguous, since only a definite failure is safe to
// clean up after.
func shouldCompensateCommit(err error) bool {
	return errors.Is(err, pgx.ErrTxCommitRollback)
}

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

func (s *Service) ListDatasets(ctx context.Context, ws identity.Workspace, testCaseID pgtype.UUID) ([]gen.Dataset, error) {

	if _, err := s.GetTestCase(ctx, ws, testCaseID); err != nil {
		return nil, err
	}
	return gen.New(s.Pool).ListDatasets(ctx, gen.ListDatasetsParams{
		TestCaseID: testCaseID, WorkspaceID: ws.ID,
	})
}

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

func (s *Service) removeDatasetObject(ctx context.Context, ds gen.Dataset) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := s.Store.Remove(cleanupCtx, ds.ObjectKey); err != nil {
		slog.Warn("dataset object not removed; retention sweep will retry", "error", err)
		return
	}
	if err := pgx.BeginFunc(cleanupCtx, s.Pool, func(tx pgx.Tx) error {
		return s.MarkDatasetPurged(cleanupCtx, tx, ds.ID)
	}); err != nil {

		slog.Warn("dataset object removed but cleanup state was not recorded; retention sweep will retry", "error", err)
	}
}

func sanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, `\`, "/")
	name = path.Base(name)
	if name == "." || name == "/" || name == ".." {
		return ""
	}

	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)

	if len(name) > MaxNameBytes {
		name = strings.ToValidUTF8(name[:MaxNameBytes], "")
	}
	return name
}

func newUUID() pgtype.UUID {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return pgtype.UUID{Bytes: b, Valid: true}
}
