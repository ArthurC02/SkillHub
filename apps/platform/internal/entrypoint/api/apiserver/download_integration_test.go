// The download surface through the real route table (WS-002, WS-004, CORE-007,
// CORE-008, SEC-006) plus the storage reconciler of 04 丙-9.
//
// What these hold the implementation to is mostly what it REFUSES: a package
// that is not available, not the caller's, expired, deleted or held back must
// answer 404 and must not spend bytes. The one thing a download is allowed to do
// is leave two rows behind — a download record and an audit event — and the
// tests assert they are two rows in two tables, because merging them is the
// mistake 03:CORE-008 names by hand.
package apiserver_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objreconcile"
"github.com/ArthurC02/skillhub/apps/platform/internal/skill/delivery"
"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

// newSweep builds the reconciler the way cmd/worker does: the two row
// corrections belong to packaging and testlab and reach the generic scanner by
// injection (ADR-033 clearance path 4), so a test that omitted them would only
// prove the fail-closed guard.
func newSweep(pool *pgxpool.Pool, store objreconcile.ObjectStore) *objreconcile.Service {
	packagingSvc := &packaging.Service{Pool: pool}
	testlabSvc := &testlab.Service{Pool: pool}
	packagingCandidates := func(list func(context.Context, int32) ([]packaging.ReconcileCandidate, error)) objreconcile.ListFunc {
		return func(ctx context.Context, limit int32) ([]objreconcile.Candidate, error) {
			rows, err := list(ctx, limit)
			if err != nil {
				return nil, err
			}
			out := make([]objreconcile.Candidate, len(rows))
			for i, row := range rows {
				out[i] = objreconcile.Candidate{ID: row.ID, WorkspaceID: row.WorkspaceID, ObjectKey: row.ObjectKey}
			}
			return out, nil
		}
	}
	return &objreconcile.Service{
		Pool: pool, Store: store,
		ListExpiredArtifacts: packagingCandidates(packagingSvc.ExpiredReconcileCandidates),
		ListClaimedArtifacts: packagingCandidates(packagingSvc.ClaimedReconcileCandidates),
		ListClaimedDatasets: func(ctx context.Context, limit int32) ([]objreconcile.Candidate, error) {
			rows, err := testlabSvc.ClaimedReconcileCandidates(ctx, limit)
			if err != nil {
				return nil, err
			}
			out := make([]objreconcile.Candidate, len(rows))
			for i, row := range rows {
				out[i] = objreconcile.Candidate{ID: row.ID, WorkspaceID: row.WorkspaceID, ObjectKey: row.ObjectKey}
			}
			return out, nil
		},
		RecordArtifactPurged: packagingSvc.MarkArtifactPurged,
		RecordDatasetLost:    testlabSvc.MarkDatasetObjectLost,
	}
}

// Exists completes objreconcile.ObjectStore over the in-memory store the rest of
// these tests already use. A key that is not in the map is missing, which is the
// whole condition the reconciler exists to notice.
func (s packageStore) Exists(_ context.Context, key string) (bool, error) {
	_, ok := s[key]
	return ok, nil
}

type downloadView struct {
	ArtifactID        string `json:"artifact_id"`
	SkillID           string `json:"skill_id"`
	SkillVersionID    string `json:"skill_version_id"`
	Target            string `json:"target"`
	FileName          string `json:"file_name"`
	SizeBytes         int64  `json:"size_bytes"`
	ContentHash       string `json:"content_hash"`
	ManifestHash      string `json:"manifest_hash"`
	Status            string `json:"status"`
	ExpiresAt         string `json:"expires_at"`
	DownloadCount     int64  `json:"download_count"`
	IncludesTestCases bool   `json:"includes_test_cases"`
}

// buildDownload packages one skill and returns the artifact the endpoint made.
func buildDownload(t *testing.T, a *api, pool *pgxpool.Pool, c *client, name string) downloadView {
	t.Helper()
	skillID, versionID := packagedSkill(t, a, pool, c, name)
	code, body := postJSON(t, c, packagingPath(skillID, versionID), `{"target":"standard"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST packaging: got %d, body %v", code, body)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	var out downloadView
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.ArtifactID == "" {
		t.Fatalf("packaging produced no artifact id: %v", body)
	}
	return out
}

func (c *client) fetchContent(t *testing.T, artifactID string) (*http.Response, []byte) {
	t.Helper()
	resp, err := c.Get(c.base + "/downloads/" + artifactID + "/content")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, data
}

func (c *client) listDownloads(t *testing.T) []downloadView {
	t.Helper()
	var out struct {
		Downloads []downloadView `json:"downloads"`
	}
	if code := getJSON(t, c.Client, c.base+"/downloads", &out); code != http.StatusOK {
		t.Fatalf("GET /downloads: got %d", code)
	}
	return out.Downloads
}

func countRows(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func downloadRecordCount(t *testing.T, pool *pgxpool.Pool, artifactID string) int64 {
	t.Helper()
	return countRows(t, pool, "SELECT count(*) FROM download_records WHERE artifact_id = $1",
		mustUUID(t, artifactID))
}

func auditCount(t *testing.T, pool *pgxpool.Pool, action, resourceID string) int64 {
	t.Helper()
	return countRows(t, pool,
		"SELECT count(*) FROM audit_events WHERE action = $1 AND resource_id = $2",
		action, mustUUID(t, resourceID))
}

// --- the happy path, and the two rows it owes --------------------------------

// One download hands over the bytes and leaves a record AND an audit event —
// two rows in two tables. 03:CORE-008 forbids merging them by name: the record
// is a product feature the owner reads and outlives the file, the audit event is
// a compliance record that stores identifiers only and is kept for 400 days.
func TestDownloadingServesTheBytesAndWritesBothARecordAndAnAuditEvent(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "downloader")
	art := buildDownload(t, a, pool, c, "served-skill")

	resp, data := c.fetchContent(t, art.ArtifactID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET content: got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/zip" {
		t.Errorf("Content-Type: got %q", got)
	}
	// The name the artifact reports, quoted, as an attachment: the browser must
	// save the file rather than try to render it.
	if got := resp.Header.Get("Content-Disposition"); got != `attachment; filename="`+art.FileName+`"` {
		t.Errorf("Content-Disposition: got %q, want the artifact's own file name %q", got, art.FileName)
	}
	if int64(len(data)) != art.SizeBytes {
		t.Errorf("served %d bytes, artifact says %d", len(data), art.SizeBytes)
	}
	// The bytes are the object, not a re-build: the same assertion the packaging
	// tests make about content_hash naming the stored object.
	if want := a.packages["downloads/"+c.workspaceID+"/"+art.ContentHash+".zip"]; string(data) != string(want) {
		t.Error("the served bytes are not the stored object")
	}

	if n := downloadRecordCount(t, pool, art.ArtifactID); n != 1 {
		t.Errorf("download_records: got %d rows, want 1", n)
	}
	if n := auditCount(t, pool, "artifact.download", art.ArtifactID); n != 1 {
		t.Errorf("audit_events for the download: got %d, want 1", n)
	}

	// Twice downloaded is twice recorded, and the count the UI shows comes from
	// those rows rather than from a counter column that could drift.
	if resp, _ := c.fetchContent(t, art.ArtifactID); resp.StatusCode != http.StatusOK {
		t.Fatalf("second GET content: got %d", resp.StatusCode)
	}
	if n := downloadRecordCount(t, pool, art.ArtifactID); n != 2 {
		t.Errorf("download_records after two downloads: got %d, want 2", n)
	}
	var single downloadView
	if code := getJSON(t, c.Client, c.base+"/downloads/"+art.ArtifactID, &single); code != http.StatusOK {
		t.Fatalf("GET /downloads/{id}: got %d", code)
	}
	if single.DownloadCount != 2 {
		t.Errorf("download_count: got %d, want 2", single.DownloadCount)
	}
	if list := c.listDownloads(t); len(list) != 1 || list[0].ArtifactID != art.ArtifactID {
		t.Errorf("GET /downloads: got %+v", list)
	}
}

// --- what must not be served -------------------------------------------------

// Quarantine is the state ADR-003 releases an artifact out of, and this is the
// endpoint that makes "only available is served" a condition rather than a habit
// of the packaging code.
func TestAnArtifactThatIsNotAvailableIsNotServed(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "quarantined")
	art := buildDownload(t, a, pool, c, "quarantined-skill")

	if _, err := pool.Exec(context.Background(),
		"UPDATE artifacts SET scan_status = 'quarantined' WHERE id = $1",
		mustUUID(t, art.ArtifactID)); err != nil {
		t.Fatal(err)
	}
	resp, _ := c.fetchContent(t, art.ArtifactID)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("content of a quarantined artifact: got %d, want 404", resp.StatusCode)
	}
	if n := downloadRecordCount(t, pool, art.ArtifactID); n != 0 {
		t.Errorf("a refused download still wrote %d records", n)
	}
	// The owner can still read the row and see why: the 404 hides the bytes, not
	// the artifact's own account of itself.
	var single downloadView
	if code := getJSON(t, c.Client, c.base+"/downloads/"+art.ArtifactID, &single); code != http.StatusOK {
		t.Fatalf("GET /downloads/{id}: got %d", code)
	}
	if single.Status != "quarantined" {
		t.Errorf("status: got %q, want quarantined", single.Status)
	}
}

// Existence is private (WS-006): a stranger gets the same 404 for somebody
// else's artifact as for one that was never built, on every route.
func TestAnotherWorkspaceCannotSeeOrFetchAnArtifact(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "download-owner")
	stranger := a.login(t, "download-stranger")
	art := buildDownload(t, a, pool, owner, "private-skill")

	if code := stranger.status(t, http.MethodGet, "/downloads/"+art.ArtifactID); code != http.StatusNotFound {
		t.Errorf("GET as a stranger: got %d, want 404", code)
	}
	if resp, _ := stranger.fetchContent(t, art.ArtifactID); resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET content as a stranger: got %d, want 404", resp.StatusCode)
	}
	if list := stranger.listDownloads(t); len(list) != 0 {
		t.Errorf("a stranger's download list: got %+v, want empty", list)
	}
	// And the stranger's delete is a no-op rather than a way to destroy somebody
	// else's file: 204 says nothing about whether it existed, and the row stays.
	if code := stranger.status(t, http.MethodDelete, "/downloads/"+art.ArtifactID); code != http.StatusNoContent {
		t.Errorf("DELETE as a stranger: got %d, want 204", code)
	}
	if resp, _ := owner.fetchContent(t, art.ArtifactID); resp.StatusCode != http.StatusOK {
		t.Error("a stranger's DELETE removed the owner's artifact")
	}
}

// A hold applied after the package was built has to stop the copy that already
// exists from going out. This is the argument against pre-signed URLs made
// executable (packaging-design §7.1): a signature minted before the hold would
// still work, a check in the handler does not.
func TestALicensingHoldAppliedAfterPackagingStopsTheDownload(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "held-downloader")
	art := buildDownload(t, a, pool, c, "held-skill")

	for _, tc := range []struct {
		name, sql string
	}{
		{"access_restriction", "UPDATE skills SET access_restriction = 'license-review' WHERE id = $1"},
		{"redistribution", "UPDATE skills SET redistribution = 'blocked' WHERE id = $1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := pool.Exec(context.Background(), tc.sql, mustUUID(t, art.SkillID)); err != nil {
				t.Fatal(err)
			}
			resp, _ := c.fetchContent(t, art.ArtifactID)
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("content under a %s hold: got %d, want 404", tc.name, resp.StatusCode)
			}
			if _, err := pool.Exec(context.Background(),
				"UPDATE skills SET access_restriction = NULL, redistribution = 'allowed' WHERE id = $1",
				mustUUID(t, art.SkillID)); err != nil {
				t.Fatal(err)
			}
		})
	}
	if resp, _ := c.fetchContent(t, art.ArtifactID); resp.StatusCode != http.StatusOK {
		t.Error("lifting both locks did not restore the download")
	}
}

// --- deletion (SEC-006, CORE-007) --------------------------------------------

// Deleting is idempotent, the bytes go, the row stops appearing, and the record
// of having downloaded it survives — it says "you downloaded this", which stays
// true after the file is gone (WS-004).
func TestDeletingADownloadIsIdempotentAndKeepsTheRecord(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "deleter")
	art := buildDownload(t, a, pool, c, "deleted-skill")

	if resp, _ := c.fetchContent(t, art.ArtifactID); resp.StatusCode != http.StatusOK {
		t.Fatal("the artifact was not downloadable before the delete")
	}
	objectKey := "downloads/" + c.workspaceID + "/" + art.ContentHash + ".zip"

	for i := range 2 {
		if code := c.status(t, http.MethodDelete, "/downloads/"+art.ArtifactID); code != http.StatusNoContent {
			t.Fatalf("DELETE #%d: got %d, want 204", i+1, code)
		}
	}
	if _, ok := a.packages[objectKey]; ok {
		t.Error("the stored object survived the delete")
	}
	if resp, _ := c.fetchContent(t, art.ArtifactID); resp.StatusCode != http.StatusNotFound {
		t.Error("content is still served after the delete")
	}
	if code := c.status(t, http.MethodGet, "/downloads/"+art.ArtifactID); code != http.StatusNotFound {
		t.Error("the deleted artifact is still readable")
	}
	// 02:SEC-006 「完成後不再出現在一般存取介面」.
	if list := c.listDownloads(t); len(list) != 0 {
		t.Errorf("the deleted artifact is still listed: %+v", list)
	}
	if n := downloadRecordCount(t, pool, art.ArtifactID); n != 1 {
		t.Errorf("download_records after the delete: got %d, want the record to survive", n)
	}
	if n := auditCount(t, pool, "artifact.delete", art.ArtifactID); n != 1 {
		t.Errorf("audit_events for the delete: got %d, want 1", n)
	}
}

// --- retention (SEC-006) ------------------------------------------------------

// An expired package stops being served the moment it expires, and the sweep
// then removes the bytes — but the row stays in the history, because "it
// expired" and "it never existed" are different answers to 02:WS-002.
func TestAnExpiredArtifactIsNotServedAndItsBytesAreSweptAway(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "expirer")
	art := buildDownload(t, a, pool, c, "expiring-skill")
	objectKey := "downloads/" + c.workspaceID + "/" + art.ContentHash + ".zip"

	if _, err := pool.Exec(context.Background(),
		"UPDATE artifacts SET expires_at = now() - interval '1 day' WHERE id = $1",
		mustUUID(t, art.ArtifactID)); err != nil {
		t.Fatal(err)
	}
	if resp, _ := c.fetchContent(t, art.ArtifactID); resp.StatusCode != http.StatusNotFound {
		t.Fatal("an expired artifact is still being served")
	}

	sweep := newSweep(pool, a.packages)
	if err := sweep.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.packages[objectKey]; ok {
		t.Error("the expired object survived the sweep")
	}
	if list := c.listDownloads(t); len(list) != 1 {
		t.Errorf("the expired artifact left the history: %+v", list)
	}

	// Idempotent, iron rule 9: a second sweep finds nothing left to do and does
	// not fail trying to remove an object that is already gone.
	if err := sweep.Sweep(context.Background()); err != nil {
		t.Fatalf("the second sweep failed: %v", err)
	}
	if n := countRows(t, pool,
		"SELECT count(*) FROM artifacts WHERE id = $1 AND purged_at IS NOT NULL",
		mustUUID(t, art.ArtifactID)); n != 1 {
		t.Error("the swept artifact was not marked purged")
	}
}

// --- the object existence reconciler (04 丙-9) --------------------------------

// The database says available, storage has nothing. One round records a sighting
// and changes nothing — an object store answering 404 during a write must not
// cost somebody their file. The second consecutive round is what acts.
func TestTheReconcilerNeedsTwoRoundsBeforeItMarksAMissingObject(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "reconciled")
	art := buildDownload(t, a, pool, c, "vanishing-skill")
	delete(a.packages, "downloads/"+c.workspaceID+"/"+art.ContentHash+".zip") // the bytes go behind the platform's back

	sweep := newSweep(pool, a.packages)
	if err := sweep.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, pool,
		"SELECT count(*) FROM object_reconcile_sightings WHERE resource_id = $1 AND rounds = 1",
		mustUUID(t, art.ArtifactID)); n != 1 {
		t.Fatal("the first round recorded no sighting")
	}
	if n := countRows(t, pool,
		"SELECT count(*) FROM artifacts WHERE id = $1 AND purged_at IS NULL",
		mustUUID(t, art.ArtifactID)); n != 1 {
		t.Error("one round was enough to mark the row; it must not be")
	}

	if err := sweep.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, pool,
		"SELECT count(*) FROM artifacts WHERE id = $1 AND purged_at IS NOT NULL",
		mustUUID(t, art.ArtifactID)); n != 1 {
		t.Fatal("two rounds did not mark the row whose object is gone")
	}
	if resp, _ := c.fetchContent(t, art.ArtifactID); resp.StatusCode != http.StatusNotFound {
		t.Error("the endpoint still offers bytes that are not there")
	}
	// Content becoming unavailable without anybody asking is exactly what the
	// audit trail is for, and the event has no actor because nobody acted.
	if n := auditCount(t, pool, "storage.object_missing", art.ArtifactID); n != 1 {
		t.Errorf("audit_events for the lost object: got %d, want 1", n)
	}
	// Nothing left to count once the row agrees with storage.
	if n := countRows(t, pool, "SELECT count(*) FROM object_reconcile_sightings WHERE resource_id = $1",
		mustUUID(t, art.ArtifactID)); n != 0 {
		t.Error("the sighting was not cleared after the row was marked")
	}
}

// An object that comes back resets the count, which is what makes `rounds` mean
// "missing two rounds running" rather than "missing twice at some point"
// (0021's bookkeeping, same reasoning).
func TestAReturningObjectResetsTheSightingCount(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "flapping")
	art := buildDownload(t, a, pool, c, "flapping-skill")
	key := "downloads/" + c.workspaceID + "/" + art.ContentHash + ".zip"
	bytes := a.packages[key]

	sweep := newSweep(pool, a.packages)
	delete(a.packages, key)
	if err := sweep.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	a.packages[key] = bytes
	if err := sweep.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, pool, "SELECT count(*) FROM object_reconcile_sightings WHERE resource_id = $1",
		mustUUID(t, art.ArtifactID)); n != 0 {
		t.Fatal("the sighting survived the object coming back")
	}

	delete(a.packages, key)
	if err := sweep.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, pool,
		"SELECT count(*) FROM artifacts WHERE id = $1 AND purged_at IS NOT NULL",
		mustUUID(t, art.ArtifactID)); n != 0 {
		t.Error("the count did not start over, so one round after a flap was enough to mark the row")
	}
	if resp, _ := c.fetchContent(t, art.ArtifactID); resp.StatusCode != http.StatusNotFound {
		t.Log("note: the object is missing again, so the download correctly fails at fetch time")
	}
}

// The 丙-9 half that started it: `RunInputsStillAvailable` reads
// `datasets.deleted_at`, so a dataset whose file is gone must end up with that
// column set — otherwise the comparison screen keeps offering a re-run that
// fails when it fetches the file.
func TestTheReconcilerCorrectsADatasetThatClaimsAMissingFile(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "dataset-loser")

	_, testCaseID := newTestCase(t, pool, a, c, "reconcile")
	datasetID, objectKey := seedDataset(t, pool, a, c, testCaseID)

	sweep := newSweep(pool, a.packages)
	delete(a.packages, objectKey)
	for range 2 {
		if err := sweep.Sweep(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if n := countRows(t, pool, "SELECT count(*) FROM datasets WHERE id = $1 AND deleted_at IS NOT NULL",
		mustUUID(t, datasetID)); n != 1 {
		t.Fatal("the dataset still claims a file that is not there")
	}
	if n := auditCount(t, pool, "storage.object_missing", datasetID); n != 1 {
		t.Errorf("audit_events for the lost dataset: got %d, want 1", n)
	}
}

// seedDataset uploads one file through the real endpoint and returns its id and
// object key. Real, because the reconciler reads the same columns the upload
// writes and a hand-made row could disagree with them.
func seedDataset(t *testing.T, pool *pgxpool.Pool, a *api, c *client, testCaseID string) (id, objectKey string) {
	t.Helper()
	code, out := c.upload(t, "/test-cases/"+testCaseID+"/datasets", "notes.txt",
		[]byte("some example content\n"))
	if code != http.StatusCreated {
		t.Fatalf("POST dataset: got %d, body %v", code, out)
	}
	id, _ = out["dataset_id"].(string)
	if id == "" {
		id, _ = out["id"].(string)
	}
	if id == "" {
		t.Fatalf("the upload returned no dataset id: %v", out)
	}
	if err := pool.QueryRow(context.Background(),
		"SELECT object_key FROM datasets WHERE id = $1", mustUUID(t, id)).Scan(&objectKey); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.packages[objectKey]; !ok {
		t.Fatalf("the upload stored nothing under %s", objectKey)
	}
	return id, objectKey
}

// --- retention configuration --------------------------------------------------

// DOWNLOAD_ARTIFACT_RETENTION is deployment configuration and not schema: 0027
// records the pointer to PDM-006 and refuses to freeze an unratified 90 days
// into a DEFAULT. This checks the value actually reaches expires_at.
func TestTheConfiguredRetentionIsWhatDecidesExpiry(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "retention")
	a.packaging.Retention = policy.DownloadRetention(time.Hour)

	art := buildDownload(t, a, pool, c, "short-lived-skill")
	expires, err := time.Parse(time.RFC3339, art.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Until(expires); d > 2*time.Hour || d < 30*time.Minute {
		t.Errorf("expires in %s; the configured hour did not reach the row", d)
	}
}
