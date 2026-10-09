package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const usage = `devctl keeps SkillHub's developer automation portable.

Usage:
  devctl doctor     check required tools and repository configuration
  devctl bootstrap  download project dependencies using native package managers
  devctl env-init   create .env from .env.example without overwriting it
  devctl profile-check model  verify a profile's required variables without printing values
  devctl gen [--check] [--scope=sql|openapi|all]  regenerate or check committed output
  devctl agent-sync [--check]  regenerate or check portable Agent artifacts from .claude
  devctl automation-check  verify Task, Agent docs and generated ownership markers
  devctl comment-lint [path-prefix...]  list comments that break AGENTS.md's comment rule
  devctl domain-memory-release  record the Domain Memory plugin's version and content digest (refuses a content change without a version bump)
  devctl test-report dir [go test args]  run the suite and report what skipped and why
  devctl seed-clean [--dry-run]  upload PORT-007's real, traceable demo skills into a clean-mode deployment
  devctl image-gate  runtime image source gates: digest-pinned base, upgrade record, image content matches its published version
  devctl preflight [--hook]  check unpushed commits for what CI would fail on (--hook reads git's pre-push input)
  devctl ci-status [ref] [--wait]  every workflow run for a commit; exit 0 green, 1 red, 3 pending, 4 no runs
  devctl dep-audit [--full]  fail on fixable vulnerabilities, disallowed licenses and workflow findings (--full: vulnerabilities in every project, dev dependencies too)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	root, err := findRepoRoot()
	if err != nil {
		fatal(err)
	}
	code, err := runCommand(root, os.Args[1], os.Args[2:])
	if err != nil {
		fatal(err)
	}
	if code != 0 {
		os.Exit(code)
	}
}

const (
	cmdDoctor    = "doctor"
	cmdBootstrap = "bootstrap"
	cmdPreflight = "preflight"
)

func runCommand(root, command string, args []string) (int, error) {
	switch command {
	case "help", "-h", "--help":
		fmt.Print(usage)
		return 0, nil
	case cmdDoctor, cmdBootstrap, "env-init", "profile-check":
		return runSetupCommand(root, command, args)
	case cmdGen, "agent-sync", "automation-check", "comment-lint", cmdDomainMemoryRelease:
		return runRepoCheckCommand(root, command, args)
	case "test-report", "seed-clean", "image-gate", cmdPreflight, "ci-status", "dep-audit":
		return runPipelineCommand(root, command, args)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", command, usage)
		return 2, nil
	}
}

func runSetupCommand(root, command string, args []string) (int, error) {
	switch command {
	case cmdDoctor:
		return 0, doctor(root, os.Stdout)
	case cmdBootstrap:
		return 0, bootstrap(root, os.Stdout)
	case "env-init":
		return 0, envInit(root, os.Stdout)
	default:
		if len(args) != 1 {
			return 0, errors.New("usage: devctl profile-check model")
		}
		return 0, profileCheck(root, args[0], os.Stdout)
	}
}

func runRepoCheckCommand(root, command string, args []string) (int, error) {
	switch command {
	case cmdGen:
		return 0, generate(root, args, os.Stdout)
	case "agent-sync":
		return 0, agentSync(root, args, os.Stdout)
	case "automation-check":
		return 0, automationCheck(root, os.Stdout)
	case cmdDomainMemoryRelease:
		return 0, recordDomainMemoryRelease(root, os.Stdout)
	default:
		return 0, commentLint(root, args, os.Stdout)
	}
}

func runPipelineCommand(root, command string, args []string) (int, error) {
	switch command {
	case "test-report":
		if len(args) < 1 {
			return 0, errors.New("usage: devctl test-report dir [go test args]")
		}
		return testReport(root, filepath.Join(root, args[0]), args[1:], os.Stdout)
	case "seed-clean":
		return 0, seedClean(root, args, os.Stdout)
	case "image-gate":
		return 0, imageGate(root, args, os.Stdout)
	case cmdPreflight:
		return 0, preflight(root, args, os.Stdin, os.Stdout)
	case "ci-status":
		return ciStatus(root, args, os.Stdout)
	default:
		return 0, depAudit(root, args, os.Stdout)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "devctl:", err)
	os.Exit(1)
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if fileExists(filepath.Join(dir, "Taskfile.yml")) && fileExists(filepath.Join(dir, agentsMdFile)) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("run devctl from inside the SkillHub repository")
		}
		dir = parent
	}
}

func bootstrap(root string, out io.Writer) error {
	steps := []struct {
		name string
		dir  string
		cmd  string
		args []string
		env  []string
	}{
		{name: "platform Go modules", dir: dirAppsPlatform, cmd: "go", args: []string{"mod", "download"}},
		{name: "sandbox Go modules", dir: dirAppsSandbox, cmd: "go", args: []string{"mod", "download"}},
		{name: "generated TypeScript client packages", dir: dirAPIClientTS, cmd: ecosystemNPM, args: []string{"ci"}},
		{name: "generated TypeScript client build", dir: dirAPIClientTS, cmd: ecosystemNPM, args: []string{cmdRun, cmdBuild}},
		{name: "web packages", dir: dirAppsWeb, cmd: ecosystemNPM, args: []string{"ci"}},
		{name: "LLM packages", dir: dirAppsLLM, cmd: "uv", args: []string{"sync", "--frozen"}, env: []string{"UV_LINK_MODE=copy"}},
		{name: "git hooks (pre-push runs devctl preflight --hook)", dir: ".", cmd: "git", args: []string{"config", "core.hooksPath", ".githooks"}},
	}
	for _, step := range steps {
		fmt.Fprintln(out, "==>", step.name)
		cmd := exec.Command(step.cmd, step.args...)
		cmd.Dir = filepath.Join(root, filepath.FromSlash(step.dir))
		cmd.Stdout, cmd.Stderr = out, out
		cmd.Env = append(os.Environ(), step.env...)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: %w", step.name, err)
		}
	}
	return nil
}

func envInit(root string, out io.Writer) error {
	target := filepath.Join(root, ".env")
	if fileExists(target) {
		fmt.Fprintln(out, ".env already exists; leaving it unchanged")
		return nil
	}
	source := filepath.Join(root, ".env.example")
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return err
	}
	fmt.Fprintln(out, "created .env from .env.example; secret fields remain blank")
	return nil
}

func profileCheck(root, profile string, out io.Writer) error {
	required := map[string][]string{
		"model": {"OPENAI_API_KEY", "LITELLM_MASTER_KEY"},
	}
	keys, ok := required[profile]
	if !ok {
		return fmt.Errorf("unknown profile %q", profile)
	}
	values, err := readDotEnv(filepath.Join(root, ".env"))
	if err != nil {
		return fmt.Errorf("read .env: %w; run env-init first", err)
	}
	var missing []string
	for _, key := range keys {
		if strings.TrimSpace(values[key]) == "" && strings.TrimSpace(os.Getenv(key)) == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("profile %s requires these variables: %s", profile, strings.Join(missing, ", "))
	}
	fmt.Fprintf(out, "%s profile prerequisites are present (values not shown)\n", profile)
	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
