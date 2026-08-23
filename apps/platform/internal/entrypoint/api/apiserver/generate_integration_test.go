package apiserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	gen "github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
)

// M5 generation, end to end against a stub model service (GEN-003, GEN-007,
// GEN-011).
//
// There is no HTTP route yet — GEN-008 mounts one behind the exposure flag
// (ADR-052) — so these drive ingest.Service directly, which is the same object
// the route will call. What they exercise is everything below that line: the
// gateway call, the packaging, admission's one validation path, the retry
// decision, the rows, and what search does with the result.

// generateStub is the internal LLM service, replying with a queued answer per
// call and counting how many times it was asked.
type generateStub struct {
	*httptest.Server
	answers []map[string]any
	calls   int
}

func newGenerateStub(t *testing.T, answers ...map[string]any) *generateStub {
	t.Helper()
	stub := &generateStub{answers: answers}
	stub.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/generate-skill" {
			t.Errorf("unexpected call to %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		i := stub.calls
		stub.calls++
		if i >= len(stub.answers) {
			t.Errorf("model called %d times; only %d answers queued", stub.calls, len(stub.answers))
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"skill":          stub.answers[i],
			"model":          "gpt-5.4-mini",
			"prompt_version": "generate-skill/v1",
		})
	}))
	t.Cleanup(stub.Close)
	return stub
}

func generatedSkill(name, body string) map[string]any {
	return map[string]any{
		"name":        name,
		"description": "把掃描的單據影像抽成表格。當使用者手上是掃描件、需要彙整成一份時使用。",
		"body":        body,
		"files":       []any{},
	}
}

// The whole path: a task description in, an immutable version out, with a
// provenance record that reproduces it (GEN-001, GEN-002, GEN-003).
func TestGeneratedSkillLandsAsAVersionWithItsOwnProvenance(t *testing.T) {
	pool := requireDB(t)
	stub := newGenerateStub(t, generatedSkill("invoice-to-table", "# 掃描單據轉表格\n\n1. 逐份抽出表格。\n"))
	a := newAPIWithLLM(t, pool, stub.URL)
	c := a.login(t, "gen-owner")

	task := "我每個月要把廠商寄來的掃描單據整理成一份表格交出去。"
	res, err := a.versions.GenerateSkill(context.Background(), workspaceOf(t, pool, c), task)
	if err != nil {
		t.Fatalf("GenerateSkill: %v", err)
	}
	if res.Report.Blocked {
		t.Fatalf("blocked: %+v", res.Report.Findings)
	}
	if res.Attempts != 1 || stub.calls != 1 {
		t.Errorf("attempts = %d, calls = %d, want 1 and 1", res.Attempts, stub.calls)
	}

	// The three columns 0037 added, read back from the row rather than from the
	// return value: ADR-047 決策 1 is a claim about what is stored.
	var sourceType, taskDescription, model, promptVersion string
	if err := pool.QueryRow(context.Background(), `
		SELECT s.source_type, s.task_description, s.generator_model, s.generator_prompt_version
		FROM skill_sources s JOIN skill_versions v ON v.source_id = s.id
		WHERE v.id = $1`, res.Version.ID,
	).Scan(&sourceType, &taskDescription, &model, &promptVersion); err != nil {
		t.Fatal(err)
	}
	if sourceType != "generated" {
		t.Errorf("source_type = %q; an upload-shaped row would take self_supplied silently", sourceType)
	}
	if taskDescription != task {
		t.Errorf("task_description = %q, want the user's own words", taskDescription)
	}
	if model != "gpt-5.4-mini" || promptVersion != "generate-skill/v1" {
		t.Errorf("generator = %q / %q", model, promptVersion)
	}

	var redistribution string
	if err := pool.QueryRow(context.Background(),
		"SELECT redistribution FROM skills WHERE id = $1", res.Skill.ID,
	).Scan(&redistribution); err != nil {
		t.Fatal(err)
	}
	if redistribution != "generated" {
		t.Errorf("redistribution = %q, want generated", redistribution)
	}
}

// ADR-047 決策 1: exactly one more attempt, same prompt, no correction hint.
// The first answer here carries an escaping entry name, which admission reports
// as a blocking finding rather than filtering away (02:GEN-003 — no warning
// removed because the platform wrote it).
func TestGenerationRetriesExactlyOnce(t *testing.T) {
	pool := requireDB(t)
	bad := generatedSkill("retry-me", "# 內容\n")
	bad["files"] = []any{map[string]any{"path": "../../evil.sh", "content": "echo hi\n"}}
	stub := newGenerateStub(t, bad, generatedSkill("retry-me", "# 內容\n\n1. 做這件事。\n"))

	a := newAPIWithLLM(t, pool, stub.URL)
	c := a.login(t, "gen-retry")

	res, err := a.versions.GenerateSkill(context.Background(), workspaceOf(t, pool, c), "把 PDF 轉成純文字。")
	if err != nil {
		t.Fatalf("GenerateSkill: %v", err)
	}
	if res.Report.Blocked {
		t.Fatalf("the second attempt was still blocked: %+v", res.Report.Findings)
	}
	if stub.calls != 2 || res.Attempts != 2 {
		t.Errorf("calls = %d, attempts = %d, want 2 and 2", stub.calls, res.Attempts)
	}
}

// ADR-048. The one blocking code that reads file content, and the one that must
// not buy a second paid call: a model writing a credential-shaped line did it
// because the task made it look useful, and the same prompt makes it look useful
// again. Only the call count can tell this apart from a normal failure.
func TestPossibleSecretIsNotRetriedEndToEnd(t *testing.T) {
	pool := requireDB(t)
	leaky := generatedSkill("leaky-setup", "# 設定\n\n照 setup.sh 執行。\n")
	leaky["files"] = []any{map[string]any{
		"path":    "setup.sh",
		"content": "export AWS_ACCESS_KEY_ID=AKIA0123456789ABCDEF\n",
	}}
	// Only one answer queued: a second call fails the test by itself.
	stub := newGenerateStub(t, leaky)

	a := newAPIWithLLM(t, pool, stub.URL)
	c := a.login(t, "gen-secret")

	res, err := a.versions.GenerateSkill(context.Background(), workspaceOf(t, pool, c), "幫我設定 AWS 憑證。")
	if err != nil {
		t.Fatalf("GenerateSkill: %v", err)
	}
	if !res.Report.Blocked {
		t.Fatal("a package with a credential-shaped line was accepted")
	}
	if stub.calls != 1 {
		t.Errorf("model called %d times; ADR-048 says once", stub.calls)
	}
	// The finding reaches the user verbatim, and never carries the matched value
	// (NFR-002, skillpkg.go's own rule).
	var found bool
	for _, f := range res.Report.Findings {
		if f.Code == "possible-secret" {
			found = true
			if strings.Contains(f.Message, "AKIA0123456789ABCDEF") {
				t.Error("the finding echoed the matched value")
			}
		}
	}
	if !found {
		t.Errorf("blocked for the wrong reason: %+v", res.Report.Findings)
	}
}

// GEN-007, as a reverse proof: the assertion that matters is the negative one,
// and a negative assertion passes for free if the skill was never created. So
// this checks both directions — present in the owner's own Skill list, absent
// from the owner's own search.
//
// The exclusion is on the read side on purpose. Deleting the search_documents
// row would also delete the static-scan facts the workspace list reads out of
// it, and 02:GEN-003 forbids a generated package disclosing one warning fewer
// than an imported one — the guarantee would have been bought by removing the
// disclosure.
func TestAGeneratedSkillIsNotFoundBySearchIncludingItsOwnCreator(t *testing.T) {
	pool := requireDB(t)
	stub := newGenerateStub(t, generatedSkill("zarquon-widget-collator",
		"# zarquon widget collator\n\nCollate zarquon widgets.\n"))
	a := newAPIWithLLM(t, pool, stub.URL)
	c := a.login(t, "gen-hidden")

	res, err := a.versions.GenerateSkill(context.Background(), workspaceOf(t, pool, c),
		"Collate my zarquon widgets into one report.")
	if err != nil || res.Report.Blocked {
		t.Fatalf("GenerateSkill: %v %+v", err, res.Report.Findings)
	}
	id := uuidString(res.Skill.ID)

	// Direction one: it exists and the owner can reach it. Without this the
	// second assertion would pass on a skill that was never created.
	if ids := c.skillIDs(t, "/skills"); !contains(ids, id) {
		t.Fatalf("the generated skill is missing from its own workspace list: %v", ids)
	}
	// Direction two: the owner's own search does not find it. The words are in
	// the name and the summary, so a document that took part would rank first.
	if ids := c.skillIDs(t, "/skills/search?q=zarquon+widget"); contains(ids, id) {
		t.Error("the creator found their own generated skill in search (GEN-007)")
	}

	// And the document itself still exists, carrying the scan facts the list
	// reads. If this ever fails, the exclusion moved to the write side and took
	// the disclosure with it.
	var documents int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM search_documents WHERE skill_id = $1", res.Skill.ID,
	).Scan(&documents); err != nil {
		t.Fatal(err)
	}
	if documents != 1 {
		t.Errorf("search_documents rows = %d, want 1: the row carries the scan facts", documents)
	}
}

// GEN-011 / ADR-047 決策 4. Nothing implements this — the fork path copies
// `redistribution` because 0036 needed it to — which is exactly why it needs a
// named test: if the copy is ever dropped, the value falls back to `unknown`,
// the download gate locks, and the owner cannot take away a package the platform
// wrote for them. Nothing goes red anywhere except the person.
func TestAForkOfAGeneratedSkillStaysGenerated(t *testing.T) {
	pool := requireDB(t)
	stub := newGenerateStub(t, generatedSkill("forkable-generated", "# 內容\n\n1. 做這件事。\n"))
	a := newAPIWithLLM(t, pool, stub.URL)
	c := a.login(t, "gen-forker")

	res, err := a.versions.GenerateSkill(context.Background(), workspaceOf(t, pool, c), "整理我的會議紀錄。")
	if err != nil || res.Report.Blocked {
		t.Fatalf("GenerateSkill: %v %+v", err, res.Report.Findings)
	}

	fork := postFork(t, c, uuidString(res.Skill.ID), http.StatusCreated)

	var redistribution string
	if err := pool.QueryRow(context.Background(),
		"SELECT redistribution FROM skills WHERE id = $1", mustUUID(t, fork.SkillID),
	).Scan(&redistribution); err != nil {
		t.Fatal(err)
	}
	if redistribution != "generated" {
		t.Errorf("fork redistribution = %q, want generated: falling back to unknown "+
			"locks the download the owner is entitled to", redistribution)
	}
}

// ADR-046 決策 1 puts generated content in a personal workspace and nowhere
// else, and GEN-007's exclusion depends on it: redistributionFor answers "" for
// a catalog workspace before it looks at the source type, so a package generated
// there would be `unknown` and would be searchable.
func TestTheCatalogueDoesNotGenerateSkills(t *testing.T) {
	pool := requireDB(t)
	stub := newGenerateStub(t) // no answers queued: a gateway call fails the test
	a := newAPIWithLLM(t, pool, stub.URL)
	c := a.login(t, "gen-curator")
	markCatalog(t, pool, c.workspaceID)

	_, err := a.versions.GenerateSkill(context.Background(), workspaceOf(t, pool, c), "任何任務。")
	if !errors.Is(err, ingest.ErrGenerateNotForCatalogue) {
		t.Fatalf("err = %v, want ErrGenerateNotForCatalogue", err)
	}
	if stub.calls != 0 {
		t.Errorf("the refusal still paid for %d gateway call(s)", stub.calls)
	}
}

func workspaceOf(t *testing.T, pool *pgxpool.Pool, c *client) identity.Workspace {
	t.Helper()
	ws, err := gen.New(pool).GetWorkspace(context.Background(), gen.GetWorkspaceParams{
		ID: mustUUID(t, c.workspaceID), OwnerUserID: mustUUID(t, c.userID),
	})
	if err != nil {
		t.Fatal(err)
	}
	return publishedWorkspace(ws)
}

func uuidString(id pgtype.UUID) string {
	s, err := id.Value()
	if err != nil || s == nil {
		return ""
	}
	return s.(string)
}
