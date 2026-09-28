package apiserver_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
)

const (
	referenceAxisNearest   = 1471
	referenceAxisElsewhere = 1472
)

type referenceShelf struct {
	ws       identity.Workspace
	skills   []string
	versions []string
	names    []string
}

func creationWorkspaceOf(t *testing.T, pool *pgxpool.Pool, c *client) identity.Workspace {
	t.Helper()
	row, err := gen.New(pool).GetWorkspace(context.Background(), gen.GetWorkspaceParams{
		ID: mustUUID(t, c.workspaceID), OwnerUserID: mustUUID(t, c.userID),
	})
	if err != nil {
		t.Fatal(err)
	}
	return publishedWorkspace(row)
}

func uniqueReferenceWord(prefix string) string {
	digits := strconv.FormatInt(time.Now().UnixNano(), 10)
	return prefix + strings.Map(func(r rune) rune { return 'a' + (r - '0') }, digits)
}

func shelveReferences(t *testing.T, a *api, pool *pgxpool.Pool, word string, axis, readable, unreadable int) referenceShelf {
	t.Helper()
	owner := a.login(t, word)
	markCatalog(t, pool, owner.workspaceID)
	shelf := referenceShelf{ws: creationWorkspaceOf(t, pool, owner)}
	for i := range readable + unreadable {
		name := fmt.Sprintf("%s %c", word, 'a'+rune(i))
		skillID := seedSkill(t, pool, owner.workspaceID, name)
		version := seedVersion(t, pool, owner.workspaceID, skillID, "hash-"+name)
		if i < readable {
			a.packages[version.PackageObjectKey] = cleanPackage(t)
		}
		seedEmbedding(t, pool, skillID, axis)
		shelf.skills = append(shelf.skills, skillID)
		shelf.versions = append(shelf.versions, uuidText(version.ID))
		shelf.names = append(shelf.names, name)
	}
	return shelf
}

func setScan(t *testing.T, pool *pgxpool.Pool, skillID, scan string) {
	t.Helper()
	tag, err := pool.Exec(context.Background(), `UPDATE search_documents SET scan = $2::jsonb WHERE skill_id = $1`, mustUUID(t, skillID), scan)
	if err != nil {
		t.Fatal(err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("skill %s has no search document to carry a scan", skillID)
	}
}

func referenceIDs(refs []creation.Reference) []string {
	ids := make([]string, len(refs))
	for i, r := range refs {
		ids[i] = r.SkillID
	}
	return ids
}

func TestCreationReferenceOfAScannedCatalogSkillCarriesItsFactsAndContent(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	shelf := shelveReferences(t, a, pool, uniqueReferenceWord("ref-scanned"), referenceAxisElsewhere, 1, 0)
	setScan(t, pool, shelf.skills[0], `{"warnings": 2, "codes": ["script-file"]}`)

	ref, content, err := a.app.CreationSvc.ResolveReference(context.Background(), shelf.ws, shelf.skills[0], "")
	if err != nil {
		t.Fatal(err)
	}
	if !ref.Available || ref.SkillID != shelf.skills[0] || ref.VersionID != shelf.versions[0] || ref.Name != shelf.names[0] {
		t.Errorf("reference = %+v, want available %s@%s named %s", ref, shelf.skills[0], shelf.versions[0], shelf.names[0])
	}
	if ref.Description != "A skill with no script." {
		t.Errorf("description = %q, want the manifest's", ref.Description)
	}
	if ref.Tier != "indexed" || ref.ScanStatus != "scanned" || ref.Warnings == nil || *ref.Warnings != 2 {
		t.Errorf("catalog facts: tier=%q scan=%q warnings=%v, want indexed/scanned/2", ref.Tier, ref.ScanStatus, ref.Warnings)
	}
	if content.Name != shelf.names[0] || !strings.Contains(content.SkillMD, "Just prose.") {
		t.Errorf("content = %+v, want the package's SKILL.md under the skill's name", content)
	}
}

func TestCreationReferenceWithoutAReadableScanCarriesNoWarningCount(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	shelf := shelveReferences(t, a, pool, uniqueReferenceWord("ref-unscanned"), referenceAxisElsewhere, 1, 0)
	setScan(t, pool, shelf.skills[0], `[]`)

	ref, _, err := a.app.CreationSvc.ResolveReference(context.Background(), shelf.ws, shelf.skills[0], "")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Tier != "indexed" || ref.ScanStatus != "unavailable" || ref.Warnings != nil {
		t.Errorf("catalog facts: tier=%q scan=%q warnings=%v, want indexed/unavailable/none", ref.Tier, ref.ScanStatus, ref.Warnings)
	}
}

func TestCreationReferenceOfAPrivateSkillCarriesNoCatalogFacts(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	private := newFixture(t, a, pool, uniqueWorklistLabel("ref-private"))

	ref, _, err := a.app.CreationSvc.ResolveReference(context.Background(), creationWorkspaceOf(t, pool, private.client), private.skillID, "")
	if err != nil {
		t.Fatal(err)
	}
	if !ref.Available || ref.Tier != "" || ref.ScanStatus != "" || ref.Warnings != nil {
		t.Errorf("reference = %+v, want available with no catalog facts", ref)
	}
}

func TestCreationReferenceAtAStatedVersionKeepsThatVersion(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	shelf := shelveReferences(t, a, pool, uniqueReferenceWord("ref-pinned"), referenceAxisElsewhere, 1, 0)
	newer := seedVersion(t, pool, uuidText(shelf.ws.ID), shelf.skills[0], "hash-newer-"+shelf.names[0])
	a.packages[newer.PackageObjectKey] = cleanPackage(t)

	pinned, _, err := a.app.CreationSvc.ResolveReference(context.Background(), shelf.ws, shelf.skills[0], shelf.versions[0])
	if err != nil || pinned.VersionID != shelf.versions[0] {
		t.Errorf("stated version: got %q err=%v, want %s", pinned.VersionID, err, shelf.versions[0])
	}
	latest, _, err := a.app.CreationSvc.ResolveReference(context.Background(), shelf.ws, shelf.skills[0], "")
	if err != nil || latest.VersionID != uuidText(newer.ID) {
		t.Errorf("no version stated: got %q err=%v, want the latest %s", latest.VersionID, err, uuidText(newer.ID))
	}
}

func TestCreationReferenceThatCannotBeReadIsMarkedUnavailable(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	shelf := shelveReferences(t, a, pool, uniqueReferenceWord("ref-unreadable"), referenceAxisElsewhere, 0, 1)

	for name, skillID := range map[string]string{
		"a package missing from storage": shelf.skills[0],
		"a skill that does not exist":    uuidText(creationID(t)),
	} {
		ref, _, err := a.app.CreationSvc.ResolveReference(context.Background(), shelf.ws, skillID, "")
		if err == nil || ref.Available {
			t.Errorf("%s: available=%v err=%v, want unavailable with an error", name, ref.Available, err)
		}
	}
}

func TestCreationReferenceWithAMalformedIDIsAnInvalidCommand(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	shelf := shelveReferences(t, a, pool, uniqueReferenceWord("ref-malformed"), referenceAxisElsewhere, 1, 0)

	for name, ids := range map[string][2]string{
		"skill id":   {"not-a-uuid", ""},
		"version id": {shelf.skills[0], "not-a-uuid"},
	} {
		ref, _, err := a.app.CreationSvc.ResolveReference(context.Background(), shelf.ws, ids[0], ids[1])
		if !errors.Is(err, creation.ErrInvalidCommand) || ref != (creation.Reference{}) {
			t.Errorf("malformed %s: ref=%+v err=%v, want the zero reference and ErrInvalidCommand", name, ref, err)
		}
	}
}

func TestCreationReferenceSearchResolvesAtMostThree(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	word := uniqueReferenceWord("refsearchcap")
	shelf := shelveReferences(t, a, pool, word, referenceAxisElsewhere, 4, 0)

	refs, err := a.app.CreationSvc.SearchReferences(context.Background(), shelf.ws, word)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != creation.MaxReferences {
		t.Fatalf("got %d references %v, want %d", len(refs), referenceIDs(refs), creation.MaxReferences)
	}
	for _, r := range refs {
		if !r.Available || !contains(shelf.skills, r.SkillID) {
			t.Errorf("reference %+v is not one of the shelf's readable skills", r)
		}
	}
}

func TestCreationReferenceSearchLeavesOutWhatCannotBeRead(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	word := uniqueReferenceWord("refsearchskip")
	shelf := shelveReferences(t, a, pool, word, referenceAxisElsewhere, 2, 1)

	refs, err := a.app.CreationSvc.SearchReferences(context.Background(), shelf.ws, word)
	if err != nil {
		t.Fatal(err)
	}
	got := referenceIDs(refs)
	if len(got) != 2 || !contains(got, shelf.skills[0]) || !contains(got, shelf.skills[1]) {
		t.Errorf("got %v, want exactly the two readable skills %v", got, shelf.skills[:2])
	}
}

func TestCreationReferenceSearchThatCannotRunReportsWhy(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	shelf := shelveReferences(t, a, pool, uniqueReferenceWord("refsearcherr"), referenceAxisElsewhere, 1, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	refs, err := a.app.CreationSvc.SearchReferences(ctx, shelf.ws, shelf.names[0])
	if err == nil || refs != nil {
		t.Errorf("cancelled search: refs=%v err=%v, want no references and the error", refs, err)
	}
}

func TestCreationCatalogChecksWithoutAnEmbeddingModelOfferNothing(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	word := uniqueReferenceWord("refdegraded")
	shelf := shelveReferences(t, a, pool, word, referenceAxisElsewhere, 1, 0)
	ctx := context.Background()
	if err := gen.New(pool).SetSearchDocumentBigram(ctx, gen.SetSearchDocumentBigramParams{
		SkillID: mustUUID(t, shelf.skills[0]), BigramText: catalog.LexicalIndexText(shelf.names[0]),
	}); err != nil {
		t.Fatal(err)
	}
	lexical := &catalog.Service{Pool: pool, CatalogWorkspaces: (&identity.Service{Pool: pool}).CatalogWorkspaceIDs}
	if knowledge, err := lexical.CreationKnowledgeIDs(ctx, word, catalog.CreationMaxDistance); err != nil || !knowledge.Degraded || !contains(knowledge.IDs, shelf.skills[0]) {
		t.Fatalf("the lexical answer must find the skill for this test to mean anything: ids=%v degraded=%v err=%v", knowledge.IDs, knowledge.Degraded, err)
	}

	for name, check := range map[string]func(context.Context, identity.Workspace, string) ([]creation.Reference, float64, error){
		"catalog check":   a.app.CreationSvc.CatalogCheck,
		"duplicate check": a.app.CreationSvc.DuplicateCheck,
	} {
		refs, cost, err := check(ctx, shelf.ws, word)
		if err != nil || len(refs) != 0 || cost != 0 {
			t.Errorf("%s: refs=%v cost=%v err=%v, want nothing offered", name, referenceIDs(refs), cost, err)
		}
	}
}

func TestCreationCatalogChecksOfferTheNearestThreeReadableSkills(t *testing.T) {
	pool := requireDB(t)
	a := newAPIWithLLM(t, pool, stubLLM(t, referenceAxisNearest, "because it fits"))
	word := uniqueReferenceWord("refsemantic")
	shelf := shelveReferences(t, a, pool, word, referenceAxisNearest, 4, 1)

	for name, check := range map[string]func(context.Context, identity.Workspace, string) ([]creation.Reference, float64, error){
		"catalog check":   a.app.CreationSvc.CatalogCheck,
		"duplicate check": a.app.CreationSvc.DuplicateCheck,
	} {
		refs, _, err := check(context.Background(), shelf.ws, word)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(refs) != creation.MaxReferences {
			t.Fatalf("%s: got %d references %v, want %d", name, len(refs), referenceIDs(refs), creation.MaxReferences)
		}
		for _, r := range refs {
			if !r.Available || !contains(shelf.skills[:4], r.SkillID) {
				t.Errorf("%s: reference %+v is not one of the shelf's readable skills", name, r)
			}
		}
	}
}

func TestCreationCatalogChecksThatCannotRunReportWhy(t *testing.T) {
	pool := requireDB(t)
	a := newAPIWithLLM(t, pool, stubLLM(t, referenceAxisNearest, "because it fits"))
	shelf := shelveReferences(t, a, pool, uniqueReferenceWord("refsemanticerr"), referenceAxisElsewhere, 1, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for name, check := range map[string]func(context.Context, identity.Workspace, string) ([]creation.Reference, float64, error){
		"catalog check":   a.app.CreationSvc.CatalogCheck,
		"duplicate check": a.app.CreationSvc.DuplicateCheck,
	} {
		refs, _, err := check(ctx, shelf.ws, shelf.names[0])
		if err == nil || refs != nil {
			t.Errorf("%s cancelled: refs=%v err=%v, want no references and the error", name, referenceIDs(refs), err)
		}
	}
}
