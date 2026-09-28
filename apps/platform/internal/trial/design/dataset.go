package testlab

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

func (s *Service) UploadDataset(ctx context.Context, ws identity.Workspace, testCaseID pgtype.UUID, fileName string, data []byte) (Dataset, error) {
	file, err := acceptDatasetFile(fileName, data)
	if err != nil {
		return Dataset{}, err
	}
	if _, err := s.GetTestCase(ctx, ws, testCaseID); err != nil {
		return Dataset{}, err
	}

	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return Dataset{}, err
	}
	locks := &datasetObjectLocks{conn: conn, workspaceID: ws.ID, key: file.key(ws.ID)}
	defer locks.release()
	intentID, err := s.reserveDatasetObject(ctx, locks)
	if err != nil {
		return Dataset{}, err
	}
	return s.writeDataset(ctx, locks, testCaseID, file, intentID)
}

type datasetFile struct {
	name        string
	contentType string
	hash        string
	data        []byte
	id          pgtype.UUID
}

func acceptDatasetFile(fileName string, data []byte) (datasetFile, error) {
	name := sanitizeFileName(fileName)
	if name == "" {
		return datasetFile{}, fmt.Errorf("%w: 檔案需要有檔名", ErrInvalid)
	}
	if len(data) == 0 {
		return datasetFile{}, fmt.Errorf("%w: 檔案是空的", ErrInvalid)
	}
	if len(data) > MaxFileBytes {
		return datasetFile{}, fmt.Errorf("%w: 檔案超過 %s", ErrLimitExceeded, humanMB(MaxFileBytes))
	}
	contentType, err := detectContentType(data)
	if err != nil {
		return datasetFile{}, err
	}
	sum := sha256.Sum256(data)
	return datasetFile{name: name, contentType: contentType, hash: hex.EncodeToString(sum[:]), data: data, id: newUUID()}, nil
}

func (f datasetFile) key(workspaceID pgtype.UUID) string {
	return fmt.Sprintf("datasets/%s/%s", pgconv.UUIDString(workspaceID), pgconv.UUIDString(f.id))
}

type datasetObjectLocks struct {
	conn          *pgxpool.Conn
	workspaceID   pgtype.UUID
	key           string
	workspaceHeld bool
	objectHeld    bool
}

func (l *datasetObjectLocks) holdWorkspace(ctx context.Context) error {
	if err := gen.New(l.conn).LockDatasetWorkspaceObjects(ctx, l.workspaceID); err != nil {
		return err
	}
	l.workspaceHeld = true
	return nil
}

func (l *datasetObjectLocks) holdObject(ctx context.Context) error {
	if err := gen.New(l.conn).LockDatasetObjectKeySession(ctx, l.key); err != nil {
		return err
	}
	l.objectHeld = true
	return nil
}

func (l *datasetObjectLocks) release() {
	if l.objectHeld {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := gen.New(l.conn).UnlockDatasetObjectKeySession(unlockCtx, l.key); err != nil {
			slog.Error("dataset object lock could not be released; closing connection", "error", err)
			_ = l.conn.Hijack().Close(context.Background())
			return
		}
	}
	if !l.workspaceHeld {
		l.conn.Release()
		return
	}
	unlockCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := gen.New(l.conn).UnlockDatasetWorkspaceObjects(unlockCtx, l.workspaceID); err != nil {
		slog.Error("dataset workspace lock could not be released; closing connection", "error", err)
		_ = l.conn.Hijack().Close(context.Background())
		return
	}
	l.conn.Release()
}

func (s *Service) reserveDatasetObject(ctx context.Context, locks *datasetObjectLocks) (pgtype.UUID, error) {
	if err := locks.holdWorkspace(ctx); err != nil {
		return pgtype.UUID{}, err
	}
	if s.MayStoreObjects == nil {
		return pgtype.UUID{}, errors.New("testlab: identity lifecycle read is not configured")
	}
	allowed, err := s.MayStoreObjects(ctx, locks.conn, locks.workspaceID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	if !allowed {
		return pgtype.UUID{}, ErrNotFound
	}
	if err := locks.holdObject(ctx); err != nil {
		return pgtype.UUID{}, err
	}
	intent, err := gen.New(locks.conn).CreateDatasetCleanupIntent(ctx, gen.CreateDatasetCleanupIntentParams{
		WorkspaceID: locks.workspaceID, ObjectKey: locks.key, Hold: pgconv.Interval(datasetCleanupHold),
	})
	if err != nil {
		return pgtype.UUID{}, err
	}
	return intent.ID, nil
}

func (s *Service) writeDataset(
	ctx context.Context, locks *datasetObjectLocks, testCaseID pgtype.UUID, file datasetFile, intentID pgtype.UUID,
) (Dataset, error) {
	conn, workspaceID, key := locks.conn, locks.workspaceID, locks.key
	keepObject := false
	defer func() {
		if !keepObject {
			s.compensateDatasetObject(ctx, conn, workspaceID, key, intentID)
		}
	}()
	if err := s.Store.Put(ctx, key, file.data); err != nil {
		return Dataset{}, err
	}
	ds, commitAttempted, err := recordDataset(ctx, locks, testCaseID, file, intentID)
	keepObject = commitAttempted && !shouldCompensateCommit(err)
	return datasetDTO(ds), err
}

func (s *Service) compensateDatasetObject(ctx context.Context, conn *pgxpool.Conn, workspaceID pgtype.UUID, key string, intentID pgtype.UUID) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := s.Store.Remove(cleanupCtx, key); err != nil {
		slog.Error("failed to compensate dataset object", "key", key, "error", err)
		return
	}
	if err := gen.New(conn).DeleteDatasetCleanupIntent(cleanupCtx, gen.DeleteDatasetCleanupIntentParams{
		ID: intentID, WorkspaceID: workspaceID,
	}); err != nil {
		slog.Error("failed to clear compensated dataset cleanup intent", "key", key, "error", err)
	}
}

func recordDataset(
	ctx context.Context, locks *datasetObjectLocks, testCaseID pgtype.UUID, file datasetFile, intentID pgtype.UUID,
) (row gen.Dataset, commitAttempted bool, err error) {
	workspaceID, key := locks.workspaceID, locks.key
	tx, err := locks.conn.Begin(ctx)
	if err != nil {
		return gen.Dataset{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	tc, err := q.LockTestCase(ctx, gen.LockTestCaseParams{ID: testCaseID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Dataset{}, false, ErrNotFound
	}
	if err != nil {
		return gen.Dataset{}, false, err
	}
	if err := checkTestCaseRoom(ctx, q, workspaceID, tc.ID, len(file.data)); err != nil {
		return gen.Dataset{}, false, err
	}
	ds, err := q.CreateDataset(ctx, gen.CreateDatasetParams{
		WorkspaceID: workspaceID,
		TestCaseID:  tc.ID,
		FileName:    file.name,
		ContentType: file.contentType,
		SizeBytes:   int64(len(file.data)),
		ContentHash: file.hash,
		ObjectKey:   key,
		ExpiresAt:   pgtype.Timestamptz{Time: time.Now().Add(DatasetRetention), Valid: true},
	})
	if err != nil {
		return gen.Dataset{}, false, err
	}
	if err := q.DeleteDatasetCleanupIntent(ctx, gen.DeleteDatasetCleanupIntentParams{
		ID: intentID, WorkspaceID: workspaceID,
	}); err != nil {
		return gen.Dataset{}, false, err
	}
	return ds, true, tx.Commit(ctx)
}

func checkTestCaseRoom(ctx context.Context, q *gen.Queries, workspaceID, testCaseID pgtype.UUID, size int) error {
	usage, err := q.SumDatasetUsage(ctx, gen.SumDatasetUsageParams{
		TestCaseID: testCaseID, WorkspaceID: workspaceID,
	})
	if err != nil {
		return err
	}
	if usage.FileCount+1 > MaxFilesPerTestCase {
		return fmt.Errorf("%w: 一個 Test Case 最多 %d 個檔案",
			ErrLimitExceeded, MaxFilesPerTestCase)
	}
	if usage.TotalBytes+int64(size) > MaxTestCaseBytes {
		return fmt.Errorf("%w: 一個 Test Case 的檔案總量最多 %s",
			ErrLimitExceeded, humanMB(MaxTestCaseBytes))
	}
	return nil
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
	ExpiresAt   pgtype.Timestamptz
}

func datasetDTO(row gen.Dataset) Dataset {
	return Dataset{
		ID: row.ID, WorkspaceID: row.WorkspaceID, TestCaseID: row.TestCaseID,
		FileName: row.FileName, ContentType: row.ContentType,
		SizeBytes: row.SizeBytes, ContentHash: row.ContentHash, ObjectKey: row.ObjectKey, ExpiresAt: row.ExpiresAt,
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

func (s *Service) ListDatasets(ctx context.Context, ws identity.Workspace, testCaseID pgtype.UUID) ([]Dataset, error) {

	if _, err := s.GetTestCase(ctx, ws, testCaseID); err != nil {
		return nil, err
	}
	return s.CaseDatasets(ctx, ws.ID, testCaseID)
}

func (s *Service) DeleteDataset(ctx context.Context, ws identity.Workspace, testCaseID, datasetID pgtype.UUID) (Dataset, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Dataset{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	ds, err := q.GetDataset(ctx, gen.GetDatasetParams{ID: datasetID, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Dataset{}, ErrNotFound
	}
	if err != nil {
		return Dataset{}, err
	}

	if ds.TestCaseID != testCaseID {
		return Dataset{}, ErrNotFound
	}
	if _, err := q.LockTestCase(ctx, gen.LockTestCaseParams{ID: testCaseID, WorkspaceID: ws.ID}); errors.Is(err, pgx.ErrNoRows) {
		return Dataset{}, ErrNotFound
	} else if err != nil {
		return Dataset{}, err
	}
	ds, err = q.SoftDeleteDataset(ctx, gen.SoftDeleteDatasetParams{ID: datasetID, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Dataset{}, ErrNotFound
	}
	if err != nil {
		return Dataset{}, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        ws.OwnerUserID,
		Workspace:    ws.ID,
		Action:       audit.ActionDatasetDelete,
		ResourceType: audit.ResourceDataset,
		ResourceID:   ds.ID,
		Metadata:     map[string]any{"test_case_id": pgconv.UUIDString(ds.TestCaseID)},
	}); err != nil {
		return Dataset{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Dataset{}, err
	}

	s.removeDatasetObject(ctx, ds)
	return datasetDTO(ds), nil
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
	return pgtype.UUID{Bytes: uuid.New(), Valid: true}
}
