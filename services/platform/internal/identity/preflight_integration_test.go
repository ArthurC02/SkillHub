// Pre-run permission summary and its confirmation (02:TEST-005 / 03:TEST-008,009),
// plus the acceptance-criteria suggestions of 03:TEST-002.
//
// They live in identity_test with the rest of the database-backed HTTP tests so
// they serve apiserver.NewRouter's real table rather than a copy — see
// authz_integration_test.go for the helpers (TestMain, migrate, login) they reuse.
package identity_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/services/platform/internal/run"
)

// --- helpers -----------------------------------------------------------------

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
	Hash  string   `json:"summary_hash"`
	Notes []string `json:"notes"`
	Error string   `json:"error"`
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

// confirmPermissions is the happy path every run test goes through: read the
// summary, agree to it, keep the hash for the run request.
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

// --- TEST-008: the summary itself --------------------------------------------

// Every item 02:TEST-005 requires to be shown before a run: Dataset, Script,
// tools, MCP, network, Secrets, Provider and resource limits.
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
	// No package was seeded into the store, so the scan cannot run. It must say
	// so rather than reporting a clean package (DISC-004 不得自行推定為通過).
	if s.Scripts.Status != "unavailable" {
		t.Errorf("script status with no readable package = %q, want unavailable", s.Scripts.Status)
	}
	if len(s.Tools) == 0 {
		t.Error("no tool disclosure at all")
	}
	// MVP has no MCP. The field is present and empty — shown honestly, not omitted
	// so that the question looks unasked.
	if s.MCPServers == nil || len(s.MCPServers) != 0 {
		t.Errorf("mcp_servers = %v, want an empty list", s.MCPServers)
	}
	if s.Network.Mode != "default_deny" || len(s.Network.Allow) != 0 {
		t.Errorf("network = %+v, want default_deny with an empty allow list", s.Network)
	}
	// Iron rule 11: the names of what gets injected, and nothing that looks like a
	// value for them. A key is minted per attempt at dispatch and has no business
	// in a screen the user reads before the run exists.
	if len(s.InjectedSecrets) == 0 {
		t.Fatal("no secret disclosure at all")
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
	if strings.Contains(string(raw), "sk-") {
		t.Error("the summary body contains something shaped like an API key")
	}
	if s.Provider.Name != "unassigned" {
		t.Errorf("provider on a fleet-less deployment = %q, want unassigned", s.Provider.Name)
	}
	// The resource values shown are the ones the scheduler will actually enforce
	// (PDM-005 §5.2), not a second copy that can drift.
	if s.ResourceLimits != run.DefaultResourceLimits() {
		t.Errorf("resource limits shown = %+v, want DefaultResourceLimits", s.ResourceLimits)
	}
	if len(view.Notes) == 0 {
		t.Error("the summary carries no explanatory notes")
	}
	if view.Hash == "" {
		t.Error("the summary has no hash to confirm")
	}
}

// A package that does carry a script says so, from a scan of the exact stored
// bytes the run will execute.
func TestPreflightSummaryReportsScriptsInThePackage(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	// The API's run service reads packages here for the same reason cmd/api does.
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

// The same input hashes the same way, so a client can compare "what I confirmed"
// against "what is true now" without re-reading the whole body.
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

// --- TEST-009: the gate -------------------------------------------------------

// The whole flow: confirm what was shown, then the run starts.
func TestRunStartsOnlyAfterThePermissionSummaryIsConfirmed(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-gate-b")

	_, view := f.preflight(t)

	// Unconfirmed: the hash is right but nobody agreed to it. 422, and nothing is
	// created (SEC-002 gate B).
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

	// An empty hash is refused too — omitting the field is not a way past the gate.
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

// A hash the client made up is not a confirmation, and neither is one for another
// pairing: the platform rebuilds the summary and compares against that.
func TestRunRefusesAHashThatIsNotTheCurrentSummary(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-gate-b-forged")
	f.confirmPermissions(t)

	forged := strings.Repeat("a", 64)
	if code, _ := f.startWithHash(t, forged); code != http.StatusUnprocessableEntity {
		t.Errorf("run with an invented hash: got %d, want 422", code)
	}
	// And it cannot be confirmed either, so there is no way to make the row exist.
	if code, _ := f.confirm(t, forged); code != http.StatusUnprocessableEntity {
		t.Errorf("confirming an invented hash: got %d, want 422", code)
	}
}

// 02:TEST-005「權限有變更時必須重新確認,不得沿用不相符的舊確認」: adding a dataset
// after the confirmation changes what the run may read, so the old agreement stops
// working until the user has seen the new summary.
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

	// The new summary is a different hash, and confirming that one lets the run
	// through.
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
	// The stale hash stays refused afterwards: re-confirming the new summary is
	// not a blanket reset of the old agreement.
	if code, _ := f.startWithHash(t, old); code != http.StatusUnprocessableEntity {
		t.Errorf("stale hash after re-confirmation: got %d, want 422", code)
	}
}

// Removing a dataset is a permission change too — a summary is a statement about
// what the run touches, in both directions.
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

// WS-006 / iron rule 3: the summary is workspace scoped, so another user's ids
// are not a way to read what their run would be allowed to do.
func TestPreflightIsWorkspaceScoped(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := newFixture(t, a, pool, "alice-preflight-scope")
	bob := newFixture(t, a, pool, "bob-preflight-scope")

	// Bob asks about Alice's skill, version and test case, through his own session.
	stolen := fixture{client: bob.client, skillID: alice.skillID, versionID: alice.versionID, testCaseID: alice.testCaseID}
	if code, _ := stolen.preflight(t); code != http.StatusNotFound {
		t.Errorf("preflight on another workspace's run: got %d, want 404", code)
	}
	if code, _ := stolen.confirm(t, strings.Repeat("b", 64)); code != http.StatusNotFound {
		t.Errorf("confirming another workspace's summary: got %d, want 404", code)
	}

	// And Alice's own confirmation does not let Bob start her run.
	hash := alice.confirmPermissions(t)
	if code, _ := stolen.startWithHash(t, hash); code != http.StatusNotFound {
		t.Errorf("running another workspace's skill with her hash: got %d, want 404", code)
	}
}

// --- TEST-002: suggested acceptance criteria ---------------------------------

// suggestStub is the internal LLM service, standing in for the real one. It
// records the request body so the test can assert on what was sent.
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

func TestSuggestedCriteriaAreStoredAsSuggestedAndStayEditable(t *testing.T) {
	pool := requireDB(t)
	stub := newSuggestStub(t, `{"criteria":[{"text":"每個月都有一列總額"},{"text":"金額為數字"}]}`)
	a := newAPIWithLLM(t, pool, stub.URL)
	alice := a.login(t, "alice-suggest")
	_, id := newTestCase(t, pool, a, alice, "suggest")

	code, body := alice.doJSON(t, http.MethodPost, "/test-cases/"+id+"/criteria/suggest", "")
	if code != http.StatusOK {
		t.Fatalf("POST criteria/suggest: got %d, body %v", code, body)
	}
	list := criteriaOf(t, body)
	if len(list) != 2 {
		t.Fatalf("suggested criteria = %d, want 2: %v", len(list), list)
	}
	for _, c := range list {
		// A model's proposal is never presented as the user's own judgement, and
		// never arrives pre-agreed (02:TEST-001 自動建議為可選強化).
		if c["source"] != "suggested" {
			t.Errorf("suggested criterion has source %v, want \"suggested\"", c["source"])
		}
		if c["confirmed_at"] != nil {
			t.Errorf("a suggestion arrived already confirmed: %v", c)
		}
	}

	// The user owns them from here: editing the text makes the wording theirs.
	cid := list[0]["id"].(string)
	_, body = alice.doJSON(t, http.MethodPatch, "/test-cases/"+id+"/criteria/"+cid,
		`{"text":"每個月都有一列總額,且含幣別"}`)
	edited := criteriaOf(t, body)
	if edited[0]["source"] != "user" {
		t.Errorf("source after the user rewrote the text = %v, want \"user\"", edited[0]["source"])
	}

	// And deleting one works exactly as it does for a hand-written criterion.
	_, body = alice.doJSON(t, http.MethodDelete, "/test-cases/"+id+"/criteria/"+cid, "")
	if len(criteriaOf(t, body)) != 1 {
		t.Errorf("delete of a suggested criterion: %v", body)
	}
}

// Iron rule 11 / 02:TEST-002 資料使用範圍: the model is told the column names and
// an inferred type, and nothing from inside the rows.
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

// A deployment with no LLM service says so and leaves the manual path intact.
func TestSuggestionIsUnavailableWithoutTheLLMService(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool) // no LLM base URL
	alice := a.login(t, "alice-suggest-off")
	_, id := newTestCase(t, pool, a, alice, "suggest-off")

	code, body := alice.doJSON(t, http.MethodPost, "/test-cases/"+id+"/criteria/suggest", "")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("suggest with no LLM service: got %d, body %v", code, body)
	}
	// Manual creation still works, which is what makes the 503 tolerable.
	if code, _ := alice.doJSON(t, http.MethodPost, "/test-cases/"+id+"/criteria", `{"text":"written by hand"}`); code != http.StatusCreated {
		t.Errorf("manual criterion after a failed suggestion: got %d", code)
	}
}

// The LLM service answering 502 (its own provider failed) is not a 500 here: the
// user is told the optional feature is unavailable, not that something broke.
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
