package apiserver_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	packaging "github.com/ArthurC02/skillhub/apps/platform/internal/skill/delivery"
)

type curatedForkWorld struct {
	a                                            *api
	pool                                         *pgxpool.Pool
	curator, owner                               *client
	root, leaf, version, publicCase, privateCase string
	skills                                       []string
}

func newCuratedForkWorld(t *testing.T, depth int) curatedForkWorld {
	t.Helper()
	pool := requireDB(t)
	a := newAPI(t, pool)
	curator := a.login(t, "ancestor-curator")
	makeCatalog(t, pool, curator.workspaceID)
	root, _ := packagedSkill(t, a, pool, curator, "ancestor-example")
	publicCase := seedTestCase(t, pool, curator.workspaceID, root)
	if code, body := curator.upload(t, "/test-cases/"+publicCase+"/datasets", "public.csv", []byte("city,value\nTaipei,7\n")); code != http.StatusCreated {
		t.Fatalf("upload curated Dataset: %d %v", code, body)
	}
	owner := a.login(t, "ancestor-owner")
	w := curatedForkWorld{a: a, pool: pool, curator: curator, owner: owner, root: root, leaf: root, publicCase: publicCase, skills: []string{root}}
	for range depth {
		code, body := postJSON(t, owner, "/skills/"+w.leaf+"/fork", `{}`)
		if code != http.StatusCreated {
			t.Fatalf("fork ancestor: %d %v", code, body)
		}
		w.leaf, _ = body["skill_id"].(string)
		w.skills = append(w.skills, w.leaf)
		w.privateCase = seedTestCase(t, pool, owner.workspaceID, w.leaf)
		if code, body := owner.upload(t, "/test-cases/"+w.privateCase+"/datasets", "private.txt", []byte("PRIVATE-ANCESTOR-DATA\n")); code != http.StatusCreated {
			t.Fatalf("upload private Dataset: %d %v", code, body)
		}
	}
	w.version = latestVersionID(t, pool, w.leaf)
	return w
}

func TestCuratedDatasetTravelsThroughForkAncestryWithoutCopies(t *testing.T) {
	for _, depth := range []int{1, 3} {
		t.Run(fmt.Sprint(depth), func(t *testing.T) {
			w := newCuratedForkWorld(t, depth)
			var preview map[string]any
			code := getJSON(t, w.owner.Client, w.owner.base+packagingPath(w.leaf, w.version)+"/preview?target=standard&include_test_cases=true", &preview)
			if code != http.StatusOK {
				t.Fatalf("preview ancestor Dataset: %d %v", code, preview)
			}
			assertCuratedAncestorSelection(t, preview, w.publicCase, w.privateCase)
			code, body := postJSON(t, w.owner, packagingPath(w.leaf, w.version), `{"target":"standard","include_test_cases":true}`)
			if code != http.StatusCreated {
				t.Fatalf("package ancestor Dataset: %d %v", code, body)
			}
			hash, _ := body["content_hash"].(string)
			assertCuratedDatasetFiles(t, zipEntries(t, w.a, hash))
			w.assertNoCopies(t, depth+1)
		})
	}
}

func assertCuratedDatasetFiles(t *testing.T, entries map[string][]byte) {
	t.Helper()
	assertExactlyOneCuratedPortableCase(t, entries)
	publicFiles := 0
	for name, data := range entries {
		if strings.HasSuffix(name, "/data/public.csv") {
			publicFiles++
			if string(data) != "city,value\nTaipei,7\n" {
				t.Fatalf("curated Dataset bytes changed: %q", data)
			}
		}
		if strings.HasSuffix(name, "private.txt") || bytes.Contains(data, []byte("PRIVATE-ANCESTOR-DATA")) {
			t.Fatalf("private ancestor data travelled in %s", name)
		}
	}
	if publicFiles != 1 {
		t.Fatalf("curated Dataset files = %d, want 1", publicFiles)
	}
}

func (w curatedForkWorld) assertNoCopies(t *testing.T, want int) {
	t.Helper()
	workspaces := []string{w.curator.workspaceID, w.owner.workspaceID}
	cases := countRow(t, w.pool, `SELECT count(*) FROM test_cases WHERE skill_id::text = ANY($1::text[]) AND workspace_id::text = ANY($2::text[])`, w.skills, workspaces)
	datasets := countRow(t, w.pool, `SELECT count(*) FROM datasets d JOIN test_cases tc ON tc.id=d.test_case_id AND tc.workspace_id=d.workspace_id WHERE tc.skill_id::text = ANY($1::text[]) AND tc.workspace_id::text = ANY($2::text[])`, w.skills, workspaces)
	if cases != want || datasets != want {
		t.Fatalf("Fork or packaging copied examples: cases=%d datasets=%d, want %d", cases, datasets, want)
	}
}

func TestCuratedAncestorPrefersTheNearestCatalogSource(t *testing.T) {
	w := newCuratedForkWorld(t, 2)
	makeCatalog(t, w.pool, w.owner.workspaceID)
	source, found, err := w.a.packaging.CuratedSource(t.Context(), mustUUID(t, w.owner.workspaceID), mustUUID(t, w.leaf))
	if err != nil || !found || source.SkillID != mustUUID(t, w.skills[1]) || source.WorkspaceID != mustUUID(t, w.owner.workspaceID) {
		t.Fatalf("nearest catalog source was replaced: %+v %v %v", source, found, err)
	}
}

func TestCatalogReadsKeepTheRestrictionInheritedFromACuratedAncestor(t *testing.T) {
	w := newCuratedForkWorld(t, 1)
	makeCatalog(t, w.pool, w.owner.workspaceID)
	if _, err := w.pool.Exec(t.Context(), `UPDATE skills SET access_restriction='license-review' WHERE id=$1 AND workspace_id=$2`, mustUUID(t, w.root), mustUUID(t, w.curator.workspaceID)); err != nil {
		t.Fatal(err)
	}
	skill, found, err := w.a.versions.References.CatalogSkill(t.Context(), mustUUID(t, w.leaf))
	if err != nil || !found || skill.ID != mustUUID(t, w.leaf) || skill.InheritedRestriction != "license-review" {
		t.Fatalf("catalog read lost the inherited restriction: %+v %v %v", skill, found, err)
	}
}

func TestCuratedAncestorRequiresALiveOwnedRoot(t *testing.T) {
	w := newCuratedForkWorld(t, 1)
	if _, err := w.pool.Exec(t.Context(), `UPDATE skills SET deleted_at=now() WHERE id=$1 AND workspace_id=$2`, mustUUID(t, w.leaf), mustUUID(t, w.owner.workspaceID)); err != nil {
		t.Fatal(err)
	}
	if source, found, err := w.a.packaging.CuratedSource(t.Context(), mustUUID(t, w.owner.workspaceID), mustUUID(t, w.leaf)); err != nil || found || source != (packaging.CuratedSource{}) {
		t.Fatalf("deleted root still read a curated ancestor: %+v %v %v", source, found, err)
	}
}

func TestAnOptedOutForkCarriesNoCuratedDataset(t *testing.T) {
	w := newCuratedForkWorld(t, 2)
	code, body := postJSON(t, w.owner, packagingPath(w.leaf, w.version), `{"target":"standard","include_test_cases":false}`)
	if code != http.StatusCreated {
		t.Fatalf("opted-out package: %d %v", code, body)
	}
	hash, _ := body["content_hash"].(string)
	for name := range zipEntries(t, w.a, hash) {
		if strings.HasPrefix(name, "test-cases/") {
			t.Fatalf("opted-out example travelled in %s", name)
		}
	}
	w.assertNoCopies(t, 3)
}

func TestACatalogCycleDoesNotReturnTheRequestedSkillAsItsOwnAncestor(t *testing.T) {
	w := newCuratedForkWorld(t, 1)
	if _, err := w.pool.Exec(t.Context(), `UPDATE skills SET forked_from_skill_id=id WHERE id=$1 AND workspace_id=$2`, mustUUID(t, w.root), mustUUID(t, w.curator.workspaceID)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if source, found, err := w.a.packaging.CuratedSource(ctx, mustUUID(t, w.curator.workspaceID), mustUUID(t, w.root)); err != nil || found || source != (packaging.CuratedSource{}) {
		t.Fatalf("catalog cycle returned itself or failed to terminate: %+v %v %v", source, found, err)
	}
}

func assertCuratedAncestorSelection(t *testing.T, body map[string]any, publicCase, privateCase string) {
	t.Helper()
	included, _ := body["included_test_cases"].([]any)
	excluded, _ := body["excluded_test_cases"].([]any)
	if len(included) != 1 || len(excluded) != 1 {
		t.Fatalf("ancestor selection = %v, want one public case and one private exclusion", body)
	}
	if included[0].(map[string]any)["test_case_id"] != publicCase {
		t.Fatalf("wrong curated ancestor case: %v", included)
	}
	row := excluded[0].(map[string]any)
	if row["test_case_id"] != privateCase || row["reason"] != "user_uploaded_dataset" {
		t.Fatalf("private leaf case was not excluded: %v", row)
	}
}

func TestCuratedAncestorReadsStayScopedAndSurviveOnlyIntermediateDeletion(t *testing.T) {
	w := newCuratedForkWorld(t, 2)
	stranger := w.a.login(t, "ancestor-stranger")
	if source, found, err := w.a.packaging.CuratedSource(t.Context(), mustUUID(t, stranger.workspaceID), mustUUID(t, w.leaf)); err != nil || found || source != (packaging.CuratedSource{}) {
		t.Fatalf("foreign Workspace obtained an ancestor: %+v %v %v", source, found, err)
	}
	if _, err := w.pool.Exec(t.Context(), `UPDATE skills SET deleted_at=now() WHERE id=$1 AND workspace_id=$2`, mustUUID(t, w.skills[1]), mustUUID(t, w.owner.workspaceID)); err != nil {
		t.Fatal(err)
	}
	source, found, err := w.a.packaging.CuratedSource(t.Context(), mustUUID(t, w.owner.workspaceID), mustUUID(t, w.leaf))
	if err != nil || !found || source.SkillID != mustUUID(t, w.root) || source.WorkspaceID != mustUUID(t, w.curator.workspaceID) {
		t.Fatalf("intermediate deletion lost the public ancestor: %+v %v %v", source, found, err)
	}
	if _, err := w.pool.Exec(t.Context(), `UPDATE skills SET deleted_at=now() WHERE id=$1 AND workspace_id=$2`, mustUUID(t, w.root), mustUUID(t, w.curator.workspaceID)); err != nil {
		t.Fatal(err)
	}
	if source, found, err := w.a.packaging.CuratedSource(t.Context(), mustUUID(t, w.owner.workspaceID), mustUUID(t, w.leaf)); err != nil || found || source != (packaging.CuratedSource{}) {
		t.Fatalf("deleted curated source was exposed: %+v %v %v", source, found, err)
	}
}

func TestCuratedAncestorCyclesDoNotExposePrivateSources(t *testing.T) {
	w := newCuratedForkWorld(t, 2)
	if _, err := w.pool.Exec(t.Context(), `UPDATE skills SET forked_from_skill_id=$1 WHERE id=$2 AND workspace_id=$3`, mustUUID(t, w.leaf), mustUUID(t, w.skills[1]), mustUUID(t, w.owner.workspaceID)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if source, found, err := w.a.packaging.CuratedSource(ctx, mustUUID(t, w.owner.workspaceID), mustUUID(t, w.leaf)); err != nil || found || source != (packaging.CuratedSource{}) {
		t.Fatalf("cyclic private lineage returned a source or failed to terminate: %+v %v %v", source, found, err)
	}
}

func TestAnUnreadableCuratedAncestorDoesNotBecomeAnEmptyExampleList(t *testing.T) {
	w := newCuratedForkWorld(t, 2)
	outage := errors.New("curated source unavailable")
	w.a.packaging.CuratedSource = func(context.Context, pgtype.UUID, pgtype.UUID) (packaging.CuratedSource, bool, error) {
		return packaging.CuratedSource{}, false, outage
	}
	before := len(w.a.packages)
	if code, body := postJSON(t, w.owner, packagingPath(w.leaf, w.version), `{"target":"standard","include_test_cases":true}`); code != http.StatusInternalServerError {
		t.Fatalf("unreadable examples were claimed absent: %d %v", code, body)
	}
	if len(w.a.packages) != before {
		t.Fatal("failed ancestor lookup stored a package")
	}
}
