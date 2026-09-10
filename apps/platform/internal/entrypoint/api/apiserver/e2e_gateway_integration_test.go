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

	a.runs.Store = store
	a.runs.Providers = run.NewRegistry(run.NewProvider(
		"self_hosted", sandboxURL, os.Getenv("SKILLHUB_E2E_SANDBOX_TOKEN")))
	a.runs.Gateway = run.GatewayFromEnv()
	if a.runs.Gateway == nil {
		t.Fatal("SKILLHUB_MODEL_GATEWAY_URL / _KEY are required for this test")
	}
	a.runs.PollInterval = time.Second

	a.runs.MaxAttempts = 1

	// httptest binds 127.0.0.1, unreachable from the sandbox container that
	// pushes trace events back, so the route table is served again on an
	// address that is.
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

	if err := store.Put(ctx, "packages/hash-e2e-gateway.zip", e2eSkillPackage(t)); err != nil {
		t.Fatal(err)
	}

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
		t.Fatalf("run ended %s (%s / %s)", final.Status, final.FailureClass.Value, final.StatusReason)
	}

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

	if !strings.Contains(string(archive), "SKILLHUB-SCRIPT-RAN py3.") {
		t.Error("the archive carries no output from the package's own script: the skill's files were not executed (ADR-023 §2)")
	}

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
