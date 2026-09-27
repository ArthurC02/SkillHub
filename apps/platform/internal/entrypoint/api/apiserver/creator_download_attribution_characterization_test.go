package apiserver_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
)

func TestADownloadStartedBySignedInCreatorIsAttributedToTheirWorkspace(t *testing.T) {
	pool := requireDB(t)
	a := betaAPI(t, pool, policy.QuotaLimits{}, nil, 180*24*time.Hour)
	f := newFixture(t, a, pool, "creator-download-attribution")
	session := f.analyticsSession(t)
	if session == "" {
		t.Fatal("no analytics session cookie was issued")
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM analytics_events WHERE session_id = $1`, session)
	})

	f.status(t, http.MethodGet, "/downloads/00000000-0000-4000-8000-0000000000ff/content")

	var workspace *string
	if err := pool.QueryRow(context.Background(), `
		SELECT workspace_id::text FROM analytics_events
		WHERE event_name = 'download_started' AND session_id = $1`, session,
	).Scan(&workspace); err != nil {
		t.Fatalf("no download_started event: %v", err)
	}
	if workspace == nil || *workspace != f.workspaceID {
		t.Errorf("download_started workspace = %v, want %s", workspace, f.workspaceID)
	}
}
