// Pre-run permission summary and its confirmation (02:TEST-005 / 03:TEST-008,009),
// plus the acceptance-criteria suggestions of 03:TEST-002.
//
// They live in apiserver_test with the rest of the database-backed HTTP tests so
// they serve apiserver.NewRouter's real table rather than a copy — see
// authz_integration_test.go for the helpers (TestMain, migrate, login) they reuse.
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

	"github.com/ArthurC02/skillhub/apps/platform/internal/run"
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
	Hash          string   `json:"summary_hash"`
	Notes         []string `json:"notes"`
	EstimatedCost struct {
		Currency string  `json:"currency"`
		Low      float64 `json:"low"`
		Typical  float64 `json:"typical"`
		High     float64 `json:"high"`
		Basis    string  `json:"basis"`
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
	// newFixture seeds a clean package, so the scan ran and found nothing. The
	// `unavailable` answer — scanned nothing, claimed nothing — has its own test
	// below, together with the gate that refuses to run on it.
	if s.Scripts.Status != "none" {
		t.Errorf("script status for a clean package = %q, want none", s.Scripts.Status)
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
	// PDM-005 §5.3: the estimated cost, and §5.2a-6: as a range, because prompt
	// caching makes a first run and a repeat differ by roughly 8x.
	if view.EstimatedCost.Currency == "" || view.EstimatedCost.High <= view.EstimatedCost.Low {
		t.Errorf("estimated cost = %+v, want a currency and a non-degenerate range", view.EstimatedCost)
	}
	if view.EstimatedCost.Basis == "" {
		t.Error("the cost estimate does not say where its numbers came from")
	}
}

// The estimate is display material, not a permission. It sits beside the hash and
// not inside it, so recalibrating it against a bigger sample cannot silently
// revoke every confirmation a user has outstanding — the same rule that keeps the
// user prompt out (02:TEST-005).
//
// Asserted over the wire rather than over the Go type: the thing that must not
// change is the hash the client quotes back, and this reproduces it from the
// bytes the client actually received.
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

// --- SEC-002 gate B: the static scan condition (02:SEC-003) -------------------

// Fail-closed. A version whose package cannot be read has not been scanned, and
// SEC-002 says a check that cannot be performed counts as not passed. The summary
// says `unavailable` — never `none` (DISC-004 不得自行推定為通過) — and the run is
// refused rather than dispatched to fail inside a sandbox.
func TestRunIsRefusedWhenThePackageCannotBeScanned(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-gate-b-unscannable")
	// The package disappears from storage after the version was created.
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

// An error-level finding is blocking (SKILL-002's severity policy, applied at
// gate B rather than only at import). The refusal names the finding codes so the
// author can act on it, and nothing from inside the package — a message can quote
// package content and this string reaches an HTTP response.
func TestRunIsRefusedWhenTheStaticScanIsBlocking(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-gate-b-blocked")
	// No SKILL.md at all: skillpkg's `skill-md-missing`, an error-level finding.
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

// --- SEC-002 gate B: the workspace concurrency ceiling (PDM-005 §5.2) ---------

// Two in flight is the limit, and the third is refused before a row exists. The
// runs stay queued because nothing works them here, which is the point: what the
// limit bounds is what a workspace has outstanding, not what a provider is busy
// with.
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

	// Iron rule 3: the ceiling is per workspace, so another user is unaffected by
	// this one filling theirs.
	other := newFixture(t, a, pool, "bob-gate-b-concurrency")
	otherHash := other.confirmPermissions(t)
	if code, view := other.startWithHash(t, otherHash); code != http.StatusCreated {
		t.Fatalf("another workspace's run: got %d (%s), want 201", code, view.Error)
	}

	// Finishing one frees the slot. Retired straight through the state machine's
	// terminal transition, which is what a real run does.
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

// The summary is only worth confirming if it describes the policy the run is
// actually held to. This reads back runs.policy_snapshot — the frozen copy the
// scheduler matches providers against — and compares it with what the user was
// shown, so a second definition of the default policy on either side fails here
// rather than silently letting a stale screen keep passing the hash check.
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

// suggestionsOf reads the proposal texts out of a suggest response.
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

// 02:TEST-001 makes automatic suggestion 可選強化 and leaves the confirmation with
// the user, so the route offers and stores nothing. Writing the model's proposals
// into the draft and leaving the user to delete the unwanted ones is that rule
// backwards, and it is what this used to do.
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

	// The draft is exactly as it was. This is the whole point of the change.
	if _, after := alice.doJSON(t, http.MethodGet, "/test-cases/"+id, ""); len(criteriaOf(t, after)) != 0 {
		t.Fatalf("suggest wrote into the draft; it must store nothing: %v", after)
	}

	// Adopting one is the ordinary write — and it keeps the label EVAL-001 reads,
	// because the user picked the wording rather than writing it.
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

	// Asking again does not re-offer what the draft already holds.
	_, body = alice.doJSON(t, http.MethodPost, "/test-cases/"+id+"/criteria/suggest", "")
	if again := suggestionsOf(t, body); len(again) != 1 || again[0] != proposals[1] {
		t.Errorf("second suggest = %v, want only the criterion not yet adopted", again)
	}

	// The user owns it from here: editing the text makes the wording theirs.
	cid := list[0]["id"].(string)
	_, body = alice.doJSON(t, http.MethodPatch, "/test-cases/"+id+"/criteria/"+cid,
		`{"text":"每個月都有一列總額,且含幣別"}`)
	if edited := criteriaOf(t, body); edited[0]["source"] != "user" {
		t.Errorf("source after the user rewrote the text = %v, want \"user\"", edited[0]["source"])
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
