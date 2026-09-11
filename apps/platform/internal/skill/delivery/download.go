package packaging

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

var ErrGone = errors.New("this package is no longer available for download")

func (s *Service) ListDownloads(ctx context.Context, ws identity.Workspace) ([]Artifact, error) {
	rows, err := gen.New(s.Pool).ListDownloadArtifacts(ctx, ws.ID)
	if err != nil {
		return nil, err
	}
	out := make([]Artifact, 0, len(rows))
	for _, r := range rows {
		out = append(out, Artifact{
			ArtifactID: pgconv.UUIDString(r.ArtifactID), SkillID: pgconv.UUIDString(r.SkillID),
			SkillVersionID: pgconv.UUIDString(r.SkillVersionID), Target: r.Target,
			FileName: r.FileName, SizeBytes: r.SizeBytes,
			ContentHash: r.ContentHash, ManifestHash: r.ManifestHash,
			Status: r.ScanStatus, ExpiresAt: rfc3339(r.ExpiresAt), CreatedAt: rfc3339(r.CreatedAt),
			DownloadCount: r.DownloadCount, IncludesTestCases: r.IncludesTestCases,
			PackagerVersion: r.PackagerVersion, ProfileVersion: r.ProfileVersion,
			VersionNumber: r.VersionNumber, LatestVersionNumber: r.LatestVersionNumber,
		}.withVersionState().withServeState(r.ExpiresAt.Time, r.PurgedAt.Time))
	}
	return out, nil
}

type DownloadRecord struct {
	DownloadedAt string `json:"downloaded_at"`
	Actor        string `json:"actor"`
}

func (s *Service) ListDownloadRecords(
	ctx context.Context, ws identity.Workspace, id pgtype.UUID,
) ([]DownloadRecord, error) {
	if _, err := s.downloadRow(ctx, ws, id); err != nil {
		return nil, err
	}
	rows, err := gen.New(s.Pool).ListDownloadRecordsForArtifact(ctx,
		gen.ListDownloadRecordsForArtifactParams{WorkspaceID: ws.ID, ArtifactID: id})
	if err != nil {
		return nil, err
	}
	out := make([]DownloadRecord, 0, len(rows))
	for _, r := range rows {

		actor := "deleted user"
		if r.DisplayName != nil && *r.DisplayName != "" {
			actor = *r.DisplayName
		}
		out = append(out, DownloadRecord{DownloadedAt: rfc3339(r.DownloadedAt), Actor: actor})
	}
	return out, nil
}

func (s *Service) GetDownload(ctx context.Context, ws identity.Workspace, id pgtype.UUID) (Artifact, error) {
	row, err := s.downloadRow(ctx, ws, id)
	if err != nil {
		return Artifact{}, err
	}
	return Artifact{
		ArtifactID: pgconv.UUIDString(row.ArtifactID), SkillID: pgconv.UUIDString(row.SkillID),
		SkillVersionID: pgconv.UUIDString(row.SkillVersionID), Target: row.Target,
		FileName: row.FileName, SizeBytes: row.SizeBytes,
		ContentHash: row.ContentHash, ManifestHash: row.ManifestHash,
		Status: row.ScanStatus, ExpiresAt: rfc3339(row.ExpiresAt), CreatedAt: rfc3339(row.CreatedAt),
		DownloadCount: row.DownloadCount, IncludesTestCases: row.IncludesTestCases,
		PackagerVersion: row.PackagerVersion, ProfileVersion: row.ProfileVersion,
		VersionNumber: row.VersionNumber, LatestVersionNumber: row.LatestVersionNumber,
	}.withVersionState().withServeState(row.ExpiresAt.Time, row.PurgedAt.Time), nil
}

func (s *Service) downloadRow(
	ctx context.Context, ws identity.Workspace, id pgtype.UUID,
) (gen.GetDownloadArtifactRow, error) {
	row, err := gen.New(s.Pool).GetDownloadArtifact(ctx, gen.GetDownloadArtifactParams{
		WorkspaceID: ws.ID, ArtifactID: id,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	return row, err
}

func (s *Service) Download(
	ctx context.Context, ws identity.Workspace, id pgtype.UUID,
) (gen.GetDownloadArtifactRow, []byte, error) {
	var none gen.GetDownloadArtifactRow
	if s.Store == nil {
		return none, nil, ErrNoStore
	}
	row, err := s.downloadRow(ctx, ws, id)
	if err != nil {
		return none, nil, err
	}
	if row.ScanStatus != "available" || row.PurgedAt.Valid || !row.ExpiresAt.Time.After(time.Now()) {
		return none, nil, ErrGone
	}
	if reason, _ := gateFlags(row.AccessRestriction, row.Redistribution); reason != "" {
		return none, nil, ErrGone
	}

	data, err := s.Store.Get(ctx, row.ObjectKey)
	if err != nil {

		slog.Warn("download artifact object unreadable",
			"artifact_id", pgconv.UUIDString(row.ArtifactID), "error", err)
		return none, nil, ErrGone
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return none, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	if err := q.InsertDownloadRecord(ctx, gen.InsertDownloadRecordParams{
		WorkspaceID: ws.ID, ArtifactID: row.ArtifactID, ActorUserID: ws.OwnerUserID,
	}); err != nil {
		return none, nil, err
	}

	if err := audit.Log(ctx, tx, audit.Event{
		Actor: ws.OwnerUserID, Workspace: ws.ID,
		Action: audit.ActionArtifactDownload, ResourceType: audit.ResourceArtifact,
		ResourceID: row.ArtifactID,
		Metadata: map[string]any{
			"skill_version_id": pgconv.UUIDString(row.SkillVersionID),
			"target":           row.Target,
			"content_hash":     row.ContentHash,
		},
	}); err != nil {
		return none, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return none, nil, err
	}
	return row, data, nil
}

func (s *Service) DeleteDownload(ctx context.Context, ws identity.Workspace, id pgtype.UUID) error {
	lookup, err := gen.New(s.Pool).GetDownloadArtifactForDelete(ctx, gen.GetDownloadArtifactForDeleteParams{
		ID: id, WorkspaceID: ws.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	conn, err := s.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	locked := false
	defer func() {
		if !locked {
			conn.Release()
			return
		}
		// A session lock that fails to release must not go back to the pool
		// still held; hijacking and closing the connection instead forces the
		// pool to open a fresh one.
		unlockCtx, cancel := context.WithTimeout(context.Background(), objectCleanupTimeout)
		defer cancel()
		if _, err := gen.New(conn).UnlockDownloadObjectKeySession(unlockCtx, downloadObjectLockKey(lookup.ObjectKey)); err != nil {
			slog.Error("download object lock could not be released; closing connection", "error", err)
			_ = conn.Hijack().Close(context.Background())
			return
		}
		conn.Release()
	}()
	if err := gen.New(conn).LockDownloadObjectKeySession(ctx, downloadObjectLockKey(lookup.ObjectKey)); err != nil {
		return err
	}
	locked = true

	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	row, err := q.SoftDeleteDownloadArtifact(ctx, gen.SoftDeleteDownloadArtifactParams{
		ID: id, WorkspaceID: ws.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor: ws.OwnerUserID, Workspace: ws.ID,
		Action: audit.ActionArtifactDelete, ResourceType: audit.ResourceArtifact,
		ResourceID: row.ID,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	if row.PurgedAt.Valid || s.Store == nil {
		return nil
	}

	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), objectCleanupTimeout)
	defer cancel()
	live, err := gen.New(conn).CountArtifactsSharingObject(cleanupCtx, row.ObjectKey)
	if err == nil && live == 0 {
		err = s.Store.Remove(cleanupCtx, row.ObjectKey)
	}
	if err == nil {
		err = pgx.BeginFunc(cleanupCtx, conn, func(tx pgx.Tx) error {
			return s.MarkArtifactPurged(cleanupCtx, tx, row.ID)
		})
	}
	if err != nil {
		slog.Warn("download object not removed; cleanup will retry",
			"object_key", row.ObjectKey, "error", err)
	}
	return nil
}

func (h *Handler) Downloads(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	out, err := h.Svc.ListDownloads(r.Context(), ws)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "download list failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, struct {
		Downloads []Artifact `json:"downloads"`
	}{out})
}

func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	id, ok := artifactID(w, r)
	if !ok {
		return
	}
	art, err := h.Svc.GetDownload(r.Context(), ws, id)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "download not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "download lookup failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, art)
}

func (h *Handler) DownloadRecords(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	id, ok := artifactID(w, r)
	if !ok {
		return
	}
	out, err := h.Svc.ListDownloadRecords(r.Context(), ws, id)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "download not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "download record lookup failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, struct {
		Records []DownloadRecord `json:"records"`
	}{out})
}

func (h *Handler) DownloadContent(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	id, ok := artifactID(w, r)
	if !ok {
		return
	}
	row, data, err := h.Svc.Download(r.Context(), ws, id)
	switch {
	case err == nil:
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrGone):

		httpx.WriteError(w, http.StatusNotFound, "download not found")
		return
	case errors.Is(err, ErrNoStore):
		httpx.WriteError(w, http.StatusServiceUnavailable, err.Error())
		return
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "download failed")
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(sanitizeHeaderValue(row.FileName)))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if _, err := w.Write(data); err != nil {

		slog.Warn("download response truncated", "artifact_id", pgconv.UUIDString(row.ArtifactID), "error", err)
	}
}

func (h *Handler) DeleteDownload(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	id, ok := artifactID(w, r)
	if !ok {
		return
	}
	if err := h.Svc.DeleteDownload(r.Context(), ws, id); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func artifactID(w http.ResponseWriter, r *http.Request) (id pgtype.UUID, ok bool) {
	if err := id.Scan(r.PathValue("artifactId")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "download not found")
		return id, false
	}
	return id, true
}

func sanitizeHeaderValue(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == '"' || r == '\\' {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return "download.zip"
	}
	return string(out)
}
