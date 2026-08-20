package packaging

// The download surface of contracts/openapi/public.yaml:
//
//	GET    /downloads                        the workspace's packages
//	GET    /downloads/{artifactId}           one of them
//	DELETE /downloads/{artifactId}           delete one of one's own
//	GET    /downloads/{artifactId}/content   the bytes
//
// The platform streams the bytes; it does not hand out a pre-signed URL
// (packaging-design §7.1). Three consequences, and all three are the reason:
// workspace scope is the session middleware already in place rather than a
// second authorization scheme; the URL is not secret material, so it can appear
// in a log and in the audit event a download writes, which a pre-signed URL
// never could; and the status, expiry and licensing checks happen in this
// handler on every request rather than once at signing time, where a hold
// applied inside the TTL would not have stopped anything. Bytes crossing the API
// process is movement, not execution (iron rule 1).
//
// One successful download writes two rows in one transaction (iron rule 9): a
// download record (WS-004, what the owner reads) and an audit event (CORE-008,
// what compliance keeps). They are deliberately not one row — see internal/audit.

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/httpx"
)

// ErrGone is every reason the bytes are not being served, folded into one:
// not available, expired, purged, or held back since it was built. The endpoint
// answers 404 for all of them and for "not yours" alike, because the existence
// of one workspace's artifact is exactly what must not leak (WS-006). An owner
// who wants to tell the cases apart reads GET /downloads/{id}, which still
// describes the row.
var ErrGone = errors.New("this package is no longer available for download")

// ListDownloads is GET /downloads: the workspace's packages, newest first.
func (s *Service) ListDownloads(ctx context.Context, ws gen.Workspace) ([]Artifact, error) {
	rows, err := gen.New(s.Pool).ListDownloadArtifacts(ctx, ws.ID)
	if err != nil {
		return nil, err
	}
	out := make([]Artifact, 0, len(rows))
	for _, r := range rows {
		out = append(out, Artifact{
			ArtifactID: uuidString(r.ArtifactID), SkillID: uuidString(r.SkillID),
			SkillVersionID: uuidString(r.SkillVersionID), Target: r.Target,
			FileName: r.FileName, SizeBytes: r.SizeBytes,
			ContentHash: r.ContentHash, ManifestHash: r.ManifestHash,
			Status: r.ScanStatus, ExpiresAt: rfc3339(r.ExpiresAt), CreatedAt: rfc3339(r.CreatedAt),
			DownloadCount: r.DownloadCount, IncludesTestCases: r.IncludesTestCases,
			PackagerVersion: r.PackagerVersion, ProfileVersion: r.ProfileVersion,
		})
	}
	return out, nil
}

// DownloadRecord is one line of WS-004's "誰、何時": one download that happened.
type DownloadRecord struct {
	DownloadedAt string `json:"downloaded_at"`
	Actor        string `json:"actor"`
}

// ListDownloadRecords is GET /downloads/{artifactId}/records.
//
// The artifact is read first, so an id belonging to another workspace answers 404
// rather than an empty list — an empty list would say "this exists and has never
// been downloaded" about somebody else's package.
func (s *Service) ListDownloadRecords(
	ctx context.Context, ws gen.Workspace, id pgtype.UUID,
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
		// A purged account keeps its download records de-identified (PDM-006 §6.1),
		// so the name can be gone while the row is still true.
		actor := "deleted user"
		if r.DisplayName != nil && *r.DisplayName != "" {
			actor = *r.DisplayName
		}
		out = append(out, DownloadRecord{DownloadedAt: rfc3339(r.DownloadedAt), Actor: actor})
	}
	return out, nil
}

// GetDownload is GET /downloads/{artifactId}.
func (s *Service) GetDownload(ctx context.Context, ws gen.Workspace, id pgtype.UUID) (Artifact, error) {
	row, err := s.downloadRow(ctx, ws, id)
	if err != nil {
		return Artifact{}, err
	}
	return Artifact{
		ArtifactID: uuidString(row.ArtifactID), SkillID: uuidString(row.SkillID),
		SkillVersionID: uuidString(row.SkillVersionID), Target: row.Target,
		FileName: row.FileName, SizeBytes: row.SizeBytes,
		ContentHash: row.ContentHash, ManifestHash: row.ManifestHash,
		Status: row.ScanStatus, ExpiresAt: rfc3339(row.ExpiresAt), CreatedAt: rfc3339(row.CreatedAt),
		DownloadCount: row.DownloadCount, IncludesTestCases: row.IncludesTestCases,
		PackagerVersion: row.PackagerVersion, ProfileVersion: row.ProfileVersion,
	}, nil
}

func (s *Service) downloadRow(
	ctx context.Context, ws gen.Workspace, id pgtype.UUID,
) (gen.GetDownloadArtifactRow, error) {
	row, err := gen.New(s.Pool).GetDownloadArtifact(ctx, gen.GetDownloadArtifactParams{
		WorkspaceID: ws.ID, ArtifactID: id,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, ErrNotFound
	}
	return row, err
}

// Download is GET /downloads/{artifactId}/content: the servability checks, the
// bytes, and the two rows one download owes.
//
// The checks are re-run here rather than trusted from build time. `available` is
// the state ADR-003's quarantine releases into and this is the endpoint that
// makes "only available is served" a condition a test can hold; expiry and
// purge are facts about the object; and the two licensing locks are asked again
// because a hold applied since the package was built must stop the copy that
// already exists from going out.
func (s *Service) Download(
	ctx context.Context, ws gen.Workspace, id pgtype.UUID,
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

	// Read before recording. A download record is a claim that the user got the
	// file, and writing one for bytes that were not there would make the history
	// wrong in the direction nobody checks.
	//
	// ponytail: the whole package is materialised (32 MiB ceiling,
	// skillpkg.MaxZipBytes). Give ObjectStore an io.ReadCloser variant if packages or
	// concurrency grow past what that costs in memory.
	data, err := s.Store.Get(ctx, row.ObjectKey)
	if err != nil {
		// The object is gone while the row still claims it is there. The
		// reconciler is what corrects the row (04 丙-9); this request only has to
		// avoid promising what it cannot deliver.
		slog.Warn("download artifact object unreadable",
			"artifact_id", uuidString(row.ArtifactID), "error", err)
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
	// Identifiers and outcome only, never content (iron rule 11). Which package
	// and which target, because "who took a copy of what" is the question a
	// review of the download trail asks.
	if err := audit.Log(ctx, q, audit.Event{
		Actor: ws.OwnerUserID, Workspace: ws.ID,
		Action: audit.ActionArtifactDownload, ResourceType: audit.ResourceArtifact,
		ResourceID: row.ArtifactID,
		Metadata: map[string]any{
			"skill_version_id": uuidString(row.SkillVersionID),
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

// DeleteDownload is DELETE /downloads/{artifactId}, and it is idempotent: an id
// that is not there, is already deleted, or belongs to somebody else all reach
// the same 204. The caller's intent — this artifact must not exist — holds in
// every one of those cases, and answering 404 to a repeat of a delete that
// worked reports success as failure.
//
// The row is soft-deleted rather than removed. download_records point at it and
// outlive it by design (WS-004: "you downloaded this" stays true after the file
// is gone), and download_artifacts carries 0027's immutability trigger. What
// 02:SEC-006 asks for is that deleted content stops appearing in ordinary access
// surfaces, and deleted_at is exactly that.
func (s *Service) DeleteDownload(ctx context.Context, ws gen.Workspace, id pgtype.UUID) error {
	tx, err := s.Pool.Begin(ctx)
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
	if err := audit.Log(ctx, q, audit.Event{
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
		return nil // the bytes are already gone, or there is no store to ask
	}
	// After the commit, so the count cannot include the row just deleted and a
	// rollback can never leave a live row pointing at removed bytes.
	//
	// Object keys are content addressed and a shared object is what governance.sql
	// spares package objects for. A download package's manifest carries its own
	// version ids so a collision is close to impossible — but this delete is not
	// recoverable, and "close to impossible" is not the standard for that.
	shared, err := gen.New(s.Pool).CountArtifactsSharingObject(ctx, row.ObjectKey)
	if err != nil || shared > 0 {
		if err != nil {
			slog.Warn("could not check whether a download object is shared; leaving the bytes",
				"object_key", row.ObjectKey, "error", err)
		}
		return nil
	}
	// Best effort, as the dataset delete already does: the row is gone either
	// way, Remove is idempotent, and the retention sweep reaches the same key.
	if err := s.Store.Remove(ctx, row.ObjectKey); err != nil {
		slog.Warn("download object not removed; the retention sweep will retry",
			"object_key", row.ObjectKey, "error", err)
	}
	return nil
}

// --- HTTP --------------------------------------------------------------------

// Downloads handles GET /downloads.
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

// Download handles GET /downloads/{artifactId}.
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

// DownloadRecords handles GET /downloads/{artifactId}/records.
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

// DownloadContent handles GET /downloads/{artifactId}/content.
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
		// One answer for every reason, deliberately. Unlike GET /api/skills/{id}
		// /files, where the skill is public and search lists it anyway, here the
		// artifact is private to one workspace and its existence is the thing that
		// must not leak.
		httpx.WriteError(w, http.StatusNotFound, "download not found")
		return
	case errors.Is(err, ErrNoStore):
		httpx.WriteError(w, http.StatusServiceUnavailable, err.Error())
		return
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "download failed")
		return
	}

	// quoteFileName, not the raw name: it reaches a browser's save dialog, and a
	// quote or a newline in it would let the file name rewrite the header.
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(sanitizeHeaderValue(row.FileName)))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	if _, err := w.Write(data); err != nil {
		// The status line is long gone; there is nothing to tell the client. The
		// download record stays, which is correct: the platform did hand the bytes
		// over, and a truncated transfer is the network's account of it, not the
		// platform's.
		slog.Warn("download response truncated", "artifact_id", uuidString(row.ArtifactID), "error", err)
	}
}

// DeleteDownload handles DELETE /downloads/{artifactId}.
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

// artifactID parses the path id. A malformed one answers 404 rather than 400 for
// the reason every unknown id does: the caller learns nothing either way.
func artifactID(w http.ResponseWriter, r *http.Request) (id pgtype.UUID, ok bool) {
	if err := id.Scan(r.PathValue("artifactId")); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "download not found")
		return id, false
	}
	return id, true
}

// sanitizeHeaderValue drops what has no business in a header. File names are
// built by the packager from a skill name, so this is depth rather than the only
// guard — but the value is echoed to a browser and the packager is not the only
// thing that will ever write one.
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
