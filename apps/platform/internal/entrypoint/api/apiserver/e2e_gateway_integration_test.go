// The one test with no fake on any leg (SBX-007, SBX-008, TRACE-004).
//
// A real skill package in real object storage, a real pre-run confirmation, a
// real dispatch to a real sandboxd, a real container running the Agent SDK, a
// real model call through the real LiteLLM gateway on a per-Run Virtual Key, and
// the trace, the artifacts and the key revocation read back afterwards.
//
// It costs money, so it is gated on SKILLHUB_E2E_SANDBOX_URL being set and skips
// otherwise — CI never runs it. The budget on the key it mints is the backstop:
// even a runaway agent cannot spend past ModelGatewayGrant.max_budget_usd.
//
// Running it: docs/plans/mvp/m2/README.md, the fourth-batch section.
package apiserver_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

// e2eSkillPackage is a real, spec-valid Agent Skill: frontmatter the validator
// accepts and an instruction concrete enough that a passing run proves the skill
// was loaded rather than that the model happened to answer. Small on purpose —
// one short turn on the mini tier (PDM-003 v5).
//
// Two entries, not one, and the second is a script under a subdirectory. That is
// ADR-023's fourth measured test (Skill 載入條件) given an executable form: until
// 2026-08-30 it was the one item on that list nobody could run without doing it
// by hand, so every runtime image upgrade recorded it as 人手跑. A single-file
// package also exercised none of what run.mjs's own ZIP reader does — nested
// paths, several central-directory entries, the per-entry and total size bounds
// added in 2026.08-5 — and that reader is what a new image most often changes.
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
		// Its output has to come from running it: the marker is built from the
		// interpreter's own version, which the file does not contain. A model
		// that read the script instead of executing it cannot produce this line.
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
	sandboxURL := os.Getenv("SKILLHUB_E2E_SANDBOX_URL")
	if sandboxURL == "" {
		t.Skip("SKILLHUB_E2E_SANDBOX_URL not set; skipping the paid end to end run")
	}
	pool := requireDB(t)
	ctx := context.Background()

	store, err := objstore.New(
		os.Getenv("OBJSTORE_ENDPOINT"), os.Getenv("OBJSTORE_ACCESS_KEY"),
		os.Getenv("OBJSTORE_SECRET_KEY"), objstoreBucket(), false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}

	a := newAPI(t, pool)
	// The real object store, so the grants minted at dispatch are real pre-signed
	// URLs against real bytes (SBX-008).
	a.runs.Store = store
	a.runs.Providers = run.NewRegistry(run.NewProvider(
		"self_hosted", sandboxURL, os.Getenv("SKILLHUB_E2E_SANDBOX_TOKEN")))
	a.runs.Gateway = run.GatewayFromEnv()
	if a.runs.Gateway == nil {
		t.Fatal("SKILLHUB_MODEL_GATEWAY_URL / _KEY are required for this test")
	}
	a.runs.PollInterval = time.Second
	// One attempt: a paid run that fails should fail once and say why, not spend
	// the budget three times over.
	a.runs.MaxAttempts = 1

	// The sandbox provider runs in another container and pushes trace events back
	// (TRACE-002). httptest binds 127.0.0.1, which is unreachable from there, so
	// the same route table is served a second time on an address that is not.
	public := httptest.NewUnstartedServer(a.handler)
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	public.Listener = listener
	public.Start()
	defer public.Close()
	a.runs.TraceSigner = a.traceSigner
	a.runs.TraceIngestBaseURL = fmt.Sprintf("http://%s:%d",
		os.Getenv("SKILLHUB_E2E_PUBLIC_HOST"), listener.Addr().(*net.TCPAddr).Port)

	f := newFixture(t, a, pool, "e2e-gateway")
	// The package the run will actually execute, under the key the seeded version
	// names. Both halves have to be the same bytes or the pre-run script scan and
	// the sandbox would be describing different packages.
	if err := store.Put(ctx, "packages/hash-e2e-gateway.zip", e2eSkillPackage(t)); err != nil {
		t.Fatal(err)
	}
	// A prompt that names the skill: PDM-011 measured the autonomous trigger rate
	// at 0, so a benchmark prompt states which skill to use (02:CONTENT-007).
	if _, err := pool.Exec(ctx,
		`UPDATE test_cases SET user_prompt = $2 WHERE id = $1`,
		mustUUID(t, f.testCaseID),
		"Use the run-marker skill to produce the run marker.",
	); err != nil {
		t.Fatal(err)
	}

	startWorkerWith(t, a.runs, a.evaluations)
	view := f.start(t)
	final := waitForTerminal(t, f.client, view.RunID, 6*time.Minute)
	if final.Status != "succeeded" {
		t.Fatalf("run ended %s (%s / %s)", final.Status, final.FailureClass, final.StatusReason)
	}

	// --- TRACE-004: the cost came from the gateway ---------------------------
	usage := traceUsageEvent(t, f.client, view.RunID)
	if usage["cost_source"] != "gateway" {
		t.Errorf("cost_source = %v, want gateway (a local estimate is not a cost)", usage["cost_source"])
	}
	cost, ok := usage["cost_usd"].(float64)
	if !ok || cost <= 0 {
		t.Errorf("cost_usd = %v, want a positive number reported by the gateway", usage["cost_usd"])
	}
	if tokens, ok := usage["input_tokens"].(float64); !ok || tokens <= 0 {
		t.Errorf("input_tokens = %v, want the SDK's own count", usage["input_tokens"])
	}
	t.Logf("gateway-reported cost for this run: $%.6f", cost)

	// --- SBX-008: the artifact was collected and uploaded --------------------
	attempts := final.Attempts
	if len(attempts) == 0 {
		t.Fatal("the run recorded no attempt")
	}
	key := fmt.Sprintf("run-artifacts/%s/%s/artifacts.tar", view.RunID, attempts[len(attempts)-1].RunAttemptID)
	archive, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("the artifact archive is not in object storage at %s: %v", key, err)
	}
	if !strings.Contains(string(archive), "SKILLHUB-E2E-OK") {
		t.Error("the uploaded archive does not carry the marker the skill was asked to write")
	}
	// ADR-023's fourth test: the script that shipped inside the untrusted package
	// was extracted to a path the agent could reach and actually executed by the
	// image's own interpreter. The version suffix is what makes this an execution
	// check rather than a transcription check, and it is matched loosely on
	// purpose — pinning the minor version here would fail the next base image
	// bump for a reason that has nothing to do with skill loading.
	if !strings.Contains(string(archive), "SKILLHUB-SCRIPT-RAN py3.") {
		t.Error("the archive carries no output from the package's own script: the skill's files were not executed (ADR-023 §2)")
	}

	// --- SEC-005: the key is gone -------------------------------------------
	// Cleanup revokes it in the same pass that destroys the sandbox, and records
	// a failure if it could not. `cleaned` is therefore the assertion.
	cleaned := waitForCleanupOutcome(t, f.client, view.RunID)
	if cleaned != "cleaned" {
		t.Errorf("cleanup_status = %q, want cleaned (which includes revoking the Virtual Key)", cleaned)
	}
}

func objstoreBucket() string {
	if v := os.Getenv("OBJSTORE_BUCKET"); v != "" {
		return v
	}
	return "skillhub"
}

// waitForTerminal is waitForStatus with a deadline a real model call can meet.
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

// waitForCleanupOutcome is waitForCleanup that reports a failed cleanup instead
// of failing the test on it: the assertion here is what the status became, and
// "failed" carries the reason the Virtual Key survived.
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

// traceUsageEvent reads the run's advanced trace back through the API and
// returns the run-total usage payload the harness produced.
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
