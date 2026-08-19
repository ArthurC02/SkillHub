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

	"github.com/ArthurC02/skillhub/apps/platform/internal/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
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
// The object lands in storage before the row commits. A rolled back transaction
// therefore leaves an orphan object rather than a row pointing at nothing, and
// the key is per-dataset (not content addressed) so deleting one dataset can
// never remove bytes another dataset still references.
func (s *Service) UploadDataset(ctx context.Context, ws gen.Workspace, testCaseID pgtype.UUID, fileName string, data []byte) (gen.Dataset, error) {
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

	tx, err := s.Pool.Begin(ctx)
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

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	// Keyed by dataset id, not by content hash: two test cases uploading the same
	// bytes must be able to delete their own copy independently (TEST-004 "檔案只
	// 授權給指定 Run／Test Case"). The id is minted here so the object exists
	// before the row does.
	id := newUUID()
	key := fmt.Sprintf("datasets/%s/%s", uuidString(ws.ID), uuidString(id))
	if err := s.Store.Put(ctx, key, data); err != nil {
		return gen.Dataset{}, err
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
	return ds, tx.Commit(ctx)
}

// ListDatasets returns one test case's live files.
func (s *Service) ListDatasets(ctx context.Context, ws gen.Workspace, testCaseID pgtype.UUID) ([]gen.Dataset, error) {
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
func (s *Service) DeleteDataset(ctx context.Context, ws gen.Workspace, datasetID pgtype.UUID) (gen.Dataset, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.Dataset{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	ds, err := q.SoftDeleteDataset(ctx, gen.SoftDeleteDatasetParams{ID: datasetID, WorkspaceID: ws.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Dataset{}, ErrNotFound
	}
	if err != nil {
		return gen.Dataset{}, err
	}
	if err := audit.Log(ctx, q, audit.Event{
		Actor:        ws.OwnerUserID,
		Workspace:    ws.ID,
		Action:       audit.ActionDatasetDelete,
		ResourceType: audit.ResourceDataset,
		ResourceID:   ds.ID,
		Metadata:     map[string]any{"test_case_id": uuidString(ds.TestCaseID)},
	}); err != nil {
		return gen.Dataset{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.Dataset{}, err
	}

	s.removeObject(ctx, ds.ObjectKey)
	return ds, nil
}

// removeObject deletes the stored bytes after the row is already gone. A failure
// is logged and swallowed: the file is unreachable either way, Remove is
// idempotent, and the retention sweep passes over the same key later.
func (s *Service) removeObject(ctx context.Context, key string) {
	if err := s.Store.Remove(ctx, key); err != nil {
		slog.Warn("dataset object not removed; retention sweep will retry", "error", err)
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
	if len(name) > MaxNameBytes {
		name = name[:MaxNameBytes]
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

func uuidString(u pgtype.UUID) string {
	v, _ := u.Value()
	s, _ := v.(string)
	return s
}
