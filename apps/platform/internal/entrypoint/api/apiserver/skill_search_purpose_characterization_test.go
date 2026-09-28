package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func searchModel(t *testing.T, embedAxis int, reasonCalls *atomic.Int32) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /embed", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"embeddings": [][]float32{unitVector(embedAxis)}, "model": "embed-model", "dimensions": embedDims})
	})
	mux.HandleFunc("POST /match-reasons", func(w http.ResponseWriter, _ *http.Request) {
		reasonCalls.Add(1)
		writeJSON(w, map[string]any{"reasons": []map[string]string{}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func seedCatalogSkill(t *testing.T, pool *pgxpool.Pool, a *api, name string, axis int) string {
	t.Helper()
	ctx := context.Background()
	var userID, wsID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO users (email, display_name) VALUES ($1, $1) RETURNING id`, name+"@example.test").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO workspaces (owner_user_id, name) VALUES ($1, $2) RETURNING id`, userID, name).Scan(&wsID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { purgeCatalogWorkspace(t, pool, wsID, userID) })
	owner := &client{workspaceID: uuidText(wsID), userID: uuidText(userID)}
	markCatalog(t, pool, owner.workspaceID)
	id := importPackage(t, pool, a.packages, owner, name, true)
	seedEmbedding(t, pool, id, axis)
	return id
}

func purgeCatalogWorkspace(t *testing.T, pool *pgxpool.Pool, wsID, userID pgtype.UUID) {
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Errorf("cleanup: %v", err)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, step := range []struct {
		stmt string
		args []any
	}{
		{`SET LOCAL skillhub.purge = 'on'`, nil},
		{`DELETE FROM audit_events WHERE workspace_id = $1 OR actor_user_id = $2`, []any{wsID, userID}},
		{`DELETE FROM search_documents WHERE workspace_id = $1`, []any{wsID}},
		{`DELETE FROM skill_versions WHERE workspace_id = $1`, []any{wsID}},
		{`DELETE FROM skills WHERE workspace_id = $1`, []any{wsID}},
		{`DELETE FROM skill_sources WHERE workspace_id = $1`, []any{wsID}},
		{`DELETE FROM outbox_events WHERE workspace_id = $1`, []any{wsID}},
		{`DELETE FROM workspaces WHERE id = $1`, []any{wsID}},
		{`DELETE FROM users WHERE id = $1`, []any{userID}},
	} {
		if _, err := tx.Exec(ctx, step.stmt, step.args...); err != nil {
			t.Errorf("cleanup %q: %v", step.stmt, err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Errorf("cleanup: %v", err)
	}
}

func TestAReferenceSearchAsksTheModelForNoMatchReasons(t *testing.T) {
	pool := requireDB(t)
	var reasonCalls atomic.Int32
	a := newAPIWithLLM(t, pool, searchModel(t, 1234, &reasonCalls))
	name := uniqueWorklistLabel("borogove-reference")
	id := seedCatalogSkill(t, pool, a, name, 1234)
	anon := &client{Client: http.DefaultClient, base: a.URL}

	reference := anon.search(t, "/api/skills/search?purpose=reference&q="+url.QueryEscape(name))
	if !slices.Contains(reference.ids(), id) || reasonCalls.Load() != 0 {
		t.Fatalf("reference search found %v with %d match-reason calls, want %s and none", reference.ids(), reasonCalls.Load(), id)
	}

	person := anon.search(t, "/api/skills/search?q="+url.QueryEscape(name))
	if !slices.Contains(person.ids(), id) || reasonCalls.Load() == 0 {
		t.Fatalf("person search found %v with %d match-reason calls, want %s explained", person.ids(), reasonCalls.Load(), id)
	}
}

func TestASkillDetailSaysHowFarItsEnrichmentGot(t *testing.T) {
	pool := requireDB(t)
	var reasonCalls atomic.Int32
	a := newAPIWithLLM(t, pool, searchModel(t, 1236, &reasonCalls))
	id := seedCatalogSkill(t, pool, a, uniqueWorklistLabel("jubjub-detail"), 1236)

	resp, err := http.Get(a.URL + "/api/skills/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var detail struct {
		Enrichment struct {
			Status string `json:"status"`
			Note   string `json:"note"`
		} `json:"enrichment"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("detail status=%d err=%v", resp.StatusCode, err)
	}
	if detail.Enrichment.Status != "enriched" || detail.Enrichment.Note == "" {
		t.Fatalf("enrichment = %+v, want the enriched status and its note", detail.Enrichment)
	}
}

func TestAMeaningOnlyMatchRemovedByAFilterIsReportedAsFilteredOut(t *testing.T) {
	pool := requireDB(t)
	var reasonCalls atomic.Int32
	a := newAPIWithLLM(t, pool, searchModel(t, 1235, &reasonCalls))
	id := seedCatalogSkill(t, pool, a, uniqueWorklistLabel("mome-semantic"), 1235)
	anon := &client{Client: http.DefaultClient, base: a.URL}

	unfiltered := anon.search(t, "/api/skills/search?q=vorpalquux")
	if unfiltered.Degraded || !slices.Contains(unfiltered.ids(), id) {
		t.Fatalf("unfiltered search degraded=%v found %v, want the meaning-only match %s", unfiltered.Degraded, unfiltered.ids(), id)
	}

	filtered := anon.search(t, "/api/skills/search?q=vorpalquux&script=no")
	if len(filtered.Results) != 0 || !filtered.FilteredOut || filtered.NoResults {
		t.Fatalf("filtered search = %+v, want an empty page that blames the filters", filtered)
	}
}
