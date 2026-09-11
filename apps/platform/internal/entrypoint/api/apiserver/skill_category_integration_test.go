package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func putCategory(t *testing.T, c *client, skillID, category string) (int, map[string]any) {
	t.Helper()
	body := `{"category":"` + category + `"}`
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut,
		c.base+"/skills/"+skillID+"/category", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func skillCategoryColumns(t *testing.T, pool *pgxpool.Pool, skillID string) (category, source *string) {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(skillID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(),
		"SELECT category, category_source FROM skills WHERE id = $1", id,
	).Scan(&category, &source); err != nil {
		t.Fatal(err)
	}
	return category, source
}

func TestSetCategoryOwnerCanAssignAnyOfTheThreeShelves(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "category-owner")
	skillID := importPackage(t, pool, a.packages, owner, "category-shelf-pick", false)

	for _, shelf := range []string{"documents", "writing", "data"} {
		code, resp := putCategory(t, owner, skillID, shelf)
		if code != http.StatusOK {
			t.Fatalf("category=%s: got %d, body=%v", shelf, code, resp)
		}
		if id, _ := resp["skill_id"].(string); id != skillID {
			t.Errorf("category=%s: response skill_id = %q, want %q", shelf, id, skillID)
		}
		category, source := skillCategoryColumns(t, pool, skillID)
		if category == nil || *category != shelf {
			t.Errorf("category=%s: stored category = %v, want %q", shelf, category, shelf)
		}
		if source == nil || *source != "owner" {
			t.Errorf("category=%s: stored category_source = %v, want \"owner\"", shelf, source)
		}
	}
}

func TestSetCategoryUnassignedClearsBothColumns(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "category-clear")
	skillID := importPackage(t, pool, a.packages, owner, "category-clear-pick", false)

	if code, _ := putCategory(t, owner, skillID, "writing"); code != http.StatusOK {
		t.Fatalf("initial set: got %d", code)
	}
	if category, _ := skillCategoryColumns(t, pool, skillID); category == nil {
		t.Fatal("initial set did not persist; the rest of this test proves nothing")
	}

	code, _ := putCategory(t, owner, skillID, "unassigned")
	if code != http.StatusOK {
		t.Fatalf("unassigned: got %d", code)
	}
	category, source := skillCategoryColumns(t, pool, skillID)
	if category != nil {
		t.Errorf("unassigned left category = %q, want NULL", *category)
	}
	if source != nil {
		t.Errorf("unassigned left category_source = %q, want NULL", *source)
	}
}

func TestSetCategoryRejectsAnUnknownShelf(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "category-bogus")
	skillID := importPackage(t, pool, a.packages, owner, "category-bogus-pick", false)

	code, resp := putCategory(t, owner, skillID, "bogus")
	if code != http.StatusBadRequest {
		t.Fatalf("category=bogus: got %d, body=%v", code, resp)
	}
	want := `category must be "documents", "writing", "data" or "unassigned"`
	if got, _ := resp["error"].(string); got != want {
		t.Errorf("error = %q, want %q", got, want)
	}

	if category, source := skillCategoryColumns(t, pool, skillID); category != nil || source != nil {
		t.Errorf("a rejected write still touched the row: category=%v source=%v", category, source)
	}
}

func TestSetCategoryOnAnotherWorkspacesSkillIs404(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "category-owner-2")
	intruder := a.login(t, "category-intruder")
	skillID := importPackage(t, pool, a.packages, owner, "category-not-yours", false)

	code, _ := putCategory(t, intruder, skillID, "documents")
	if code != http.StatusNotFound {
		t.Fatalf("intruder PUT: got %d, want 404", code)
	}

	if category, source := skillCategoryColumns(t, pool, skillID); category != nil || source != nil {
		t.Errorf("a rejected write still touched the row: category=%v source=%v", category, source)
	}
}

func TestSetCategoryRequiresSession(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "category-session-owner")
	skillID := importPackage(t, pool, a.packages, owner, "category-needs-session", false)

	anon := &client{Client: http.DefaultClient, base: a.URL}
	code, _ := putCategory(t, anon, skillID, "documents")
	if code != http.StatusUnauthorized {
		t.Fatalf("no session PUT: got %d, want 401", code)
	}
}
