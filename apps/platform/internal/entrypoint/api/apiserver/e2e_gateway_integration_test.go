package apiserver_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func e2eSkillPackage(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"SKILL.md": "---\n" +
			"name: run-marker\n" +
			"description: Writes a verification marker file for a Skill Hub end to end run.\n" +
			"license: MIT\n" +
			"---\n\n" +
			"When asked for the run marker, do both of these and nothing else:\n\n" +
			"1. Run `python3 scripts/check.py` from this skill's own directory with the\n" +
			"   Bash tool, and write its entire standard output to\n" +
			"   `/out/artifacts/check.txt` using the Write tool.\n" +
			"2. Write the exact text `SKILLHUB-E2E-OK` to `/out/artifacts/marker.txt`\n" +
			"   using the Write tool.\n\n" +
			"Then reply with the single word DONE and nothing else.\n",

		"scripts/check.py": "import sys\n" +
			"print('SKILLHUB-SCRIPT-RAN py%d.%d' % sys.version_info[:2])\n",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestEndToEndRunCallsTheModelThroughItsOwnVirtualKey(t *testing.T) {
	archive := realGatewayRun(t, realGatewayRunSpec{
		user:   "e2e-gateway",
		pkg:    e2eSkillPackage(t),
		prompt: "Use the run-marker skill to produce the run marker.",
	})
	if !strings.Contains(archive, "SKILLHUB-E2E-OK") {
		t.Error("the uploaded archive does not carry the marker the skill was asked to write")
	}
	if !strings.Contains(archive, "SKILLHUB-SCRIPT-RAN py3.") {
		t.Error("the archive carries no output from the package's own script: the skill's files were not executed")
	}
}

type realGatewayRunSpec struct {
	user          string
	pkg           []byte
	storedInstead []byte
	sourcePath    string
	prompt        string
}

type realGatewayEnvironment struct {
	sandboxURL, gatewayURL, adminGatewayURL, model string
}

func realGatewayEnv(t *testing.T) realGatewayEnvironment {
	t.Helper()
	sandboxURL := os.Getenv("SKILLHUB_E2E_SANDBOX_URL")
	if sandboxURL == "" {
		t.Skip("SKILLHUB_E2E_SANDBOX_URL not set; skipping the paid end to end run")
	}
	gatewayURL := os.Getenv("SKILLHUB_E2E_GATEWAY_URL")
	if gatewayURL == "" {
		t.Fatal("SKILLHUB_E2E_GATEWAY_URL is required so the sandbox can reach the model gateway")
	}
	adminGatewayURL := os.Getenv("SKILLHUB_MODEL_GATEWAY_ADMIN_URL")
	if adminGatewayURL == "" {
		adminGatewayURL = os.Getenv("SKILLHUB_MODEL_GATEWAY_URL")
	}
	if adminGatewayURL == "" {
		t.Fatal("SKILLHUB_MODEL_GATEWAY_ADMIN_URL is required so the test can issue a virtual key")
	}
	model := os.Getenv("SKILLHUB_RUN_MODEL")
	if model == "" {
		t.Fatal("SKILLHUB_RUN_MODEL is required so the run uses the virtual key's allowed model")
	}
	return realGatewayEnvironment{sandboxURL: sandboxURL, gatewayURL: gatewayURL, adminGatewayURL: adminGatewayURL, model: model}
}

func configureRealGatewayRuns(t *testing.T, a *api, store *objstore.Client, env realGatewayEnvironment) {
	t.Helper()
	a.runs.Store = store
	a.runs.Providers = run.NewRegistry(run.NewProvider(
		"self_hosted", env.sandboxURL, os.Getenv("SKILLHUB_E2E_SANDBOX_TOKEN")))
	a.runs.Gateway = run.NewGateway(run.GatewayConfig{
		AdminBaseURL:   env.adminGatewayURL,
		AdminKey:       os.Getenv("SKILLHUB_MODEL_GATEWAY_KEY"),
		SandboxBaseURL: env.gatewayURL,
		Model:          env.model,
	})
	a.runs.Deployment.Model = env.model
	a.runs.Deployment.GatewayURL = env.gatewayURL
	if a.runs.Gateway == nil {
		t.Fatal("SKILLHUB_MODEL_GATEWAY_URL / _KEY are required for this test")
	}
	a.runs.PollInterval = time.Second

	a.runs.MaxAttempts = 1
}

type realGatewayRig struct {
	ctx   context.Context
	pool  *pgxpool.Pool
	store *objstore.Client
}

func (r realGatewayRig) stagePackage(t *testing.T, f *fixture, spec realGatewayRunSpec) {
	t.Helper()
	key, _ := skillpkg.PackageObjectKey(spec.pkg)
	stored := spec.pkg
	if spec.storedInstead != nil {
		stored = spec.storedInstead
	}
	if err := r.store.Put(r.ctx, key, stored); err != nil {
		t.Fatal(err)
	}

	version, err := gen.New(r.pool).CreateSkillVersion(r.ctx, gen.CreateSkillVersionParams{
		WorkspaceID:      mustUUID(t, f.workspaceID),
		SkillID:          mustUUID(t, f.skillID),
		VersionNumber:    nextVersionNumber(t, r.pool, f.skillID),
		ContentHash:      "hash-" + spec.user + "-at-" + spec.sourcePath,
		PackageObjectKey: key,
		SourcePath:       spec.sourcePath,
		Manifest:         []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	refreshListing(t, r.pool, f.skillID)
	f.versionID = uuidText(version.ID)

	if _, err := r.pool.Exec(r.ctx,
		`UPDATE test_cases SET user_prompt = $2 WHERE id = $1`,
		mustUUID(t, f.testCaseID), spec.prompt,
	); err != nil {
		t.Fatal(err)
	}
}

func gatewayUsageFindings(usage map[string]any) (float64, []string) {
	var problems []string
	if usage["cost_source"] != "gateway" {
		problems = append(problems, fmt.Sprintf("cost_source = %v, want gateway (a local estimate is not a cost)", usage["cost_source"]))
	}
	cost, ok := usage["cost_usd"].(float64)
	if !ok || cost <= 0 {
		problems = append(problems, fmt.Sprintf("cost_usd = %v, want a positive number reported by the gateway", usage["cost_usd"]))
	}
	if tokens, ok := usage["input_tokens"].(float64); !ok || tokens <= 0 {
		problems = append(problems, fmt.Sprintf("input_tokens = %v, want the SDK's own count", usage["input_tokens"]))
	}
	return cost, problems
}

type realGatewayOutcome struct {
	ctx    context.Context
	store  *objstore.Client
	client *client
	runID  string
	final  runView
}

func realGatewayRun(t *testing.T, spec realGatewayRunSpec) string {
	t.Helper()
	o := finishRealGatewayRun(t, spec)
	if o.final.Status != "succeeded" {
		t.Fatalf("run ended %s (%s / %s)", o.final.Status, o.final.FailureClass.Value, o.final.StatusReason)
	}

	usage := traceUsageEvent(t, o.client, o.runID)
	cost, problems := gatewayUsageFindings(usage)
	for _, problem := range problems {
		t.Error(problem)
	}
	t.Logf("gateway-reported cost for this run: $%.6f", cost)

	attempts := o.final.Attempts
	if len(attempts) == 0 {
		t.Fatal("the run recorded no attempt")
	}
	key := fmt.Sprintf("run-artifacts/%s/%s/artifacts.tar", o.runID, attempts[len(attempts)-1].RunAttemptID)
	archive, err := o.store.Get(o.ctx, key)
	if err != nil {
		t.Fatalf("the artifact archive is not in object storage at %s: %v", key, err)
	}
	cleaned := waitForCleanupOutcome(t, o.client, o.runID)
	if cleaned != "cleaned" {
		t.Errorf("cleanup_status = %q, want cleaned (which includes revoking the Virtual Key)", cleaned)
	}
	return string(archive)
}

func finishRealGatewayRun(t *testing.T, spec realGatewayRunSpec) realGatewayOutcome {
	t.Helper()
	env := realGatewayEnv(t)
	pool := requireDB(t)
	ctx := context.Background()

	store := ensuredObjectStore(t, ctx)

	a := newAPI(t, pool)

	configureRealGatewayRuns(t, a, store, env)

	// httptest binds 127.0.0.1, unreachable from the sandbox container that
	// pushes trace events back, so the route table is served again on an
	// address that is.
	public, port := startRoutableServer(t, a.handler)
	t.Cleanup(public.Close)
	a.runs.TraceSigner = a.traceSigner
	a.runs.TraceIngestBaseURL = fmt.Sprintf("http://%s:%d",
		os.Getenv("SKILLHUB_E2E_PUBLIC_HOST"), port)

	f := newFixture(t, a, pool, spec.user)

	realGatewayRig{ctx: ctx, pool: pool, store: store}.stagePackage(t, &f, spec)

	startWorkerWith(t, a.runs, a.evaluations)
	view := f.start(t)
	final := waitForTerminal(t, f.client, view.RunID, 6*time.Minute)
	return realGatewayOutcome{ctx: ctx, store: store, client: f.client, runID: view.RunID, final: final}
}

func objstoreBucket() string {
	if v := os.Getenv("OBJSTORE_BUCKET"); v != "" {
		return v
	}
	return "skillhub"
}

func waitForTerminal(t *testing.T, c *client, runID string, within time.Duration) runView {
	t.Helper()
	deadline := time.Now().Add(within)
	var last runView
	for time.Now().Before(deadline) {
		code, view := c.getRun(t, runID)
		if code != http.StatusOK {
			t.Fatalf("GET /runs/%s: got %d", runID, code)
		}
		last = view
		switch view.Status {
		case "succeeded", "failed", "cancelled", "timed_out":
			return view
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("run %s never finished; last status %q (%s)", runID, last.Status, last.StatusReason)
	return last
}

func waitForCleanupOutcome(t *testing.T, c *client, runID string) string {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		_, view := c.getRun(t, runID)
		last = view.CleanupStatus.Value
		if last == "cleaned" || last == "failed" {
			return last
		}
		time.Sleep(time.Second)
	}
	return last
}

func traceUsageEvent(t *testing.T, c *client, runID string) map[string]any {
	t.Helper()
	resp, err := c.Get(c.base + "/runs/" + runID + "/trace?mode=advanced")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct {
		Events []struct {
			Type    string         `json:"type"`
			Payload map[string]any `json:"payload"`
		} `json:"events"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	for _, e := range body.Events {
		if e.Type == "usage" {
			return e.Payload
		}
	}
	t.Fatal("the run produced no usage event; TRACE-004 has nothing to report")
	return nil
}

func e2ePluginPackage(t *testing.T) []byte {
	t.Helper()
	return zipOf(t, e2ePluginFiles())
}

func e2ePluginFiles() map[string]string {
	root := "skills/run-marker/"
	return map[string]string{
		"plugin.json": `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",` +
			`"name":"e2e-desk-tools","version":"1.0.0"}`,
		"mcp.json": `{"mcpServers":{}}`,
		root + "SKILL.md": "---\n" +
			"name: run-marker\n" +
			"description: Writes a verification marker file for a Skill Hub end to end run.\n" +
			"license: MIT\n" +
			"---\n\n" +
			"When asked for the run marker, do both of these and nothing else:\n\n" +
			"1. Run `python3 scripts/check.py` from this skill's own directory with the\n" +
			"   Bash tool, and write its entire standard output to\n" +
			"   `/out/artifacts/check.txt` using the Write tool.\n" +
			"2. Write the exact text `SKILLHUB-E2E-OK` to `/out/artifacts/marker.txt`\n" +
			"   using the Write tool.\n\n" +
			"Then reply with the single word DONE and nothing else.\n",
		root + "scripts/check.py": "import os, sys\n" +
			"print('SKILLHUB-SCRIPT-RAN py%d.%d' % sys.version_info[:2])\n" +
			"here = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))\n" +
			"print('SKILL-FILES=' + ','.join(sorted(os.listdir(here))))\n" +
			"print('SKILLS-INSTALLED=' + ','.join(sorted(os.listdir(os.path.dirname(here)))))\n",
		"skills/split-csv/SKILL.md": "---\nname: split-csv\ndescription: A sibling skill of the same plugin.\n---\n\nProse.\n",
	}
}

func TestEndToEndRunOfASkillInsideAPluginInstallsThatDirectoryAlone(t *testing.T) {
	archive := realGatewayRun(t, realGatewayRunSpec{
		user:       "e2e-plugin",
		pkg:        e2ePluginPackage(t),
		sourcePath: "skills/run-marker",
		prompt:     "Use the run-marker skill to produce the run marker.",
	})
	assertOnlyThePluginSkillRan(t, archive)
}

func TestEndToEndRunOfAPluginInsideARepositoryDirectoryInstallsItsSkill(t *testing.T) {
	wrapped := map[string]string{}
	for name, body := range e2ePluginFiles() {
		wrapped["desk-tools-main/"+name] = body
	}
	archive := realGatewayRun(t, realGatewayRunSpec{
		user:       "e2e-wrapped-plugin",
		pkg:        zipOf(t, wrapped),
		sourcePath: "skills/run-marker",
		prompt:     "Use the run-marker skill to produce the run marker.",
	})
	assertOnlyThePluginSkillRan(t, archive)
}

func TestEndToEndRunRefusesAPackageWhoseStoredBytesAreNotTheAdmittedOnes(t *testing.T) {
	o := finishRealGatewayRun(t, realGatewayRunSpec{
		user: "e2e-swapped-package",
		pkg:  e2eSkillPackage(t),
		storedInstead: zipOf(t, map[string]string{
			"SKILL.md": "---\nname: run-marker\ndescription: Not the package that was admitted.\nlicense: MIT\n---\n\nReply DONE.\n",
		}),
		prompt: "Use the run-marker skill to produce the run marker.",
	})
	if o.final.Status != "failed" || o.final.FailureClass.Value != "provider_error" {
		t.Fatalf("run ended %s (%s / %s), want failed / provider_error before the workload starts", o.final.Status, o.final.FailureClass.Value, o.final.StatusReason)
	}
}

func assertOnlyThePluginSkillRan(t *testing.T, archive string) {
	t.Helper()
	if !strings.Contains(archive, "SKILLHUB-E2E-OK") {
		t.Error("the plugin's skill produced no marker: the Agent found no skill to activate")
	}
	if got := lineStartingWith(archive, "SKILL-FILES="); got != "SKILL-FILES=SKILL.md,scripts" {
		t.Errorf("the installed skill carries more than the plugin's own subdirectory: %q", got)
	}
	if got := lineStartingWith(archive, "SKILLS-INSTALLED="); got != "SKILLS-INSTALLED=run-marker" {
		t.Errorf("the sandbox holds a skill the run did not ask for: %q", got)
	}
}

func lineStartingWith(archive, prefix string) string {
	for _, line := range strings.Split(archive, "\n") {
		if i := strings.Index(line, prefix); i >= 0 {
			return strings.TrimRight(line[i:], "\x00\r")
		}
	}
	return "(absent)"
}
