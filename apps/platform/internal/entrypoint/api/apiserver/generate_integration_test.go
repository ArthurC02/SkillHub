package apiserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/apiserver"
	apigen "github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	gen "github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	policy "github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
)

// M5 generation, end to end against a stub model service (GEN-003, GEN-007,
// GEN-011).
//
// The first tests here drive ingest.Service directly — they predate the route,
// and the service is the same object the route calls, so what they exercise is
// everything below that line: the gateway call, the packaging, admission's one
// validation path, the retry decision, the rows, and what search does with the
// result. The later ones go through the mounted routes (POST /skills/generate,
// GET /skills/generate/failures) behind the exposure flag (ADR-052, GEN-008).

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
			// The same base URL serves index-time enrichment, which an upload in
			// one of these tests will reach. Refused rather than failed: import
			// treats an enrichment failure as "document left pending", which is a
			// path these tests do not care about. The assertion that matters is
			// the generate-call COUNT below, and it is separate.
			w.WriteHeader(http.StatusBadGateway)
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

// ADR-052's boundary, both directions. The default is off, and "off" has to
// mean the route is absent rather than refusing: a 403 or a 422 tells a probe
// the feature exists and is coming, which is enough for an entry point to be
// drawn somewhere. /me has to agree, because that is the only thing the web
// asks before deciding whether to draw one.
func TestTheGenerationEntryPointIsInvisibleUntilItIsExposed(t *testing.T) {
	pool := requireDB(t)

	off := newAPI(t, pool)
	c := off.login(t, "gen-flag-off")
	// The assertion is not a particular status code, it is SAMENESS: with the
	// flag off, /skills/generate must answer exactly what any other path under
	// /skills that nobody registered answers. (Today both are 405, because
	// DELETE /skills/{id} matches the shape — which is precisely why pinning 404
	// would have been asserting an accident.)
	absent, _ := postJSON(t, c, "/skills/zzz-not-a-route", `{}`)
	if code, _ := postJSON(t, c, "/skills/generate", `{"task_description":"任何任務"}`); code != absent {
		t.Errorf("POST /skills/generate with the flag off answered %d; an unregistered "+
			"sibling answers %d, and a different answer is how a probe finds the feature", code, absent)
	}
	if features(t, c) != nil {
		t.Errorf("/me advertised features with the flag off: %v", features(t, c))
	}

	on := newAPIExposingGenerate(t, pool)
	c2 := on.login(t, "gen-flag-on")
	if code, _ := postJSON(t, c2, "/skills/generate", `{"task_description":""}`); code == absent {
		t.Errorf("POST /skills/generate with the flag on still answers %d", code)
	}
	if f := features(t, c2); f == nil || !f["generate_skill"] {
		t.Errorf("/me did not advertise generate_skill with the flag on: %v", f)
	}
}

// A blank description is refused before the gateway, and the refusal says what
// to add rather than only that it was refused (02:GEN-001, same discipline
// DISC-001 already applies to an empty search).
func TestABlankTaskDescriptionIsRefusedWithAdvice(t *testing.T) {
	pool := requireDB(t)
	stub := newGenerateStub(t) // no answers: a gateway call fails the test
	a := newAPIExposingGenerate(t, pool, stub.URL)
	c := a.login(t, "gen-blank")

	code, body := postJSON(t, c, "/skills/generate", `{"task_description":"    "}`)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422", code)
	}
	msg, _ := body["error"].(string)
	if !strings.Contains(msg, "預期產出") {
		t.Errorf("refusal gives no advice: %q", msg)
	}
	if stub.calls != 0 {
		t.Errorf("a blank description paid for %d gateway call(s)", stub.calls)
	}
}

func features(t *testing.T, c *client) map[string]bool {
	t.Helper()
	resp, err := c.Get(c.base + "/me")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		Features map[string]bool `json:"features"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out.Features
}

// newAPIExposingGenerate is newAPIWithLLM with ADR-052's flag on. It goes
// through newAPITuned's hook for the reason that hook exists: the exposure flag
// changes which routes are in the table at all, so it has to be set before the
// table is built, not after.
func newAPIExposingGenerate(t *testing.T, pool *pgxpool.Pool, llmBaseURL ...string) *api {
	t.Helper()
	base := ""
	if len(llmBaseURL) > 0 {
		base = llmBaseURL[0]
	}
	return newAPITuned(t, pool, base, func(d *apiserver.Deps) {
		d.GenerateExposed = true
		// The other half of the same switch: cmd/api sets both from one env var,
		// and a test that flipped only the route would be asserting a state no
		// deployment can be in.
		d.Auth.Features = map[string]bool{"generate_skill": true}
	})
}

// `skills.redistribution` is decided when the skills row is created and never
// revisited, and GEN-007's search exclusion keys on that column. So a manifest
// name that collides with an existing skill in the same workspace mixes the two
// kinds of content under one verdict, in both directions and with no symptom
// either way:
//
//   - generated content landing on an uploaded skill keeps `self_supplied`, and
//     becomes searchable — including to its own creator, which is the one thing
//     GEN-007 promises;
//   - an upload landing on a generated skill keeps `generated`, and the user's
//     own upload can never be found again.
//
// The model is asked for a name derived from the task, so `pdf-extract`
// colliding with an uploaded `pdf-extract` is ordinary, not exotic.
func TestAGeneratedNameCollisionIsRefusedInBothDirections(t *testing.T) {
	pool := requireDB(t)

	t.Run("generating onto an uploaded skill", func(t *testing.T) {
		stub := newGenerateStub(t, generatedSkill("pdf-extract", "# 內容\n\n1. 做這件事。\n"))
		a := newAPIWithLLM(t, pool, stub.URL)
		c := a.login(t, "gen-collide-a")
		importFiles(t, a, pool, c, map[string]string{
			"SKILL.md": "---\nname: pdf-extract\ndescription: An uploaded one.\n---\n\nDo it.\n",
		})

		_, err := a.versions.GenerateSkill(context.Background(), workspaceOf(t, pool, c), "抽出 PDF 文字。")
		if !errors.Is(err, ingest.ErrGeneratedNameCollision) {
			t.Fatalf("err = %v, want ErrGeneratedNameCollision", err)
		}
	})

	t.Run("uploading onto a generated skill", func(t *testing.T) {
		stub := newGenerateStub(t, generatedSkill("pdf-extract", "# 內容\n\n1. 做這件事。\n"))
		a := newAPIWithLLM(t, pool, stub.URL)
		c := a.login(t, "gen-collide-b")
		if _, err := a.versions.GenerateSkill(context.Background(), workspaceOf(t, pool, c), "抽出 PDF 文字。"); err != nil {
			t.Fatalf("GenerateSkill: %v", err)
		}

		ws := workspaceOf(t, pool, c)
		_, err := a.versions.UploadZip(context.Background(), ws, zipOf(t, map[string]string{
			"SKILL.md": "---\nname: pdf-extract\ndescription: An uploaded one.\n---\n\nDo it.\n",
		}))
		if !errors.Is(err, ingest.ErrGeneratedNameCollision) {
			t.Fatalf("err = %v, want ErrGeneratedNameCollision", err)
		}
	})

	// The third direction, which the guard used to let through: regenerating the
	// same task takes the same name from the same model, and the second
	// generation landed on the first — as version N with different bytes, or as
	// persistVersion's duplicate early-return with identical ones, which writes
	// no skill_sources row (a paid generation the allowance never counted) and
	// answered 201 「已經產生一個 Skill」 about nothing. 02:GEN-001 says 「第一個版本」.
	t.Run("generating onto a generated skill", func(t *testing.T) {
		same := generatedSkill("pdf-extract", "# 內容\n\n1. 做這件事。\n")
		stub := newGenerateStub(t, same, same)
		a := newAPIWithLLM(t, pool, stub.URL)
		c := a.login(t, "gen-collide-c")
		ws := workspaceOf(t, pool, c)
		if _, err := a.versions.GenerateSkill(context.Background(), ws, "抽出 PDF 文字。"); err != nil {
			t.Fatalf("first GenerateSkill: %v", err)
		}
		_, err := a.versions.GenerateSkill(context.Background(), ws, "抽出 PDF 文字。")
		if !errors.Is(err, ingest.ErrGeneratedNameCollision) {
			t.Fatalf("second generation of the same name: err = %v, want ErrGeneratedNameCollision", err)
		}
		// One generation happened, and the counter that bills generations says so.
		var used int64
		if err := pool.QueryRow(context.Background(),
			`SELECT count(*) FROM skill_sources WHERE workspace_id = $1 AND source_type = 'generated'`, ws.ID,
		).Scan(&used); err != nil {
			t.Fatal(err)
		}
		if used != 1 {
			t.Errorf("skill_sources counts %d generated rows, want 1", used)
		}
	})

	// The fourth direction, and the one importZip's guard could never see:
	// POST /skills/{id}/versions names the target by id, so there is no name to
	// collide and no importZip on the path. SaveVersion hardcoded
	// sourceMeta{Type: "upload"} and wrote the version, `skills.redistribution`
	// stayed `generated` because it is decided at creation and never recomputed,
	// and GEN-007's exclusion went on applying — to content the user wrote
	// themselves. It succeeded, answered 201, and the skill was permanently
	// unfindable with no symptom on any screen. http.go's refusal sentence
	// promises the platform will not do this; until the guard moved down into
	// persistVersion — which both writers share — that sentence was false.
	t.Run("saving a version onto a generated skill", func(t *testing.T) {
		stub := newGenerateStub(t, generatedSkill("pdf-extract", "# 內容\n\n1. 做這件事。\n"))
		a := newAPIWithLLM(t, pool, stub.URL)
		c := a.login(t, "gen-collide-d")
		ws := workspaceOf(t, pool, c)
		res, err := a.versions.GenerateSkill(context.Background(), ws, "抽出 PDF 文字。")
		if err != nil {
			t.Fatalf("GenerateSkill: %v", err)
		}

		_, err = a.versions.SaveVersion(context.Background(), ws, res.Skill.ID, zipOf(t, map[string]string{
			"SKILL.md": "---\nname: pdf-extract\ndescription: My own second version.\n---\n\nI wrote this.\n",
		}))
		if !errors.Is(err, ingest.ErrGeneratedNameCollision) {
			t.Fatalf("SaveVersion onto a generated skill: err = %v, want ErrGeneratedNameCollision", err)
		}

		// Nothing was written by the refusal: no second source row, and the
		// generated skill still has exactly the one version 02:GEN-001 promises.
		var sources, versions int64
		if err := pool.QueryRow(context.Background(),
			`SELECT count(*) FROM skill_sources WHERE workspace_id = $1`, ws.ID,
		).Scan(&sources); err != nil {
			t.Fatal(err)
		}
		if sources != 1 {
			t.Errorf("skill_sources counts %d rows in the workspace, want 1 — the refused upload wrote one", sources)
		}
		if err := pool.QueryRow(context.Background(),
			`SELECT count(*) FROM skill_versions WHERE skill_id = $1`, res.Skill.ID,
		).Scan(&versions); err != nil {
			t.Fatal(err)
		}
		if versions != 1 {
			t.Errorf("the generated skill has %d versions, want 1", versions)
		}
	})
}

// The allowance could not be counted: 503 with a static sentence, not the 502
// the gateway gets and not the 422 「用完了」 an exhausted allowance gets
// (d555564 split the sentinels; this pins the generate handler's mapping).
// The pool handed to ingest points at a database that is not there; identity
// keeps the real pool, so the session and workspace lookups still work and the
// request reaches the allowance check and nothing past it — the stub has no
// answers queued, so a gateway call would fail the test by itself.
func TestAnUncountableAllowanceIsA503NotAnExhaustedOne(t *testing.T) {
	pool := requireDB(t)
	stub := newGenerateStub(t)
	deadPool, err := pgxpool.New(context.Background(), "postgres://nobody:nobody@127.0.0.1:1/nope?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(deadPool.Close)
	a := newAPITuned(t, pool, stub.URL, func(d *apiserver.Deps) {
		d.GenerateExposed = true
		d.Auth.Features = map[string]bool{"generate_skill": true}
		d.Importer.Svc.GenerateQuota = policy.DefaultGenerateQuotaLimits()
		d.Importer.Svc.Pool = deadPool
	})
	c := a.login(t, "gen-503")

	code, body := postJSON(t, c, "/skills/generate", `{"task_description":"把掃描的單據整理成一張表"}`)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("got %d %v, want 503", code, body)
	}
	msg, _ := body["error"].(string)
	if strings.Contains(msg, "額度已用完") || strings.Contains(msg, "used its free") {
		t.Errorf("an uncountable allowance was reported as an exhausted one: %q", msg)
	}
	if strings.Contains(msg, "nobody") || strings.Contains(msg, "127.0.0.1") {
		t.Errorf("the connection string reached the response body: %q", msg)
	}
	if stub.calls != 0 {
		t.Errorf("an uncountable allowance paid for %d gateway call(s)", stub.calls)
	}
}

// ADR-052's flag is deployment-wide; the permission behind the route is not.
// POST /skills/generate is RequireInvited, so an ordinary signed-in account
// outside the beta cohort was being shown the entry point, typing a
// description, waiting, and getting a 403 with an English paragraph on a
// Chinese page. `/me` now answers per caller.
func TestTheEntryPointIsNotAdvertisedToSomeoneWhoMayNotUseIt(t *testing.T) {
	pool := requireDB(t)
	a := newAPITuned(t, pool, "", func(d *apiserver.Deps) {
		d.GenerateExposed = true
		d.Auth.Features = map[string]bool{"generate_skill": true}
		// A cohort that exists and does not contain this test's user. The dev
		// login mints provider ids from the name, so a literal nobody-id closes it.
		d.Auth.Invited = map[string]bool{"a-provider-id-nobody-holds": true}
	})
	c := a.login(t, "gen-uninvited")

	if f := features(t, c); f != nil {
		t.Errorf("/me advertised %v to an account the route refuses", f)
	}
	if code, _ := postJSON(t, c, "/skills/generate", `{"task_description":"把掃描的單據整理成表格。"}`); code != http.StatusForbidden {
		t.Errorf("POST /skills/generate = %d, want 403 — the test's premise is that this caller is refused", code)
	}
}

// ADR-052 left 「曝光旗標要不要有稽核事件」 open and named its own weakness in the
// same paragraph: 「沒有任何機制會告訴我們它被誤開過」. This is that mechanism.
// Written even when everything is off, because "nothing was exposed on this
// deployment, from this time" is the statement somebody will need, and an absent
// row is equally consistent with a build that predates the flag.
func TestTheExposureFlagLeavesATraceEitherWay(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()

	for _, tc := range []struct {
		name    string
		exposed bool
		want    string
	}{
		{"off", false, "[]"},
		{"on", true, `["generate_skill"]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Audit rows are immutable by trigger (ADR-003), so the test reads
			// forward from a watermark rather than clearing the table.
			var before int64
			if err := pool.QueryRow(ctx,
				"SELECT coalesce(max(id), 0) FROM audit_events",
			).Scan(&before); err != nil {
				t.Fatal(err)
			}
			a := newAPITuned(t, pool, "", func(d *apiserver.Deps) {
				d.GenerateExposed = tc.exposed
				if tc.exposed {
					d.Auth.Features = map[string]bool{"generate_skill": true}
				}
			})
			a.app.AuditRosters(ctx)

			var enabled string
			if err := pool.QueryRow(ctx,
				`SELECT metadata->>'enabled' FROM audit_events
				 WHERE action = $1 AND id > $2 ORDER BY id DESC LIMIT 1`,
				"feature_flags.roster", before,
			).Scan(&enabled); err != nil {
				t.Fatalf("no feature-flag audit row: %v", err)
			}
			if enabled != tc.want {
				t.Errorf("enabled = %s, want %s", enabled, tc.want)
			}
		})
	}
}

// GEN-003's last clause: 「在工作區留下可查的失敗紀錄」.
//
// The write half has existed since the first pass, and for a while that was
// counted as the criterion being met. It was not: a row only the person holding
// a database connection can see is not a record left in the workspace, and the
// failure has no symptom — the write succeeds, the tests of the write pass, and
// the user sees nothing at all.
func TestARefusedGenerationIsReadableAfterwards(t *testing.T) {
	pool := requireDB(t)
	// An invalid name is a structural finding, so it blocks AND is retried once
	// (ADR-047 決策 1 / ADR-048) — hence two queued answers, and hence attempts=2
	// below, which is the number the failure screen's retry sentence depends on.
	bad := generatedSkill("Not A Valid Name!", "步驟一：把每一份掃描件轉成表格。")
	stub := newGenerateStub(t, bad, bad)
	a := newAPIExposingGenerate(t, pool, stub.URL)
	c := a.login(t, "gen-history")

	if code, _ := postJSON(t, c, "/skills/generate", `{"task_description":"把掃描的單據整理成一張表"}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("expected the empty body to be blocked, got %d", code)
	}

	var out struct {
		Failures []struct {
			OccurredAt string   `json:"occurred_at"`
			Failure    string   `json:"failure"`
			Codes      []string `json:"codes"`
			Attempts   int      `json:"attempts"`
		} `json:"failures"`
	}
	if code := getJSON(t, c.Client, c.base+"/skills/generate/failures", &out); code != http.StatusOK {
		t.Fatalf("GET failures: %d", code)
	}
	if len(out.Failures) != 1 {
		t.Fatalf("got %d failure rows, want 1: %+v", len(out.Failures), out.Failures)
	}
	f := out.Failures[0]
	if f.Failure != "blocked" {
		t.Errorf("failure = %q, want blocked", f.Failure)
	}
	if len(f.Codes) == 0 {
		t.Error("a blocked failure with no codes tells the user nothing they can act on")
	}
	if f.Attempts != 2 {
		t.Errorf("attempts = %d, want 2 — a blocked report is retried once", f.Attempts)
	}
	if f.OccurredAt == "" {
		t.Error("no timestamp: 「可查」 means the user can tell which attempt this was")
	}
}

// The task description is deliberately not in the audit row (it belongs to the
// skill_sources row, under NFR-002 deletion, while audit rows are kept 400 days
// under a different rule). The read path must not be the place that puts it
// back — nothing would report it, and the retention promise would be broken by
// a screen rather than by a schema.
func TestTheFailureHistoryDoesNotEchoTheTaskDescription(t *testing.T) {
	pool := requireDB(t)
	bad := generatedSkill("Not A Valid Name!", "步驟一：把每一份掃描件轉成表格。")
	stub := newGenerateStub(t, bad, bad)
	a := newAPIExposingGenerate(t, pool, stub.URL)
	c := a.login(t, "gen-history-quiet")

	const secretish = "把客戶名單裡的每一列都整理好"
	postJSON(t, c, "/skills/generate", `{"task_description":"`+secretish+`"}`)

	resp, err := c.Get(c.base + "/skills/generate/failures")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), secretish) {
		t.Errorf("the failure history echoed the task description back: %s", body)
	}
}

// Iron rule 3. The rows are workspace-scoped in SQL and not actor-scoped, and in
// a population where every workspace has exactly one member the two queries
// return the same thing — a second login is a second actor AND a second
// workspace, and tells them apart no better than the first. So this test writes
// a row the two scopes disagree on: actor = A, workspace = B's. Workspace scope
// shows it to B and hides it from A; actor scope would do the opposite.
func TestTheFailureHistoryIsScopedByWorkspaceNotByActor(t *testing.T) {
	pool := requireDB(t)
	a := newAPIExposingGenerate(t, pool)
	alice := a.login(t, "gen-scope-alice")
	bob := a.login(t, "gen-scope-bob")
	aliceWS := workspaceOf(t, pool, alice)
	bobWS := workspaceOf(t, pool, bob)

	// Written the way the product writes it, minus the product: audit.Log with
	// Alice as actor and Bob's workspace as the scope.
	if err := audit.Log(context.Background(), pool, audit.Event{
		Actor:        aliceWS.OwnerUserID,
		Workspace:    bobWS.ID,
		Action:       audit.ActionSkillGenerateFailed,
		ResourceType: audit.ResourceSkill,
		Metadata:     map[string]any{"failure": "gateway", "attempts": 1},
	}); err != nil {
		t.Fatal(err)
	}

	var out struct {
		Failures []map[string]any `json:"failures"`
	}
	if code := getJSON(t, bob.Client, bob.base+"/skills/generate/failures", &out); code != http.StatusOK {
		t.Fatalf("bob GET failures: %d", code)
	}
	if len(out.Failures) != 1 {
		t.Errorf("the row is in Bob's workspace and Bob read %d rows; the query is not workspace-scoped", len(out.Failures))
	}
	out.Failures = nil
	if code := getJSON(t, alice.Client, alice.base+"/skills/generate/failures", &out); code != http.StatusOK {
		t.Fatalf("alice GET failures: %d", code)
	}
	if len(out.Failures) != 0 {
		t.Errorf("Alice is the actor but not the workspace and read %d rows; the query is actor-scoped", len(out.Failures))
	}
}

// The failure vocabulary lives in four places — ingest's constants, the
// contract's enum, the generated client's enum, the web sentence table — and
// nothing links them. This pins ingest to the contract (via ogen's generated
// enum); generate.test.tsx pins the web table to the generated TS client. A
// value written here and missing there would reach the screen as "unreadable".
func TestEveryFailureValueIngestWritesIsInTheContract(t *testing.T) {
	known := map[string]bool{}
	for _, v := range apigen.GenerationFailureFailure("").AllValues() {
		known[string(v)] = true
	}
	for _, v := range ingest.FailureVocabulary {
		if !known[v] {
			t.Errorf("ingest writes failure=%q, which the contract's GenerationFailure.failure enum does not list", v)
		}
	}
}

// 04 丙-54 was closed as if the limiter already covered generation. It did not:
// limited() wrapped the two import routes and anonymous search, and a token
// bucket is only debited by the handlers it wraps, so a loop that touched
// nothing but POST /skills/generate spent no tokens at all. The three things
// that look like they cover it do not — the single-slot gate is about
// concurrency, GENERATE_QUOTA may be `off`, and a generation that fails
// validation commits no skill_sources row and so never reaches the allowance,
// having already paid for the gateway call on the shared static key.
//
// The caller here is outside the invited cohort, so every request that gets
// past the limiter is refused by RequireInvited before any gateway call: what
// this pins is that limited() is on the route AND outside the auth wrappers,
// which is where the other three sit and why the shield covers the
// authentication path too. A 403 five times over is the failure this asserts
// against.
func TestGenerationIsRateLimitedWhenALimiterIsConfigured(t *testing.T) {
	pool := requireDB(t)
	a := newAPITuned(t, pool, "", func(d *apiserver.Deps) {
		d.GenerateExposed = true
		d.Auth.Features = map[string]bool{"generate_skill": true}
		d.Auth.Invited = map[string]bool{"a-provider-id-nobody-holds": true}
		d.Limits = httpx.NewRateLimiter(60, 2)
	})
	c := a.login(t, "gen-ratelimit")

	codes := []int{}
	for i := 0; i < 5; i++ {
		resp, err := c.Post(c.base+"/skills/generate", "application/json",
			strings.NewReader(`{"task_description":"把掃描的單據整理成一張表"}`))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		codes = append(codes, resp.StatusCode)
		if resp.StatusCode == http.StatusTooManyRequests {
			if resp.Header.Get("Retry-After") == "" {
				t.Error("429 without Retry-After")
			}
			return
		}
	}
	t.Fatalf("five POSTs to /skills/generate against a burst of two never saw a 429: %v", codes)
}
