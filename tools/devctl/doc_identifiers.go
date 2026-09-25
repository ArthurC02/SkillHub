package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var docIdentifierScope = []string{
	"AGENTS.md",
	"docs/plans/01-goals-and-plan.md",
	"docs/plans/02-specifications-and-acceptance-criteria.md",
	"docs/plans/03-work-items.md",
	"docs/plans/04-backlog-and-handoffs.md",
	"docs/plans/05-pending-rulings.md",
	"docs/design/system.md",
	"docs/design/information-architecture.md",
	"docs/plans/mvp/m5/README.md",
}

var docIdentifierTrees = []string{
	"docs/adr",
	"docs/development",
	"contracts",
}

var docIdentifierPattern = regexp.MustCompile(
	"`(Test[A-Za-z0-9_]{3,}|test_[a-z0-9_]{3,}|[A-Z][A-Za-z0-9]{4,}" +
		"|[a-z][a-z0-9]*(?:[A-Z][A-Za-z0-9]*)+|[a-z][a-z0-9]*(?:_[a-z0-9]+)+)`")

var allowedDocWords = map[string]string{
	"PurgeExpiredCostEvents":         "removed with the credit retention sweep (05 R-76); 03 CRED-006 records it as it was",
	"PurgeExpiredCreditEntries":      "removed with the same sweep",
	"PurgeUser":                      "removed with the account-deletion credit step; 03 CRED-006 and 04 record it as it was",
	"GVISOR":                         "a typo the dispatch gate refused when isolation was named after products; 03 PORT-010a records that fix as it was",
	"Superseded":                     "ADR status vocabulary (AGENTS.md), not a symbol",
	"Proposed":                       "ADR status vocabulary",
	"Accepted":                       "ADR status vocabulary",
	"FileCountLimit":                 "tail of an elided list: TestDatasetUploadEnforcesPerFileSizeLimit／FileCountLimit／TotalSizeLimit",
	"TotalSizeLimit":                 "tail of the same elided list",
	"Deallocate":                     "pgx / Postgres protocol message, not a SkillHub symbol",
	"MaxConnLifetime":                "pgxpool.Config field, not a SkillHub symbol",
	"QueryExecModeExec":              "pgx query exec mode, not a SkillHub symbol",
	"ReadyForQuery":                  "Postgres wire-protocol message",
	"NOTIFY":                         "Postgres command",
	"ModuleNotFoundError":            "Python builtin exception",
	"test_cases_skill_id_fkey":       "constraint name Postgres generates for the test_cases foreign key",
	"Querier":                        "sqlc interface that db/sqlc.yaml deliberately does not emit",
	"MARKER":                         "shell variable in tools/sec009 (.sh is outside codeExtensions)",
	"RunEvaluation":                  "renamed to features/runs/evaluation/EvaluationPanel.tsx; 04 records past work under the old name",
	"SnapshotInputsStillAvailable":   "replaced by trial/design's snapshotInputsAvailable over GetSnapshotInputs; 04 丙-9 records it as it was",
	"CountUnreadableRunArtifacts":    "replaced by trial/execution's evaluationArtifacts over ListRunArtifactsWithLifecycle; 03 records past work under the old name",
	"ResetCatalogueEnrichmentBefore": "replaced by skill/discovery's RequeueCatalogueEnrichment; 04 and 05 record the ruling under the old name",
	"TestResetCatalogueEnrichmentBeforeQueuesOnlyOlderPromptVersions": "renamed to TestRequeueingCatalogueEnrichmentQueuesOnlyOlderPromptVersions; 05 records the ruling under the old name",
	"RunRequested":            "workflow vocabulary from the run orchestration decision; domain-events.md maps it to the wire type run.queued",
	"RunStarted":              "the same vocabulary; the wire types are run.* and the commands are Go methods",
	"StartRun":                "the same vocabulary, on the command side",
	"RunExecutionCompleted":   "the same vocabulary; maps to run.succeeded|failed|timed_out",
	"CleanupCompleted":        "the same vocabulary; maps to run.cleanup_cleaned",
	"NULLIF":                  "SQL keyword",
	"PGDATA":                  "the Postgres image's environment variable",
	"GOTOOLCHAIN":             "the Go toolchain's environment variable",
	"XxxFacts":                "a naming pattern with a placeholder, not a type: <Collaborator>Facts",
	"activeProps":             "TanStack Router prop, not a SkillHub symbol",
	"beforeLoad":              "TanStack Router route option",
	"dangerouslySetInnerHTML": "React prop",
	"toHaveScreenshot":        "Playwright matcher",
	"agentType":               "an option of the host's workflow script API, not repository code",
	"InstructionsLoaded":      "the host's hook event, configured in a settings file this repository does not track",
	"firstRefusal":            "the name the convergence note gives a generic it argues against writing; it exists so the argument can name it",

	"pg_bigm":                "PostgreSQL extension",
	"pg_dumpall":             "PostgreSQL command",
	"uv_build":               "uv's PEP 517 build backend",
	"tool_use":               "a content block type of the model provider's API",
	"tool_result":            "the same API's other block type",
	"url_safe":               "another vendor's link-allowlist feature, named because it was bypassed",
	"agent_type":             "a field of the host's agent definitions, not repository code",
	"spawn_agent":            "the same host's tool name",
	"session_start":          "the same host's hook event",
	"load_reason":            "a field of the host's skill-loading report",
	"path_glob_match":        "the same report's other field",
	"nested_traversal":       "the same report's other field",
	"stopped_because":        "a field of the host's run summary",
	"project_doc_max_bytes":  "a setting of the host, not of this repository",
	"skillhub_default":       "the name a deployment gives one of its own profiles; nothing in the tree declares it",
	"first_exempted_at":      "the column name the image runbook gives its own CVE exemption table, which is a document and not a schema",
	"opted_in_at":            "a column the trial-evidence opt-in will need; its work item is unticked",
	"gateway_revoke_failed":  "a metric SBX-012 adds; that work item is unticked",
	"sandbox_destroy_failed": "the other metric of the same unticked work item",
	"plugin_component":       "the exclusion reason code the packaging decision names; INGEST-018 is unticked, so no code carries it yet",
}

var codeExtensions = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".py": true, ".sql": true, ".yaml": true, ".yml": true, ".json": true,
	".tmpl": true,
}

func docIdentifierFiles(root string) ([]string, []string) {
	files := append([]string(nil), docIdentifierScope...)
	var problems []string
	for _, tree := range docIdentifierTrees {
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(tree)), func(path string, entry os.DirEntry, err error) error {
			switch {
			case err != nil:
				return err
			case entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md"):
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(relative))
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			problems = append(problems, fmt.Sprintf("doc-identifier: %v", err))
		}
	}
	sort.Strings(files)
	return files, problems
}

func trackedFiles(root string) map[string]bool {
	out, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		return nil
	}
	tracked := map[string]bool{}
	for _, name := range strings.Split(string(out), "\x00") {
		if name != "" {
			tracked[name] = true
		}
	}
	if len(tracked) == 0 {
		return nil
	}
	return tracked
}

func docIdentifierProblems(root string) []string {
	declared := map[string]bool{}
	word := regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{3,}`)
	skip := map[string]bool{".git": true, "node_modules": true, ".venv": true, ".devctl": true, "dist": true, "__pycache__": true}
	tracked := trackedFiles(root)

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !codeExtensions[strings.ToLower(filepath.Ext(path))] {
			return nil
		}

		if filepath.Base(path) == "doc_identifiers.go" {
			return nil
		}
		if tracked != nil {
			relative, err := filepath.Rel(root, path)
			if err != nil || !tracked[filepath.ToSlash(relative)] {
				return nil
			}
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, w := range word.FindAllString(string(body), -1) {
			declared[w] = true
		}
		return nil
	})

	missing := map[string][]string{}
	scope, walkProblems := docIdentifierFiles(root)
	for _, rel := range scope {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		for _, m := range docIdentifierPattern.FindAllStringSubmatch(string(body), -1) {
			name := m[1]
			if declared[name] || allowedDocWords[name] != "" {
				continue
			}
			if !contains(missing[name], rel) {
				missing[name] = append(missing[name], rel)
			}
		}
	}

	names := make([]string, 0, len(missing))
	for n := range missing {
		names = append(names, n)
	}
	sort.Strings(names)

	problems := walkProblems
	for _, n := range names {
		problems = append(problems, fmt.Sprintf(
			"doc-identifier: %s is named in %s but declared in no file. "+
				"Correct the document, or add it to allowedDocWords with the reason it is prose.",
			n, strings.Join(missing[n], ", ")))
	}
	return problems
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
