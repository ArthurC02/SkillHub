package apiserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type workspaceReadCounter struct{ reads atomic.Int64 }

func (c *workspaceReadCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "name: ListWorkspacesByOwner") {
		c.reads.Add(1)
	}
	return ctx
}

func (*workspaceReadCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestASignedInRequestFindsItsWorkspaceWithTheSessionLookup(t *testing.T) {
	config := requireDB(t).Config()
	counter := &workspaceReadCounter{}
	config.ConnConfig.Tracer = counter
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	a := newAPI(t, pool)
	c := a.login(t, "one-identity-lookup")

	counter.reads.Store(0)
	var me struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if code := getJSON(t, c.Client, c.base+"/me", &me); code != http.StatusOK {
		t.Fatalf("GET /me: got %d", code)
	}
	if me.WorkspaceID != c.workspaceID {
		t.Fatalf("GET /me answered workspace %q, want the signed-in user's %q", me.WorkspaceID, c.workspaceID)
	}
	if n := counter.reads.Load(); n != 0 {
		t.Errorf("the request read the workspace table %d more times after the session lookup, want 0", n)
	}
}

func TestTheWorkspaceFoundWithTheSessionIsTheOneTheTableHolds(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "session-workspace-fields")
	if _, err := pool.Exec(t.Context(), "UPDATE workspaces SET is_catalog = true, updated_at = created_at + interval '1 hour' WHERE id = $1", mustUUID(t, c.workspaceID)); err != nil {
		t.Fatal(err)
	}
	leaveCatalogAtEnd(t, pool, mustUUID(t, c.workspaceID))

	other := a.login(t, "session-workspace-someone-else")

	var fromSession, fromTable, othersWorkspace identity.Workspace
	var lookupErr error
	signedIn := a.auth.RequireSession(func(_ http.ResponseWriter, r *http.Request) {
		user, _ := identity.SessionUser(r.Context())
		fromSession, lookupErr = a.auth.Service.PersonalWorkspace(r.Context(), user)
		fromTable, _ = a.auth.Service.PersonalWorkspace(context.Background(), user)
		othersWorkspace, _ = a.auth.Service.PersonalWorkspace(r.Context(), identity.User{ID: mustUUID(t, other.userID)})
	})
	req := httptest.NewRequest(http.MethodGet, c.base+"/me", nil)
	for _, cookie := range c.Jar.Cookies(req.URL) {
		req.AddCookie(cookie)
	}
	signedIn(httptest.NewRecorder(), req)

	if lookupErr != nil || !fromTable.ID.Valid {
		t.Fatalf("no workspace was resolved: %v", lookupErr)
	}
	if fromSession != fromTable {
		t.Errorf("workspace from the session %+v differs from the table's %+v", fromSession, fromTable)
	}
	if got := pgconv.UUIDString(othersWorkspace.ID); got != other.workspaceID {
		t.Errorf("asking for another account's workspace inside a signed-in request answered %s, want %s", got, other.workspaceID)
	}
}
