package apiserver_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/apiserver"
)

func TestAccountDeletionStatesThePurgeDateTheConfiguredGraceWillActOn(t *testing.T) {
	pool := requireDB(t)
	a := newAPITuned(t, pool, "", func(d *apiserver.Deps) { d.Auth.PurgeGrace = 1440 * time.Hour })
	c := a.login(t, "sixty-day-deleter")

	graceOf := func(body map[string]any, requestedField string) time.Duration {
		t.Helper()
		requested, err := time.Parse(time.RFC3339, body[requestedField].(string))
		if err != nil {
			t.Fatalf("%s = %v: %v", requestedField, body[requestedField], err)
		}
		purge, err := time.Parse(time.RFC3339, body["purge_after"].(string))
		if err != nil {
			t.Fatalf("purge_after = %v: %v", body["purge_after"], err)
		}
		return purge.Sub(requested.Truncate(time.Second))
	}

	status, deleted := deleteJSON(t, c, "/me")
	if status != http.StatusOK {
		t.Fatalf("DELETE /me: got %d", status)
	}
	if got := graceOf(deleted, "deletion_requested_at"); got != 1440*time.Hour {
		t.Errorf("DELETE /me purge_after is %s after the request, want 1440h", got)
	}
	if got := graceOf(c.me(t), "deletion_requested_at"); got != 1440*time.Hour {
		t.Errorf("GET /me purge_after is %s after the request, want 1440h", got)
	}
}
