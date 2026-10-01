package apiserver_test

import (
	"net/http"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
)

func (c *client) getWithTag(t *testing.T, path, ifNoneMatch string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, c.base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp
}

func TestTheCatalogueIsSharedBrieflyAndRevalidatesWithoutABody(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "catalogue-revalidation")

	first := c.getWithTag(t, "/api/skills/catalog", "")
	if first.StatusCode != http.StatusOK || first.Header.Get("Cache-Control") != httpx.SharedBriefly || first.Header.Get("ETag") == "" {
		t.Fatalf("catalogue answered %d Cache-Control %q ETag %q", first.StatusCode, first.Header.Get("Cache-Control"), first.Header.Get("ETag"))
	}
	if again := c.getWithTag(t, "/api/skills/catalog", first.Header.Get("ETag")); again.StatusCode != http.StatusNotModified {
		t.Fatalf("revalidating with the current tag answered %d, want 304", again.StatusCode)
	}
}

func TestAnOwnSkillIsRevalidatedOnEveryView(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "own-skill-revalidation")
	skillID, _ := importFiles(t, a, pool, c, map[string]string{
		"SKILL.md": "---\nname: own-skill\ndescription: Mine to change.\n---\n\nDo the thing.\n",
	})

	for _, path := range []string{"/api/skills/" + skillID, "/api/skills/" + skillID + "/files"} {
		resp := c.getWithTag(t, path, "")
		if resp.StatusCode != http.StatusOK || resp.Header.Get("Cache-Control") != httpx.PrivateFresh {
			t.Errorf("%s answered %d Cache-Control %q, want 200 and %q", path, resp.StatusCode, resp.Header.Get("Cache-Control"), httpx.PrivateFresh)
		}
	}
}
