package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// 05 R-36's checker: a deployment variable that does not say what it blocks
// fails CI.
//
// The example R-36 gives is the one that prompted it — DOWNLOAD_ARTIFACT_RETENTION
// existed in .env.example, was read by the code, and no place at all said it
// stops packaging. The capability table in apps/platform/cmd/api now says so,
// and this compares the two.
//
// # Why it runs the binary
//
// The table is Go (R-36: 能力→前提的宣告放 Go) and this is a separate module, so
// the choice was between parsing Go source and asking the program. Parsing would
// make the checker agree with the table only as long as nobody wrote the literal
// a different way; `--capabilities` prints the table the program actually holds,
// before it reads or dials anything.
//
// # The ledger below is stock, not an extension point
//
// Same shape as db/query-owners.yaml's `allow:` and the design system's 偏離帳:
// named, reasoned, and it may only get shorter. Every entry is a variable that
// was already in .env.example on 2026-09-01 and does not yet say what it blocks.
// A NEW variable has to go in the capability table — that is the whole ratchet.
var capabilityLedger = []struct {
	reason string
	vars   []string
}{
	{
		// These change HOW something behaves, never WHETHER it is there. A
		// capability table row for them would have an empty 「沒有它會怎樣」.
		reason: "不是能力的前提，是同一個能力的參數",
		vars: []string{
			"APP_URL", "COOKIE_INSECURE", "DEV_CORS_ORIGIN", "METRICS_ADDR",
			"OBJSTORE_SSL", "SKILLHUB_CLEAN_MODE", "SKILLHUB_MODEL_GATEWAY_ADMIN_URL",
			"SKILLHUB_RUN_MAX_BUDGET_USD", "SKILLHUB_RUN_TPM_LIMIT",
		},
	},
	{
		// envx's onUnlessOff idiom: unset = enforced. Nothing is gated by their
		// absence, so "what does it block" has no answer — turning one OFF is
		// the action somebody has to write down, and that is a different check.
		reason: "保護措施：未設定＝已啟用，所以它不擋任何能力（envx 的 onUnlessOff）",
		vars:   []string{"GENERATE_QUOTA", "RATE_LIMIT", "RUN_QUOTA"},
	},
	{
		// The API reads these only to disclose them on GET /policy/data-retention;
		// the process that deletes is cmd/maintenance, which refuses to start
		// without them. Two readers, two idioms, both already pinned by
		// cmd/api/main_test.go.
		reason: "由 cmd/maintenance 擁有（refuseUnlessSet），API 只是揭露它",
		vars: []string{
			"AUDIT_RETENTION", "FEEDBACK_RETENTION", "SKILL_DELETION_GRACE", "TRACE_RETENTION",
		},
	},
	{
		reason: "屬於別的行程或測試調具，不是這個 API 的部署變數",
		vars: []string{
			"LITELLM_API_KEY", "LITELLM_BASE_URL", "LITELLM_MASTER_KEY", "OPENAI_API_KEY",
			// The gate test harness and the QA-002 corpus: read by tools/, never
			// by cmd/api, and absent means those harnesses skip themselves.
			"QA002_CORPUS", "SKILLHUB_E2E_EGRESS_NETWORK", "SKILLHUB_E2E_GATEWAY_KEY",
			"SKILLHUB_E2E_GATEWAY_URL", "SKILLHUB_E2E_RUNTIME_IMAGE",
			"SKILLHUB_E2E_SANDBOX_TOKEN", "SKILLHUB_E2E_SANDBOX_URL",
			"SKILLHUB_SANDBOX_NETWORK", "SKILLHUB_SANDBOX_PROVIDERS", "SKILLHUB_SANDBOX_RUNTIME",
			"SKILLHUB_SANDBOX_TOKEN", "SKILLHUB_TEST_DATABASE_URL",
		},
	},
	{
		// Both ends live inside the launcher's process tree, so it mints them
		// (05 R-36 第一段, category ①). Nobody supplies these by hand.
		reason: "啟動器自己鑄造（R-36 第一段的第①類）",
		vars:   []string{"SKILLHUB_TRACE_INGEST_SECRET", "SKILLHUB_TRACE_INGEST_URL"},
	},
	{
		// ⛔ These four genuinely gate something and are not in the table yet.
		// They are the debt this ledger exists to make visible, not to excuse.
		reason: "⛔ 欠一列能力表：它們確實擋著東西，只是還沒有人寫下擋著什麼",
		vars: []string{
			"BETA_ALLOWLIST", "DEV_LOGIN", "GENERATE_SKILL_EXPOSED",
			"GITHUB_CLIENT_ID", "GITHUB_CLIENT_SECRET", "IMPORT_ALLOW_INSECURE",
			"IMPORT_EXTRA_HOSTS", "OAUTH_REDIRECT_URL",
			"OBJSTORE_ACCESS_KEY", "OBJSTORE_BUCKET", "OBJSTORE_ENDPOINT", "OBJSTORE_SECRET_KEY",
		},
	},
}

var envExampleVar = regexp.MustCompile(`(?m)^([A-Z][A-Z0-9_]*)=`)

func capabilityTableProblems(root string) []string {
	example := filepath.Join(root, ".env.example")
	data, err := os.ReadFile(example)
	if err != nil {
		return []string{fmt.Sprintf(".env.example: %v", err)}
	}
	declared, err := declaredCapabilityVars(root)
	if err != nil {
		return []string{fmt.Sprintf("capability table: %v", err)}
	}

	excused := map[string]string{}
	for _, bucket := range capabilityLedger {
		for _, v := range bucket.vars {
			excused[v] = bucket.reason
		}
	}

	var problems []string
	inExample := map[string]bool{}
	for _, m := range envExampleVar.FindAllStringSubmatch(string(data), -1) {
		name := m[1]
		inExample[name] = true
		if declared[name] || excused[name] != "" {
			continue
		}
		problems = append(problems, fmt.Sprintf(
			".env.example: %s does not say what it blocks. Add it to the capability table in "+
				"apps/platform/cmd/api/capabilities.go, or to capabilityLedger in this file with a reason "+
				"(05 R-36)", name))
	}
	// The other direction: a capability standing on a variable nobody documents
	// is a precondition an operator cannot discover.
	for name := range declared {
		if !inExample[name] {
			problems = append(problems, fmt.Sprintf(
				"capability table names %s, which .env.example does not document", name))
		}
	}
	// A ledger entry for a variable that no longer exists is a claim nobody can
	// check, and it makes the list look shorter than it is.
	for name, reason := range excused {
		if !inExample[name] {
			problems = append(problems, fmt.Sprintf(
				"capabilityLedger excuses %s (%q), which is no longer in .env.example — delete the row", name, reason))
		}
	}
	sort.Strings(problems)
	return problems
}

// declaredCapabilityVars asks the API binary for its table.
func declaredCapabilityVars(root string) (map[string]bool, error) {
	platform := filepath.Join(root, "apps", "platform")
	if _, err := os.Stat(filepath.Join(platform, "go.mod")); err != nil {
		return nil, fmt.Errorf("apps/platform is not here: %w", err)
	}
	cmd := exec.Command("go", "-C", platform, "run", "./cmd/api", "--capabilities")
	cmd.Env = append(os.Environ(), "GOFLAGS=")
	done := time.AfterFunc(3*time.Minute, func() { _ = cmd.Process.Kill() })
	defer done.Stop()
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go run ./cmd/api --capabilities: %w", err)
	}
	var rows []struct {
		ID    string   `json:"id"`
		Needs []string `json:"needs"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("--capabilities did not print the table: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("--capabilities printed an empty table")
	}
	declared := map[string]bool{}
	for _, r := range rows {
		if strings.TrimSpace(r.ID) == "" {
			return nil, fmt.Errorf("--capabilities printed a row with no id")
		}
		for _, n := range r.Needs {
			declared[n] = true
		}
	}
	return declared, nil
}
