package apiserver_test

import (
	"context"
	"net/http"
	"testing"
)

func TestDeletingADownloadKeepsAnObjectAnotherLiveArtifactShares(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, uniqueWorklistLabel("download-shared-object"))
	art := buildDownload(t, a, pool, c, uniqueWorklistLabel("download-shared-object-skill"))
	ctx := context.Background()
	var objectKey string
	if err := pool.QueryRow(ctx, `
		INSERT INTO artifacts (workspace_id, run_id, kind, file_name, content_type, size_bytes,
		                       content_hash, object_key, scan_status, expires_at)
		SELECT workspace_id, run_id, kind, file_name, content_type, size_bytes,
		       content_hash, object_key, scan_status, expires_at
		FROM artifacts WHERE id = $1
		RETURNING object_key`, mustUUID(t, art.ArtifactID)).Scan(&objectKey); err != nil {
		t.Fatal(err)
	}

	if code, _ := deleteJSON(t, c, "/downloads/"+art.ArtifactID); code != http.StatusNoContent {
		t.Fatalf("DELETE download: got %d", code)
	}

	if _, kept := a.packages[objectKey]; !kept {
		t.Fatalf("object %q was removed although another live artifact still shares it", objectKey)
	}
}
