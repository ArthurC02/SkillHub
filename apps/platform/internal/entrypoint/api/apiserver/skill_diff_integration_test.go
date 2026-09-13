package apiserver_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type skillDiffFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Diff   string `json:"diff"`
}

type skillDiffBody struct {
	Files []skillDiffFile `json:"files"`
	Error string          `json:"error"`
}

func (c *client) skillDiff(t *testing.T, skillID, from, to string) (int, skillDiffBody) {
	t.Helper()
	resp, err := c.Get(c.base + "/skills/" + skillID + "/diff?from=" + from + "&to=" + to)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out skillDiffBody
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestSkillDiffReportsAddedRemovedAndModifiedFilesInDirection(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "diff-owner")
	skillID := seedSkill(t, pool, owner.workspaceID, "diff-skill")

	fromVersion := seedVersion(t, pool, owner.workspaceID, skillID, "diff-hash-from")
	toVersion := seedVersion(t, pool, owner.workspaceID, skillID, "diff-hash-to")

	a.packages[fromVersion.PackageObjectKey] = zipOf(t, map[string]string{
		"SKILL.md":         "---\nname: diff-skill\n---\nold line\n",
		"only-in-from.txt": "gone soon\n",
	})
	a.packages[toVersion.PackageObjectKey] = zipOf(t, map[string]string{
		"SKILL.md":       "---\nname: diff-skill\n---\nnew line\n",
		"only-in-to.txt": "just added\n",
	})

	status, body := owner.skillDiff(t, skillID, uuidText(fromVersion.ID), uuidText(toVersion.ID))
	if status != http.StatusOK {
		t.Fatalf("GET diff: got %d (%s)", status, body.Error)
	}

	byPath := map[string]skillDiffFile{}
	for _, f := range body.Files {
		byPath[f.Path] = f
	}
	if len(byPath) != 3 {
		t.Fatalf("want 3 files, got %d: %+v", len(byPath), body.Files)
	}
	if got := byPath["only-in-from.txt"].Status; got != "removed" {
		t.Errorf("only-in-from.txt status = %q, want removed", got)
	}
	if got := byPath["only-in-to.txt"].Status; got != "added" {
		t.Errorf("only-in-to.txt status = %q, want added", got)
	}
	md := byPath["SKILL.md"]
	if md.Status != "modified" {
		t.Fatalf("SKILL.md status = %q, want modified", md.Status)
	}
	if !strings.Contains(md.Diff, "-old line") || !strings.Contains(md.Diff, "+new line") {
		t.Errorf("diff direction wrong, want from's line removed and to's line added: %q", md.Diff)
	}
}
