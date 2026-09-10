package apiserver_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

type preflightView struct {
	Summary struct {
		SkillVersionID    string `json:"skill_version_id"`
		TestCaseID        string `json:"test_case_id"`
		DatasetTotalBytes int64  `json:"dataset_total_bytes"`
		Datasets          []struct {
			FileName  string `json:"file_name"`
			SizeBytes int64  `json:"size_bytes"`
		} `json:"datasets"`
		Scripts struct {
			Status   string   `json:"status"`
			Findings []string `json:"findings"`
		} `json:"scripts"`
		Tools      []string `json:"tools"`
		MCPServers []string `json:"mcp_servers"`
		Network    struct {
			Mode  string   `json:"mode"`
			Allow []string `json:"allow"`
		} `json:"network"`
		InjectedSecrets []string `json:"injected_secrets"`
		Provider        struct {
			Name string `json:"name"`
		} `json:"provider"`
		ResourceLimits run.ResourceLimits `json:"resource_limits"`
	} `json:"summary"`
	Hash          string   `json:"summary_hash"`
	Notes         []string `json:"notes"`
	EstimatedCost struct {
		LowCredits     int64  `json:"low_credits"`
		TypicalCredits int64  `json:"typical_credits"`
		HighCredits    int64  `json:"high_credits"`
		Basis          string `json:"basis"`
	} `json:"estimated_cost"`
	Error string `json:"error"`
}

func (f fixture) preflight(t *testing.T) (int, preflightView) {
	t.Helper()
	resp, err := f.Get(f.base + "/skills/" + f.skillID + "/runs/preflight" +
		"?version_id=" + f.versionID + "&test_case_id=" + f.testCaseID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out preflightView
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (f fixture) confirm(t *testing.T, hash string) (int, map[string]any) {
	t.Helper()
	return f.doJSON(t, http.MethodPost, "/skills/"+f.skillID+"/runs/preflight/confirm",
		fmt.Sprintf(`{"version_id":%q,"test_case_id":%q,"summary_hash":%q}`,
			f.versionID, f.testCaseID, hash))
}

func (f fixture) confirmPermissions(t *testing.T) string {
	t.Helper()
	code, view := f.preflight(t)
	if code != http.StatusOK {
		t.Fatalf("GET preflight: got %d (%s)", code, view.Error)
	}
	if code, body := f.confirm(t, view.Hash); code != http.StatusCreated {
		t.Fatalf("POST preflight/confirm: got %d, body %v", code, body)
	}
	return view.Hash
}

func (f fixture) startWithHash(t *testing.T, hash string) (int, runView) {
	t.Helper()
	return f.postJSON(t, "/skills/"+f.skillID+"/runs",
		`{"version_id":"`+f.versionID+`","test_case_id":"`+f.testCaseID+
			`","confirmed_summary_hash":"`+hash+`"}`)
}

func TestPreflightNamesTheInjectedSecretsAndNeverTheirValues(t *testing.T) {
	pool := requireDB(t)
	t.Setenv("SKILLHUB_MODEL_GATEWAY_URL", "http://gateway.invalid:4000")
	t.Setenv("SKILLHUB_MODEL_GATEWAY_KEY", "sk-not-a-real-key-for-this-test")
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-preflight-secrets")

	code, view := f.preflight(t)
	if code != http.StatusOK {
		t.Fatalf("GET preflight: got %d (%s)", code, view.Error)
	}
	s := view.Summary

	if len(s.Network.Allow) == 0 {
		t.Fatal("a configured gateway must appear in the egress allow list; without it this test proves nothing")
	}
	if len(s.InjectedSecrets) == 0 {
		t.Fatal("a gateway grant injects secrets, and the user has to be told which")
	}
	raw, _ := json.Marshal(view)
	for _, name := range s.InjectedSecrets {
		if name == "" {
			t.Error("an injected secret was disclosed with no name")
		}
		if strings.Contains(string(raw), name+"=") {
			t.Errorf("the summary carries a value for %s, not just its name", name)
		}
	}

	if strings.Contains(string(raw), "sk-not-a-real-key-for-this-test") {
		t.Error("the deployment's gateway key reached the pre-run summary")
	}
}

func TestPreflightSummaryDisclosesEveryRequiredItem(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-preflight-summary")

	if code, body := f.upload(t, "/test-cases/"+f.testCaseID+"/datasets", "rows.csv", csvBytes(512)); code != http.StatusCreated {
		t.Fatalf("seed upload: got %d, body %v", code, body)
	}

	code, view := f.preflight(t)
	if code != http.StatusOK {
		t.Fatalf("GET preflight: got %d (%s)", code, view.Error)
	}
	s := view.Summary

	if len(s.Datasets) != 1 || s.Datasets[0].FileName != "rows.csv" || s.Datasets[0].SizeBytes != 512 {
		t.Errorf("dataset disclosure = %+v", s.Datasets)
	}
	if s.DatasetTotalBytes != 512 {
		t.Errorf("dataset_total_bytes = %d, want 512", s.DatasetTotalBytes)
	}

	if s.Scripts.Status != "none" {
		t.Errorf("script status for a clean package = %q, want none", s.Scripts.Status)
	}
	if len(s.Tools) == 0 {
		t.Error("no tool disclosure at all")
	}

	if s.MCPServers == nil || len(s.MCPServers) != 0 {
		t.Errorf("mcp_servers = %v, want an empty list", s.MCPServers)
	}
	if s.Network.Mode != "default_deny" || len(s.Network.Allow) != 0 {
		t.Errorf("network = %+v, want default_deny with an empty allow list", s.Network)
	}

	if len(s.InjectedSecrets) != 0 {
		t.Errorf("no gateway grant, so nothing is injected, but the summary names %v", s.InjectedSecrets)
	}
	if s.InjectedSecrets == nil {
		t.Error("the row must still render as an empty list; an omitted one reads as a question never asked")
	}
	raw, _ := json.Marshal(view)
	if strings.Contains(string(raw), "sk-") {
		t.Error("the summary body contains something shaped like an API key")
	}
	if s.Provider.Name != "unassigned" {
		t.Errorf("provider on a fleet-less deployment = %q, want unassigned", s.Provider.Name)
	}

	if s.ResourceLimits != run.DefaultResourceLimits() {
		t.Errorf("resource limits shown = %+v, want DefaultResourceLimits", s.ResourceLimits)
	}
	if len(view.Notes) == 0 {
		t.Error("the summary carries no explanatory notes")
	}
	if view.Hash == "" {
		t.Error("the summary has no hash to confirm")
	}

	if view.EstimatedCost.LowCredits <= 0 || view.EstimatedCost.HighCredits <= view.EstimatedCost.LowCredits {
		t.Errorf("estimated cost = %+v, want a non-degenerate credit range", view.EstimatedCost)
	}
	if strings.Contains(view.EstimatedCost.Basis, "$") || strings.Contains(view.EstimatedCost.Basis, "美元") {
		t.Errorf("the basis still quotes money: %q", view.EstimatedCost.Basis)
	}
	if view.EstimatedCost.Basis == "" {
		t.Error("the cost estimate does not say where its numbers came from")
	}
}

func TestCostEstimateIsOutsideTheConfirmedHash(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-preflight-cost")

	resp, err := f.Get(f.base + "/skills/" + f.skillID + "/runs/preflight" +
		"?version_id=" + f.versionID + "&test_case_id=" + f.testCaseID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct {
		Summary json.RawMessage `json:"summary"`
		Hash    string          `json:"summary_hash"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body.Summary), "estimated_cost") ||
		strings.Contains(string(body.Summary), "\"basis\"") {
		t.Fatalf("the cost estimate leaked into the hashed body: %s", body.Summary)
	}
	sum := sha256.Sum256(body.Summary)
	if got := hex.EncodeToString(sum[:]); got != body.Hash {
		t.Fatalf("summary_hash %s is not sha256 over `summary` alone (%s)", body.Hash, got)
	}
}

func TestRunIsRefusedWhenThePackageCannotBeScanned(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-gate-b-unscannable")

	delete(a.packages, "packages/hash-alice-gate-b-unscannable.zip")

	_, view := f.preflight(t)
	if view.Summary.Scripts.Status != "unavailable" {
		t.Errorf("script status with no readable package = %q, want unavailable",
			view.Summary.Scripts.Status)
	}
	if code, body := f.confirm(t, view.Hash); code != http.StatusCreated {
		t.Fatalf("confirm: got %d, body %v", code, body)
	}
	code, refused := f.startWithHash(t, view.Hash)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("run on an unscannable package: got %d, want 422", code)
	}
	if !strings.Contains(refused.Error, "scan") {
		t.Errorf("refusal = %q, want it to name the scan", refused.Error)
	}
	if refused.RunID != "" {
		t.Error("a refused run still created a run row")
	}
}

func TestRunIsRefusedWhenTheStaticScanIsBlocking(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-gate-b-blocked")

	a.packages["packages/hash-alice-gate-b-blocked.zip"] = zipOf(t, map[string]string{
		"notes.txt": "not a skill package\n",
	})

	_, view := f.preflight(t)
	if code, body := f.confirm(t, view.Hash); code != http.StatusCreated {
		t.Fatalf("confirm: got %d, body %v", code, body)
	}
	code, refused := f.startWithHash(t, view.Hash)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("run on a blocking scan: got %d, want 422", code)
	}
	if !strings.Contains(refused.Error, "skill-md-missing") {
		t.Errorf("refusal = %q, want it to name the blocking finding code", refused.Error)
	}
}

func TestWorkspaceConcurrencyLimitBlocksTheThirdRun(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-gate-b-concurrency")

	hash := f.confirmPermissions(t)
	for i := 1; i <= run.MaxConcurrentRunsPerWorkspace; i++ {
		if code, view := f.startWithHash(t, hash); code != http.StatusCreated {
			t.Fatalf("run %d of %d: got %d (%s)", i, run.MaxConcurrentRunsPerWorkspace, code, view.Error)
		}
	}

	code, refused := f.startWithHash(t, hash)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("run past the limit: got %d, want 422", code)
	}
	if refused.RunID != "" {
		t.Error("a refused run still created a run row")
	}
	if !strings.Contains(refused.Error, "in progress") {
		t.Errorf("refusal = %q, want it to say why and what to do", refused.Error)
	}

	other := newFixture(t, a, pool, "bob-gate-b-concurrency")
	otherHash := other.confirmPermissions(t)
	if code, view := other.startWithHash(t, otherHash); code != http.StatusCreated {
		t.Fatalf("another workspace's run: got %d (%s), want 201", code, view.Error)
	}

	var id string
	if err := pool.QueryRow(context.Background(), `
		UPDATE runs SET status = 'failed', finished_at = now(), cleanup_status = 'cleaned'
		WHERE workspace_id = $1 AND status = 'queued'
		  AND id = (SELECT id FROM runs WHERE workspace_id = $1 AND status = 'queued' LIMIT 1)
		RETURNING id::text`, mustUUID(t, f.workspaceID)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if code, view := f.startWithHash(t, hash); code != http.StatusCreated {
		t.Fatalf("run after a slot freed up: got %d (%s)", code, view.Error)
	}
}

func TestPreflightSummaryReportsScriptsInThePackage(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	a.runs.Store = a.packages
	f := newFixture(t, a, pool, "alice-preflight-script")
	a.packages["packages/hash-alice-preflight-script.zip"] = demoPackage(t)

	_, view := f.preflight(t)
	if view.Summary.Scripts.Status != "present" {
		t.Fatalf("script status = %q, want present; findings %v",
			view.Summary.Scripts.Status, view.Summary.Scripts.Findings)
	}
	if len(view.Summary.Scripts.Findings) == 0 {
		t.Error("a package with a script disclosed no finding to look at")
	}
}

func TestPreflightShowsThePolicyTheRunIsActuallyHeldTo(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-preflight-policy")

	_, shown := f.preflight(t)
	created := f.start(t)

	var raw []byte
	if err := pool.QueryRow(context.Background(),
		"SELECT policy_snapshot FROM runs WHERE id = $1", mustUUID(t, created.RunID),
	).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var frozen struct {
		ResourceLimits run.ResourceLimits `json:"resource_limits"`
		Egress         struct {
			Mode  string           `json:"mode"`
			Allow []map[string]any `json:"allow"`
		} `json:"egress"`
	}
	if err := json.Unmarshal(raw, &frozen); err != nil {
		t.Fatal(err)
	}

	if shown.Summary.ResourceLimits != frozen.ResourceLimits {
		t.Errorf("the summary showed %+v but the run is held to %+v",
			shown.Summary.ResourceLimits, frozen.ResourceLimits)
	}
	if shown.Summary.Network.Mode != frozen.Egress.Mode {
		t.Errorf("the summary showed egress %q but the run is held to %q",
			shown.Summary.Network.Mode, frozen.Egress.Mode)
	}
	if len(shown.Summary.Network.Allow) != len(frozen.Egress.Allow) {
		t.Errorf("the summary showed %d permitted destinations but the run has %d",
			len(shown.Summary.Network.Allow), len(frozen.Egress.Allow))
	}
}

func TestPreflightHashIsStableOverIdenticalInput(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-preflight-stable")

	_, first := f.preflight(t)
	_, again := f.preflight(t)
	if first.Hash != again.Hash || first.Hash == "" {
		t.Fatalf("hash is not stable: %q vs %q", first.Hash, again.Hash)
	}
}

func TestRunStartsOnlyAfterThePermissionSummaryIsConfirmed(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-gate-b")

	_, view := f.preflight(t)

	code, run422 := f.startWithHash(t, view.Hash)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("run before confirming: got %d, want 422", code)
	}
	if run422.Error == "" {
		t.Error("the refusal carried no reason")
	}
	if run422.RunID != "" {
		t.Error("a refused run still created a run row")
	}

	if code, _ := f.startWithHash(t, ""); code != http.StatusUnprocessableEntity {
		t.Errorf("run with no confirmed hash: got %d, want 422", code)
	}

	if code, body := f.confirm(t, view.Hash); code != http.StatusCreated {
		t.Fatalf("confirm: got %d, body %v", code, body)
	}
	if code, created := f.startWithHash(t, view.Hash); code != http.StatusCreated {
		t.Fatalf("run after confirming: got %d (%s)", code, created.Error)
	}
}

func TestRunRefusesAHashThatIsNotTheCurrentSummary(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-gate-b-forged")
	f.confirmPermissions(t)

	forged := strings.Repeat("a", 64)
	if code, _ := f.startWithHash(t, forged); code != http.StatusUnprocessableEntity {
		t.Errorf("run with an invented hash: got %d, want 422", code)
	}

	if code, _ := f.confirm(t, forged); code != http.StatusUnprocessableEntity {
		t.Errorf("confirming an invented hash: got %d, want 422", code)
	}
}

func TestChangingTheDatasetInvalidatesAnEarlierConfirmation(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-gate-b-changed")

	old := f.confirmPermissions(t)

	if code, body := f.upload(t, "/test-cases/"+f.testCaseID+"/datasets", "extra.csv", csvBytes(256)); code != http.StatusCreated {
		t.Fatalf("upload: got %d, body %v", code, body)
	}

	code, refused := f.startWithHash(t, old)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("run on a stale confirmation: got %d, want 422", code)
	}
	if !strings.Contains(refused.Error, "confirm") {
		t.Errorf("refusal = %q, want it to say the summary needs confirming", refused.Error)
	}

	_, fresh := f.preflight(t)
	if fresh.Hash == old {
		t.Fatal("adding a dataset did not change the summary hash")
	}
	if code, body := f.confirm(t, fresh.Hash); code != http.StatusCreated {
		t.Fatalf("re-confirm: got %d, body %v", code, body)
	}
	if code, created := f.startWithHash(t, fresh.Hash); code != http.StatusCreated {
		t.Fatalf("run after re-confirming: got %d (%s)", code, created.Error)
	}

	if code, _ := f.startWithHash(t, old); code != http.StatusUnprocessableEntity {
		t.Errorf("stale hash after re-confirmation: got %d, want 422", code)
	}
}

func TestRemovingADatasetInvalidatesAnEarlierConfirmation(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-gate-b-removed")

	_, body := f.upload(t, "/test-cases/"+f.testCaseID+"/datasets", "rows.csv", csvBytes(256))
	datasetID, _ := body["dataset_id"].(string)
	old := f.confirmPermissions(t)

	if code, _ := f.doJSON(t, http.MethodDelete, "/test-cases/"+f.testCaseID+"/datasets/"+datasetID, ""); code != http.StatusOK {
		t.Fatalf("delete dataset: got %d", code)
	}
	if code, _ := f.startWithHash(t, old); code != http.StatusUnprocessableEntity {
		t.Errorf("run after removing a dataset: got %d, want 422", code)
	}
}

func TestPreflightIsWorkspaceScoped(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := newFixture(t, a, pool, "alice-preflight-scope")
	bob := newFixture(t, a, pool, "bob-preflight-scope")

	stolen := fixture{client: bob.client, skillID: alice.skillID, versionID: alice.versionID, testCaseID: alice.testCaseID}
	if code, _ := stolen.preflight(t); code != http.StatusNotFound {
		t.Errorf("preflight on another workspace's run: got %d, want 404", code)
	}
	if code, _ := stolen.confirm(t, strings.Repeat("b", 64)); code != http.StatusNotFound {
		t.Errorf("confirming another workspace's summary: got %d, want 404", code)
	}

	hash := alice.confirmPermissions(t)
	if code, _ := stolen.startWithHash(t, hash); code != http.StatusNotFound {
		t.Errorf("running another workspace's skill with her hash: got %d, want 404", code)
	}
}

type suggestStub struct {
	*httptest.Server
	lastBody string
}

func newSuggestStub(t *testing.T, response string) *suggestStub {
	t.Helper()
	stub := &suggestStub{}
	stub.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		stub.lastBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(stub.Close)
	return stub
}

func suggestionsOf(t *testing.T, body map[string]any) []string {
	t.Helper()
	raw, ok := body["suggestions"].([]any)
	if !ok {
		t.Fatalf("response has no suggestions: %v", body)
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("suggestion element is not an object: %v", item)
		}
		text, _ := m["text"].(string)
		if text == "" {
			t.Fatalf("suggestion has no text: %v", m)
		}
		out = append(out, text)
	}
	return out
}

func TestSuggestedCriteriaAreReturnedWithoutBeingStored(t *testing.T) {
	pool := requireDB(t)
	stub := newSuggestStub(t, `{"criteria":[{"text":"每個月都有一列總額"},{"text":"金額為數字"}]}`)
	a := newAPIWithLLM(t, pool, stub.URL)
	alice := a.login(t, "alice-suggest")
	_, id := newTestCase(t, pool, a, alice, "suggest")

	code, body := alice.doJSON(t, http.MethodPost, "/test-cases/"+id+"/criteria/suggest", "")
	if code != http.StatusOK {
		t.Fatalf("POST criteria/suggest: got %d, body %v", code, body)
	}
	proposals := suggestionsOf(t, body)
	if len(proposals) != 2 {
		t.Fatalf("suggestions = %d, want 2: %v", len(proposals), proposals)
	}

	if _, after := alice.doJSON(t, http.MethodGet, "/test-cases/"+id, ""); len(criteriaOf(t, after)) != 0 {
		t.Fatalf("suggest wrote into the draft; it must store nothing: %v", after)
	}

	code, body = alice.doJSON(t, http.MethodPost, "/test-cases/"+id+"/criteria",
		fmt.Sprintf(`{"text":%q,"source":"suggested"}`, proposals[0]))
	if code != http.StatusCreated {
		t.Fatalf("adopting a suggestion: got %d, body %v", code, body)
	}
	list := criteriaOf(t, body)
	if len(list) != 1 {
		t.Fatalf("criteria after adopting one = %d, want 1: %v", len(list), list)
	}
	if list[0]["source"] != "suggested" {
		t.Errorf("adopted suggestion has source %v, want \"suggested\"", list[0]["source"])
	}
	if list[0]["confirmed_at"] != nil {
		t.Errorf("an adopted suggestion arrived already confirmed: %v", list[0])
	}

	_, body = alice.doJSON(t, http.MethodPost, "/test-cases/"+id+"/criteria/suggest", "")
	if again := suggestionsOf(t, body); len(again) != 1 || again[0] != proposals[1] {
		t.Errorf("second suggest = %v, want only the criterion not yet adopted", again)
	}

	cid := list[0]["id"].(string)
	_, body = alice.doJSON(t, http.MethodPatch, "/test-cases/"+id+"/criteria/"+cid,
		`{"text":"每個月都有一列總額,且含幣別"}`)
	if edited := criteriaOf(t, body); edited[0]["source"] != "user" {
		t.Errorf("source after the user rewrote the text = %v, want \"user\"", edited[0]["source"])
	}
}

func TestSuggestionRequestCarriesDatasetFieldsButNoRows(t *testing.T) {
	pool := requireDB(t)
	stub := newSuggestStub(t, `{"criteria":[{"text":"ok"}]}`)
	a := newAPIWithLLM(t, pool, stub.URL)
	alice := a.login(t, "alice-suggest-privacy")
	_, id := newTestCase(t, pool, a, alice, "privacy")

	const secret = "ACME-INVOICE-4711"
	csv := "invoice_no,amount,paid\n" + secret + ",1999.50,true\n"
	if code, body := alice.upload(t, "/test-cases/"+id+"/datasets", "rows.csv", []byte(csv)); code != http.StatusCreated {
		t.Fatalf("upload: got %d, body %v", code, body)
	}
	if code, body := alice.doJSON(t, http.MethodPost, "/test-cases/"+id+"/criteria/suggest", ""); code != http.StatusOK {
		t.Fatalf("suggest: got %d, body %v", code, body)
	}

	sent := stub.lastBody
	for _, want := range []string{"invoice_no", "amount", "paid", "number", "boolean"} {
		if !strings.Contains(sent, want) {
			t.Errorf("the request did not describe %q; body was %s", want, sent)
		}
	}
	for _, forbidden := range []string{secret, "1999.50"} {
		if strings.Contains(sent, forbidden) {
			t.Fatalf("a dataset row value (%q) was sent to the model: %s", forbidden, sent)
		}
	}
}

func TestSuggestionIsUnavailableWithoutTheLLMService(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-suggest-off")
	_, id := newTestCase(t, pool, a, alice, "suggest-off")

	code, body := alice.doJSON(t, http.MethodPost, "/test-cases/"+id+"/criteria/suggest", "")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("suggest with no LLM service: got %d, body %v", code, body)
	}

	if code, _ := alice.doJSON(t, http.MethodPost, "/test-cases/"+id+"/criteria", `{"text":"written by hand"}`); code != http.StatusCreated {
		t.Errorf("manual criterion after a failed suggestion: got %d", code)
	}
}

func TestSuggestionSurvivesAnLLMServiceFailure(t *testing.T) {
	pool := requireDB(t)
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"detail":"provider error"}`, http.StatusBadGateway)
	}))
	t.Cleanup(down.Close)
	a := newAPIWithLLM(t, pool, down.URL)
	alice := a.login(t, "alice-suggest-502")
	_, id := newTestCase(t, pool, a, alice, "suggest-502")

	if code, body := alice.doJSON(t, http.MethodPost, "/test-cases/"+id+"/criteria/suggest", ""); code != http.StatusServiceUnavailable {
		t.Fatalf("suggest against a failing LLM service: got %d, body %v", code, body)
	}
}
