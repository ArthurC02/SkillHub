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

var capabilityLedger = []struct {
	reason string
	vars   []string
}{
	{

		reason: "不是能力的前提，是同一個能力的參數",
		vars: []string{
			"APP_URL", "COOKIE_INSECURE", "DEV_CORS_ORIGIN", "METRICS_ADDR",
			"OBJSTORE_SSL", "SKILLHUB_CLEAN_MODE", "SKILLHUB_MODEL_GATEWAY_ADMIN_URL",
			"SKILLHUB_RUN_MAX_BUDGET_USD", "SKILLHUB_RUN_TPM_LIMIT",

			"IMPORT_ALLOW_INSECURE", "IMPORT_EXTRA_HOSTS",
		},
	},
	{

		reason: "保護措施：未設定＝已啟用，所以它不擋任何能力（envx 的 onUnlessOff）",
		vars:   []string{"GENERATE_QUOTA", "RATE_LIMIT", "RUN_QUOTA"},
	},
	{

		reason: "由 cmd/maintenance 擁有（refuseUnlessSet），API 只是揭露它",
		vars: []string{
			"AUDIT_RETENTION", "FEEDBACK_RETENTION", "SKILL_DELETION_GRACE", "TRACE_RETENTION",
		},
	},
	{
		reason: "屬於別的行程或測試調具，不是這個 API 的部署變數",
		vars: []string{
			"LITELLM_API_KEY", "LITELLM_BASE_URL", "LITELLM_MASTER_KEY", "OPENAI_API_KEY",

			"QA002_CORPUS", "SKILLHUB_E2E_EGRESS_NETWORK", "SKILLHUB_E2E_GATEWAY_KEY",
			"SKILLHUB_E2E_GATEWAY_URL", "SKILLHUB_E2E_RUNTIME_IMAGE",
			"SKILLHUB_E2E_SANDBOX_TOKEN", "SKILLHUB_E2E_SANDBOX_URL",
			"SKILLHUB_SANDBOX_NETWORK", "SKILLHUB_SANDBOX_PROVIDERS", "SKILLHUB_SANDBOX_RUNTIME",
			"SKILLHUB_SANDBOX_TOKEN", "SKILLHUB_TEST_DATABASE_URL",
		},
	},
	{

		reason: "啟動器自己鑄造（R-36 第一段的第①類）",
		vars:   []string{"SKILLHUB_TRACE_INGEST_SECRET", "SKILLHUB_TRACE_INGEST_URL"},
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

	for name := range declared {
		if !inExample[name] {
			problems = append(problems, fmt.Sprintf(
				"capability table names %s, which .env.example does not document", name))
		}
	}

	for name, reason := range excused {
		if !inExample[name] {
			problems = append(problems, fmt.Sprintf(
				"capabilityLedger excuses %s (%q), which is no longer in .env.example — delete the row", name, reason))
		}
	}
	sort.Strings(problems)
	return problems
}

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
