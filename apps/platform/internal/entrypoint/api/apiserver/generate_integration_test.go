package apiserver_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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

			"usage": map[string]any{
				"prompt_tokens": 1200, "completion_tokens": 800,
				"cost_usd": 0.0123, "cost_source": "gateway",
			},
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

func TestGeneratedSkillLandsAsAVersionWithItsOwnProvenance(t *testing.T) {
	pool := requireDB(t)
	stub := newGenerateStub(t, generatedSkill("invoice-to-table", "# 掃描單據轉表格\n\n1. 逐份抽出表格。\n"))
	a := newAPIWithLLM(t, pool, stub.URL)
	c := a.login(t, "gen-owner")

	task := "我每個月要把廠商寄來的掃描單據整理成一份表格交出去。"
	res, err := a.versions.GenerateSkill(context.Background(), workspaceOf(t, pool, c), ingest.GenerateInput{TaskDescription: task})
	if err != nil {
		t.Fatalf("GenerateSkill: %v", err)
	}
	if res.Report.Blocked {
		t.Fatalf("blocked: %+v", res.Report.Findings)
	}
	if res.Attempts != 1 || stub.calls != 1 {
		t.Errorf("attempts = %d, calls = %d, want 1 and 1", res.Attempts, stub.calls)
	}

	var cost float64
	if err := pool.QueryRow(context.Background(), `
		SELECT (metadata->>'cost_usd')::float8 FROM audit_events
		WHERE action = 'skill.import' AND resource_id = $1`, res.Version.ID,
	).Scan(&cost); err != nil {
		t.Fatalf("the successful generation's audit row carries no cost: %v", err)
	}
	if cost != 0.0123 {
		t.Errorf("cost_usd = %v, want 0.0123 (what the gateway charged)", cost)
	}

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

func TestGenerationRetriesExactlyOnce(t *testing.T) {
	pool := requireDB(t)
	bad := generatedSkill("retry-me", "# 內容\n")
	bad["files"] = []any{map[string]any{"path": "../../evil.sh", "content": "echo hi\n"}}
	stub := newGenerateStub(t, bad, generatedSkill("retry-me", "# 內容\n\n1. 做這件事。\n"))

	a := newAPIWithLLM(t, pool, stub.URL)
	c := a.login(t, "gen-retry")

	res, err := a.versions.GenerateSkill(context.Background(), workspaceOf(t, pool, c), ingest.GenerateInput{TaskDescription: "把 PDF 轉成純文字。"})
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

func TestPossibleSecretIsNotRetriedEndToEnd(t *testing.T) {
	pool := requireDB(t)
	leaky := generatedSkill("leaky-setup", "# 設定\n\n照 setup.sh 執行。\n")
	leaky["files"] = []any{map[string]any{
		"path":    "setup.sh",
		"content": "export AWS_ACCESS_KEY_ID=AKIA0123456789ABCDEF\n",
	}}

	stub := newGenerateStub(t, leaky)

	a := newAPIWithLLM(t, pool, stub.URL)
	c := a.login(t, "gen-secret")

	res, err := a.versions.GenerateSkill(context.Background(), workspaceOf(t, pool, c), ingest.GenerateInput{TaskDescription: "幫我設定 AWS 憑證。"})
	if err != nil {
		t.Fatalf("GenerateSkill: %v", err)
	}
	if !res.Report.Blocked {
		t.Fatal("a package with a credential-shaped line was accepted")
	}
	if stub.calls != 1 {
		t.Errorf("model called %d times; ADR-048 says once", stub.calls)
	}

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

func TestAGeneratedSkillIsNotFoundBySearchIncludingItsOwnCreator(t *testing.T) {
	pool := requireDB(t)
	stub := newGenerateStub(t, generatedSkill("zarquon-widget-collator",
		"# zarquon widget collator\n\nCollate zarquon widgets.\n"))
	a := newAPIWithLLM(t, pool, stub.URL)
	c := a.login(t, "gen-hidden")

	res, err := a.versions.GenerateSkill(context.Background(), workspaceOf(t, pool, c),
		ingest.GenerateInput{TaskDescription: "Collate my zarquon widgets into one report."})
	if err != nil || res.Report.Blocked {
		t.Fatalf("GenerateSkill: %v %+v", err, res.Report.Findings)
	}
	id := uuidString(res.Skill.ID)

	if ids := c.skillIDs(t, "/skills"); !contains(ids, id) {
		t.Fatalf("the generated skill is missing from its own workspace list: %v", ids)
	}

	if ids := c.skillIDs(t, "/skills/search?q=zarquon+widget"); contains(ids, id) {
		t.Error("the creator found their own generated skill in search (GEN-007)")
	}

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

func TestAForkOfAGeneratedSkillStaysGenerated(t *testing.T) {
	pool := requireDB(t)
	stub := newGenerateStub(t, generatedSkill("forkable-generated", "# 內容\n\n1. 做這件事。\n"))
	a := newAPIWithLLM(t, pool, stub.URL)
	c := a.login(t, "gen-forker")

	res, err := a.versions.GenerateSkill(context.Background(), workspaceOf(t, pool, c), ingest.GenerateInput{TaskDescription: "整理我的會議紀錄。"})
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

func TestTheCatalogueDoesNotGenerateSkills(t *testing.T) {
	pool := requireDB(t)
	stub := newGenerateStub(t)
	a := newAPIWithLLM(t, pool, stub.URL)
	c := a.login(t, "gen-curator")
	markCatalog(t, pool, c.workspaceID)

	_, err := a.versions.GenerateSkill(context.Background(), workspaceOf(t, pool, c), ingest.GenerateInput{TaskDescription: "任何任務。"})
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

func TestTheGenerationEntryPointIsInvisibleUntilItIsExposed(t *testing.T) {
	pool := requireDB(t)

	off := newAPI(t, pool)
	c := off.login(t, "gen-flag-off")

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

func TestABlankTaskDescriptionIsRefusedWithAdvice(t *testing.T) {
	pool := requireDB(t)
	stub := newGenerateStub(t)
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

func newAPIExposingGenerate(t *testing.T, pool *pgxpool.Pool, llmBaseURL ...string) *api {
	t.Helper()
	base := ""
	if len(llmBaseURL) > 0 {
		base = llmBaseURL[0]
	}
	return newAPITuned(t, pool, base, func(d *apiserver.Deps) {
		d.GenerateExposed = true

		d.Auth.Features = map[string]bool{"generate_skill": true}
	})
}

func TestAGeneratedNameCollisionIsRefusedInBothDirections(t *testing.T) {
	pool := requireDB(t)

	t.Run("generating onto an uploaded skill", func(t *testing.T) {
		stub := newGenerateStub(t, generatedSkill("pdf-extract", "# 內容\n\n1. 做這件事。\n"))
		a := newAPIWithLLM(t, pool, stub.URL)
		c := a.login(t, "gen-collide-a")
		importFiles(t, a, pool, c, map[string]string{
			"SKILL.md": "---\nname: pdf-extract\ndescription: An uploaded one.\n---\n\nDo it.\n",
		})

		_, err := a.versions.GenerateSkill(context.Background(), workspaceOf(t, pool, c), ingest.GenerateInput{TaskDescription: "抽出 PDF 文字。"})
		if !errors.Is(err, ingest.ErrGeneratedNameCollision) {
			t.Fatalf("err = %v, want ErrGeneratedNameCollision", err)
		}
	})

	t.Run("uploading onto a generated skill", func(t *testing.T) {
		stub := newGenerateStub(t, generatedSkill("pdf-extract", "# 內容\n\n1. 做這件事。\n"))
		a := newAPIWithLLM(t, pool, stub.URL)
		c := a.login(t, "gen-collide-b")
		if _, err := a.versions.GenerateSkill(context.Background(), workspaceOf(t, pool, c), ingest.GenerateInput{TaskDescription: "抽出 PDF 文字。"}); err != nil {
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

	t.Run("generating onto a generated skill", func(t *testing.T) {
		same := generatedSkill("pdf-extract", "# 內容\n\n1. 做這件事。\n")
		stub := newGenerateStub(t, same, same)
		a := newAPIWithLLM(t, pool, stub.URL)
		c := a.login(t, "gen-collide-c")
		ws := workspaceOf(t, pool, c)
		if _, err := a.versions.GenerateSkill(context.Background(), ws, ingest.GenerateInput{TaskDescription: "抽出 PDF 文字。"}); err != nil {
			t.Fatalf("first GenerateSkill: %v", err)
		}
		_, err := a.versions.GenerateSkill(context.Background(), ws, ingest.GenerateInput{TaskDescription: "抽出 PDF 文字。"})
		if !errors.Is(err, ingest.ErrGeneratedNameCollision) {
			t.Fatalf("second generation of the same name: err = %v, want ErrGeneratedNameCollision", err)
		}

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

	t.Run("saving a version onto a generated skill", func(t *testing.T) {
		stub := newGenerateStub(t, generatedSkill("pdf-extract", "# 內容\n\n1. 做這件事。\n"))
		a := newAPIWithLLM(t, pool, stub.URL)
		c := a.login(t, "gen-collide-d")
		ws := workspaceOf(t, pool, c)
		res, err := a.versions.GenerateSkill(context.Background(), ws, ingest.GenerateInput{TaskDescription: "抽出 PDF 文字。"})
		if err != nil {
			t.Fatalf("GenerateSkill: %v", err)
		}

		_, err = a.versions.SaveVersion(context.Background(), ws, res.Skill.ID, zipOf(t, map[string]string{
			"SKILL.md": "---\nname: pdf-extract\ndescription: My own second version.\n---\n\nI wrote this.\n",
		}))
		if !errors.Is(err, ingest.ErrGeneratedNameCollision) {
			t.Fatalf("SaveVersion onto a generated skill: err = %v, want ErrGeneratedNameCollision", err)
		}

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

func TestTheEntryPointIsNotAdvertisedToSomeoneWhoMayNotUseIt(t *testing.T) {
	pool := requireDB(t)
	a := newAPITuned(t, pool, "", func(d *apiserver.Deps) {
		d.GenerateExposed = true
		d.Auth.Features = map[string]bool{"generate_skill": true}

		d.Auth.Invited = map[string]bool{"gen-invited": true}
	})
	c := a.login(t, "gen-uninvited")

	if f := features(t, c); f != nil {
		t.Errorf("/me advertised %v to an account the route refuses", f)
	}

	for _, gated := range []struct{ name, method, path, body string }{
		{"POST /skills/generate", http.MethodPost, "/skills/generate", `{"task_description":"把掃描的單據整理成表格。"}`},
		{"GET /skills/generate/failures", http.MethodGet, "/skills/generate/failures", ""},
	} {
		code, body := c.doJSON(t, gated.method, gated.path, gated.body)
		if code != http.StatusForbidden {
			t.Errorf("%s = %d, want 403 — the test's premise is that this caller is refused", gated.name, code)
		}

		if msg, _ := body["error"].(string); !strings.Contains(msg, "closed beta") {
			t.Errorf("%s refused an uninvited user for some other reason: %v", gated.name, body)
		}
	}

	invited := a.login(t, "gen-invited")
	if code, _ := invited.doJSON(t, http.MethodGet, "/skills/generate/failures", ""); code != http.StatusOK {
		t.Errorf("GET /skills/generate/failures as an invited user = %d, want 200", code)
	}
}

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

func TestARefusedGenerationIsReadableAfterwards(t *testing.T) {
	pool := requireDB(t)

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

func TestTheFailureHistoryIsScopedByWorkspaceNotByActor(t *testing.T) {
	pool := requireDB(t)
	a := newAPIExposingGenerate(t, pool)
	alice := a.login(t, "gen-scope-alice")
	bob := a.login(t, "gen-scope-bob")
	aliceWS := workspaceOf(t, pool, alice)
	bobWS := workspaceOf(t, pool, bob)

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

func TestADiagramOnlyGenerationIsCreated(t *testing.T) {
	pool := requireDB(t)
	stub := newGenerateStub(t, generatedSkill("diagram-only-flow", "# 流程圖轉來的技能\n\n1. 照圖示做。\n"))
	a := newAPIExposingGenerate(t, pool, stub.URL)
	c := a.login(t, "gen-diagram-only")

	diagram := []byte("not a real png, just some bytes to hash")
	body := `{"diagram":{"media_type":"image/png","data":"` + base64.StdEncoding.EncodeToString(diagram) + `"}}`
	code, resp := postJSON(t, c, "/skills/generate", body)
	if code != http.StatusCreated {
		t.Fatalf("got %d %v, want 201", code, resp)
	}
	if stub.calls != 1 {
		t.Errorf("model called %d times, want 1", stub.calls)
	}

	versionID, _ := resp["version_id"].(string)
	var taskDescription string
	var generationInputs []byte
	if err := pool.QueryRow(context.Background(), `
		SELECT s.task_description, s.generation_inputs
		FROM skill_sources s JOIN skill_versions v ON v.source_id = s.id
		WHERE v.id = $1`, mustUUID(t, versionID),
	).Scan(&taskDescription, &generationInputs); err != nil {
		t.Fatal(err)
	}
	if taskDescription != "" {
		t.Errorf("task_description = %q, want empty: no description was given", taskDescription)
	}
	sum := sha256.Sum256(diagram)
	var got struct {
		Diagram struct {
			MediaType string `json:"media_type"`
			SHA256    string `json:"sha256"`
			Bytes     int    `json:"bytes"`
		} `json:"diagram"`
	}
	if err := json.Unmarshal(generationInputs, &got); err != nil {
		t.Fatalf("generation_inputs did not decode: %v (%s)", err, generationInputs)
	}
	if got.Diagram.MediaType != "image/png" || got.Diagram.Bytes != len(diagram) ||
		got.Diagram.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("generation_inputs.diagram = %+v, want media_type/bytes/sha256 of the sent image", got.Diagram)
	}
	if strings.Contains(string(generationInputs), base64.StdEncoding.EncodeToString(diagram)) {
		t.Error("the image bytes themselves leaked into generation_inputs")
	}

	skillID, _ := resp["skill_id"].(string)
	var viaAPI struct {
		Source *struct {
			GenerationInputs json.RawMessage `json:"generation_inputs"`
		} `json:"source"`
	}
	if code := getJSON(t, c.Client, c.base+"/api/skills/"+skillID, &viaAPI); code != http.StatusOK {
		t.Fatalf("GET /api/skills/%s: %d", skillID, code)
	}
	if viaAPI.Source == nil || len(viaAPI.Source.GenerationInputs) == 0 {
		t.Fatalf("source.generation_inputs is absent from the detail response: %+v", viaAPI)
	}
	var gotViaAPI struct {
		Diagram struct {
			MediaType string `json:"media_type"`
			SHA256    string `json:"sha256"`
			Bytes     int    `json:"bytes"`
		} `json:"diagram"`
	}
	if err := json.Unmarshal(viaAPI.Source.GenerationInputs, &gotViaAPI); err != nil {
		t.Fatalf("the detail response's generation_inputs did not decode: %v", err)
	}
	if gotViaAPI.Diagram.MediaType != "image/png" || gotViaAPI.Diagram.Bytes != len(diagram) ||
		gotViaAPI.Diagram.SHA256 != hex.EncodeToString(sum[:]) {
		t.Errorf("detail response generation_inputs.diagram = %+v, want the sent image's digest", gotViaAPI.Diagram)
	}
}

func TestAReferenceFromTheCallersOwnWorkspaceIsUsed(t *testing.T) {
	pool := requireDB(t)
	stub := newGenerateStub(t, generatedSkill("built-from-a-reference", "# 參考既有 Skill\n\n1. 照範例做。\n"))
	a := newAPIExposingGenerate(t, pool, stub.URL)
	c := a.login(t, "gen-reference-own")

	refSkillID, refVersionID := importFiles(t, a, pool, c, map[string]string{
		"SKILL.md": "---\nname: reference-source\ndescription: An existing skill to read as a worked example.\n---\n\nDo the thing well.\n",
	})

	body := `{"task_description":"照現有 Skill 的風格再做一個。","reference_skill_ids":["` + refSkillID + `"]}`
	code, resp := postJSON(t, c, "/skills/generate", body)
	if code != http.StatusCreated {
		t.Fatalf("got %d %v, want 201", code, resp)
	}

	versionID, _ := resp["version_id"].(string)
	var generationInputs []byte
	if err := pool.QueryRow(context.Background(), `
		SELECT s.generation_inputs FROM skill_sources s JOIN skill_versions v ON v.source_id = s.id
		WHERE v.id = $1`, mustUUID(t, versionID),
	).Scan(&generationInputs); err != nil {
		t.Fatal(err)
	}
	var got struct {
		References []struct {
			SkillID   string `json:"skill_id"`
			VersionID string `json:"version_id"`
			Name      string `json:"name"`
		} `json:"references"`
	}
	if err := json.Unmarshal(generationInputs, &got); err != nil {
		t.Fatalf("generation_inputs did not decode: %v (%s)", err, generationInputs)
	}
	if len(got.References) != 1 || got.References[0].SkillID != refSkillID ||
		got.References[0].VersionID != refVersionID || got.References[0].Name != "reference-source" {
		t.Errorf("generation_inputs.references = %+v, want one entry naming %s/%s", got.References, refSkillID, refVersionID)
	}

	skillID, _ := resp["skill_id"].(string)
	var viaAPI struct {
		Source *struct {
			GenerationInputs json.RawMessage `json:"generation_inputs"`
		} `json:"source"`
	}
	if code := getJSON(t, c.Client, c.base+"/api/skills/"+skillID, &viaAPI); code != http.StatusOK {
		t.Fatalf("GET /api/skills/%s: %d", skillID, code)
	}
	if viaAPI.Source == nil || len(viaAPI.Source.GenerationInputs) == 0 {
		t.Fatalf("source.generation_inputs is absent from the detail response: %+v", viaAPI)
	}
	var gotViaAPI struct {
		References []struct {
			SkillID   string `json:"skill_id"`
			VersionID string `json:"version_id"`
			Name      string `json:"name"`
		} `json:"references"`
	}
	if err := json.Unmarshal(viaAPI.Source.GenerationInputs, &gotViaAPI); err != nil {
		t.Fatalf("the detail response's generation_inputs did not decode: %v", err)
	}
	if len(gotViaAPI.References) != 1 || gotViaAPI.References[0].SkillID != refSkillID ||
		gotViaAPI.References[0].VersionID != refVersionID || gotViaAPI.References[0].Name != "reference-source" {
		t.Errorf("detail response generation_inputs.references = %+v, want one entry naming %s/%s",
			gotViaAPI.References, refSkillID, refVersionID)
	}
}

func TestAReferenceToAnotherUsersPrivateSkillIs422(t *testing.T) {
	pool := requireDB(t)
	stub := newGenerateStub(t)
	a := newAPIExposingGenerate(t, pool, stub.URL)
	owner := a.login(t, "gen-ref-owner")
	other := a.login(t, "gen-ref-other")

	privateSkillID, _ := importFiles(t, a, pool, owner, map[string]string{
		"SKILL.md": "---\nname: someone-elses-skill\ndescription: Not yours to read.\n---\n\nPrivate.\n",
	})

	body := `{"task_description":"照別人的 Skill 做一個。","reference_skill_ids":["` + privateSkillID + `"]}`
	code, resp := postJSON(t, other, "/skills/generate", body)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("got %d %v, want 422", code, resp)
	}
	msg, _ := resp["error"].(string)
	if strings.Contains(msg, "someone-elses-skill") || strings.Contains(msg, privateSkillID) {
		t.Errorf("the refusal named the private skill: %q", msg)
	}
	if stub.calls != 0 {
		t.Errorf("an unresolvable reference still paid for %d gateway call(s)", stub.calls)
	}
}

func TestABlockedRedistributionCatalogueSkillIs422(t *testing.T) {
	pool := requireDB(t)
	stub := newGenerateStub(t)
	a := newAPIExposingGenerate(t, pool, stub.URL)
	curator := a.login(t, "gen-ref-curator")
	markCatalog(t, pool, curator.workspaceID)
	blockedSkillID, _ := importFiles(t, a, pool, curator, map[string]string{
		"SKILL.md": "---\nname: blocked-catalogue-skill\ndescription: Under a redistribution hold.\n---\n\nContent.\n",
	})
	if _, err := pool.Exec(context.Background(),
		"UPDATE skills SET redistribution = 'blocked' WHERE id = $1", mustUUID(t, blockedSkillID),
	); err != nil {
		t.Fatal(err)
	}

	caller := a.login(t, "gen-ref-caller")
	body := `{"task_description":"照目錄裡的 Skill 做一個。","reference_skill_ids":["` + blockedSkillID + `"]}`
	code, resp := postJSON(t, caller, "/skills/generate", body)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("got %d %v, want 422", code, resp)
	}
	if stub.calls != 0 {
		t.Errorf("a blocked reference still paid for %d gateway call(s)", stub.calls)
	}
}

func TestATakenDownCatalogueSkillIs422(t *testing.T) {
	pool := requireDB(t)
	stub := newGenerateStub(t)
	a := newAPIExposingGenerate(t, pool, stub.URL)
	curator := a.login(t, "gen-ref-curator-takedown")
	markCatalog(t, pool, curator.workspaceID)
	takenDownSkillID, _ := importFiles(t, a, pool, curator, map[string]string{
		"SKILL.md": "---\nname: taken-down-catalogue-skill\ndescription: Withdrawn from the catalogue.\n---\n\nContent.\n",
	})
	if _, err := pool.Exec(context.Background(),
		"UPDATE skills SET takedown_at = now(), takedown_reason = 'test takedown' WHERE id = $1",
		mustUUID(t, takenDownSkillID),
	); err != nil {
		t.Fatal(err)
	}

	caller := a.login(t, "gen-ref-caller-takedown")
	body := `{"task_description":"照目錄裡的 Skill 做一個。","reference_skill_ids":["` + takenDownSkillID + `"]}`
	code, resp := postJSON(t, caller, "/skills/generate", body)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("got %d %v, want 422", code, resp)
	}

	msg, _ := resp["error"].(string)
	if strings.Contains(msg, takenDownSkillID) || strings.Contains(msg, "taken-down-catalogue-skill") {
		t.Errorf("the refusal named the skill: %q", msg)
	}
	if stub.calls != 0 {
		t.Errorf("a taken-down reference still paid for %d gateway call(s)", stub.calls)
	}
}

func TestAnAccessRestrictedCatalogueSkillIs422(t *testing.T) {
	pool := requireDB(t)
	stub := newGenerateStub(t)
	a := newAPIExposingGenerate(t, pool, stub.URL)
	curator := a.login(t, "gen-ref-curator-restricted")
	markCatalog(t, pool, curator.workspaceID)
	restrictedSkillID, _ := importFiles(t, a, pool, curator, map[string]string{
		"SKILL.md": "---\nname: restricted-catalogue-skill\ndescription: Under a licensing hold.\n---\n\nContent.\n",
	})
	if _, err := pool.Exec(context.Background(),
		"UPDATE skills SET access_restriction = 'license-review' WHERE id = $1", mustUUID(t, restrictedSkillID),
	); err != nil {
		t.Fatal(err)
	}

	caller := a.login(t, "gen-ref-caller-restricted")
	body := `{"task_description":"照目錄裡的 Skill 做一個。","reference_skill_ids":["` + restrictedSkillID + `"]}`
	code, resp := postJSON(t, caller, "/skills/generate", body)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("got %d %v, want 422", code, resp)
	}
	msg, _ := resp["error"].(string)
	if strings.Contains(msg, restrictedSkillID) || strings.Contains(msg, "restricted-catalogue-skill") {
		t.Errorf("the refusal named the skill: %q", msg)
	}
	if stub.calls != 0 {
		t.Errorf("an access-restricted reference still paid for %d gateway call(s)", stub.calls)
	}
}

func TestBadBase64DiagramDataIs400(t *testing.T) {
	pool := requireDB(t)
	stub := newGenerateStub(t)
	a := newAPIExposingGenerate(t, pool, stub.URL)
	c := a.login(t, "gen-bad-base64")

	code, resp := postJSON(t, c, "/skills/generate",
		`{"diagram":{"media_type":"image/png","data":"not valid base64!!"}}`)
	if code != http.StatusBadRequest {
		t.Fatalf("got %d %v, want 400", code, resp)
	}
	if stub.calls != 0 {
		t.Errorf("bad base64 still paid for %d gateway call(s)", stub.calls)
	}
}

func TestADisallowedDiagramMediaTypeIs400(t *testing.T) {
	pool := requireDB(t)
	stub := newGenerateStub(t)
	a := newAPIExposingGenerate(t, pool, stub.URL)
	c := a.login(t, "gen-svg-diagram")

	body := `{"diagram":{"media_type":"image/svg+xml","data":"` +
		base64.StdEncoding.EncodeToString([]byte("<svg/>")) + `"}}`
	code, resp := postJSON(t, c, "/skills/generate", body)
	if code != http.StatusBadRequest {
		t.Fatalf("got %d %v, want 400", code, resp)
	}
	if stub.calls != 0 {
		t.Errorf("a disallowed media type still paid for %d gateway call(s)", stub.calls)
	}
}

func TestARealGatewayGenerationRecordsWhatItActuallyCost(t *testing.T) {
	base := os.Getenv("SKILLHUB_E2E_LLM_URL")
	if base == "" {
		t.Skip("set SKILLHUB_E2E_LLM_URL to a running apps/llm pointed at a real gateway; this test spends money")
	}
	pool := requireDB(t)
	a := newAPIWithLLM(t, pool, base)
	c := a.login(t, "gen-real-gateway")

	res, err := a.versions.GenerateSkill(context.Background(), workspaceOf(t, pool, c),
		ingest.GenerateInput{TaskDescription: "我每個月要把廠商寄來的掃描單據整理成一份表格交出去。"})
	if err != nil {
		t.Fatalf("GenerateSkill against a real gateway: %v", err)
	}

	query := `SELECT (metadata->>'cost_usd')::float8 FROM audit_events
		WHERE action = 'skill.import' AND resource_id = $1`
	arg := any(res.Version.ID)
	if res.Report.Blocked {
		query = `SELECT (metadata->>'cost_usd')::float8 FROM audit_events
			WHERE action = 'skill.generate_failed' AND actor_user_id = $1
			ORDER BY occurred_at DESC LIMIT 1`
		arg = mustUUID(t, c.userID)
		t.Logf("the model's answer was blocked (%+v); the cost assertion below is the failure row",
			res.Report.Findings)
	}
	var cost float64
	if err := pool.QueryRow(context.Background(), query, arg).Scan(&cost); err != nil {
		t.Fatalf("a real, paid generation left no cost on its durable row: %v\n"+
			"this is the seam the stubbed tests above cannot see -- they supply the usage block "+
			"that apps/llm is supposed to build from the gateway's reply", err)
	}
	if cost <= 0 {
		t.Fatalf("cost_usd = %v; the gateway prices every call, so a zero here means the price was "+
			"dropped somewhere between LiteLLM and the audit row", cost)
	}
	t.Logf("real gateway generation cost US$%.6f, attempts=%d", cost, res.Attempts)
}
