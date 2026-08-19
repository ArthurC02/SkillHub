package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
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
	devctl automation-check  verify Task, Agent docs and generated ownership markers
`

type checkResult struct {
	name     string
	status   string
	detail   string
	required bool
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	root, err := findRepoRoot()
	if err != nil {
		fatal(err)
	}

	switch os.Args[1] {
	case "doctor":
		if err := doctor(root, os.Stdout); err != nil {
			fatal(err)
		}
	case "bootstrap":
		if err := bootstrap(root, os.Stdout); err != nil {
			fatal(err)
		}
	case "env-init":
		if err := envInit(root, os.Stdout); err != nil {
			fatal(err)
		}
	case "profile-check":
		if len(os.Args) != 3 {
			fatal(errors.New("usage: devctl profile-check model"))
		}
		if err := profileCheck(root, os.Args[2], os.Stdout); err != nil {
			fatal(err)
		}
	case "gen":
		if err := generate(root, os.Args[2:], os.Stdout); err != nil {
			fatal(err)
		}
	case "automation-check":
		if err := automationCheck(root, os.Stdout); err != nil {
			fatal(err)
		}
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
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
		if fileExists(filepath.Join(dir, "Taskfile.yml")) && fileExists(filepath.Join(dir, "AGENTS.md")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("run devctl from inside the SkillHub repository")
		}
		dir = parent
	}
}

func doctor(root string, out io.Writer) error {
	toolchain, err := parseToolchain(filepath.Join(root, "tools", "toolchain.yaml"))
	if err != nil {
		return err
	}
	goVersion, err := readGoVersion(filepath.Join(root, "apps", "platform", "go.mod"))
	if err != nil {
		return err
	}
	nodeVersion, err := readTrimmed(filepath.Join(root, ".node-version"))
	if err != nil {
		return err
	}
	pythonVersion, err := readTrimmed(filepath.Join(root, "apps", "llm", ".python-version"))
	if err != nil {
		return err
	}

	results := []checkResult{
		checkVersion("go", []string{"version"}, "go"+goVersion, true),
		checkVersion("node", []string{"--version"}, "v"+nodeVersion, true),
		checkVersion("uv", []string{"--version"}, "uv "+toolchain["uv"], true),
		checkVersion("task", []string{"--version"}, toolchain["task"], true),
		checkVersion("docker", []string{"version", "--format", "{{.Client.Version}}"}, "", true),
		checkVersion("golangci-lint", []string{"--version"}, toolchain["golangci_lint"], false),
	}
	results = append(results, checkDockerCompose())
	results = append(results, checkDockerDaemon())
	results = append(results, checkPython(pythonVersion))
	results = append(results, checkEnv(root))

	failed := false
	for _, result := range results {
		fmt.Fprintf(out, "%-5s %-18s %s\n", result.status, result.name, result.detail)
		if result.required && result.status == "FAIL" {
			failed = true
		}
	}
	fmt.Fprintf(out, "\nplatform=%s/%s; tool versions: tools/toolchain.yaml\n", runtime.GOOS, runtime.GOARCH)
	if failed {
		return errors.New("required developer prerequisites are missing or incompatible; use the Dev Container or install the versions above")
	}
	return nil
}

func checkVersion(name string, args []string, want string, required bool) checkResult {
	path, err := exec.LookPath(name)
	if err != nil {
		status := "WARN"
		if required {
			status = "FAIL"
		}
		return checkResult{name: name, status: status, detail: "not found on PATH", required: required}
	}
	output, err := exec.Command(path, args...).CombinedOutput()
	got := strings.TrimSpace(string(output))
	if err != nil {
		return checkResult{name: name, status: "FAIL", detail: got, required: required}
	}
	if want != "" && !compatibleVersion(got, want) {
		return checkResult{name: name, status: "FAIL", detail: fmt.Sprintf("got %q; want %q", firstLine(got), want), required: required}
	}
	return checkResult{name: name, status: "PASS", detail: firstLine(got), required: required}
}

func checkDockerCompose() checkResult {
	if _, err := exec.LookPath("docker"); err != nil {
		return checkResult{name: "docker compose", status: "FAIL", detail: "docker not found", required: true}
	}
	output, err := exec.Command("docker", "compose", "version").CombinedOutput()
	if err != nil {
		return checkResult{name: "docker compose", status: "FAIL", detail: strings.TrimSpace(string(output)), required: true}
	}
	return checkResult{name: "docker compose", status: "PASS", detail: firstLine(strings.TrimSpace(string(output))), required: true}
}

func checkDockerDaemon() checkResult {
	if _, err := exec.LookPath("docker"); err != nil {
		return checkResult{name: "docker daemon", status: "FAIL", detail: "docker not found", required: true}
	}
	output, err := exec.Command("docker", "info", "--format", "{{.ServerVersion}}").CombinedOutput()
	if err != nil {
		return checkResult{name: "docker daemon", status: "FAIL", detail: firstLine(strings.TrimSpace(string(output))), required: true}
	}
	return checkResult{name: "docker daemon", status: "PASS", detail: "server " + firstLine(strings.TrimSpace(string(output))), required: true}
}

func checkPython(want string) checkResult {
	if _, err := exec.LookPath("uv"); err != nil {
		return checkResult{name: "python", status: "FAIL", detail: "uv not found", required: true}
	}
	find := exec.Command("uv", "python", "find", want)
	find.Env = append(os.Environ(), "UV_LINK_MODE=copy")
	pathOutput, err := find.CombinedOutput()
	if err != nil {
		return checkResult{name: "python", status: "FAIL", detail: strings.TrimSpace(string(pathOutput)), required: true}
	}
	pythonPath := strings.TrimSpace(string(pathOutput))
	output, err := exec.Command(pythonPath, "--version").CombinedOutput()
	got := strings.TrimSpace(string(output))
	if err != nil {
		return checkResult{name: "python", status: "FAIL", detail: got, required: true}
	}
	if !compatibleVersion(got, "Python "+want) {
		return checkResult{name: "python", status: "FAIL", detail: fmt.Sprintf("got %q; want Python %s", firstLine(got), want), required: true}
	}
	return checkResult{name: "python", status: "PASS", detail: firstLine(got), required: true}
}

func checkEnv(root string) checkResult {
	if fileExists(filepath.Join(root, ".env")) {
		return checkResult{name: ".env", status: "PASS", detail: "present (values intentionally not inspected)", required: false}
	}
	return checkResult{name: ".env", status: "WARN", detail: "missing; run task env:init", required: false}
}

func bootstrap(root string, out io.Writer) error {
	steps := []struct {
		name string
		dir  string
		cmd  string
		args []string
		env  []string
	}{
		{name: "platform Go modules", dir: "apps/platform", cmd: "go", args: []string{"mod", "download"}},
		{name: "sandbox Go modules", dir: "apps/sandbox", cmd: "go", args: []string{"mod", "download"}},
		{name: "generated TypeScript client packages", dir: "packages/api-client-ts", cmd: "npm", args: []string{"ci"}},
		{name: "generated TypeScript client build", dir: "packages/api-client-ts", cmd: "npm", args: []string{"run", "build"}},
		{name: "web packages", dir: "apps/web", cmd: "npm", args: []string{"ci"}},
		{name: "LLM packages", dir: "apps/llm", cmd: "uv", args: []string{"sync", "--frozen"}, env: []string{"UV_LINK_MODE=copy"}},
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

func readDotEnv(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return values, scanner.Err()
}

func parseToolchain(path string) (map[string]string, error) {
	return parseManifestSection(path, "tools")
}

func parseManifestSection(path, section string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	values := map[string]string{}
	inSection := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == section+":" {
			inSection = true
			continue
		}
		if !inSection || trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if len(line)-len(strings.TrimLeft(line, " ")) == 0 {
			inSection = false
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid toolchain entry %q", line)
		}
		values[parts[0]] = strings.Trim(strings.TrimSpace(parts[1]), `"'`)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("%s has no %s entries", path, section)
	}
	return values, nil
}

func readGoVersion(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == "go" {
			return fields[1], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("go directive not found in " + path)
}

func compatibleVersion(got, want string) bool {
	gotParts := numericVersion(got)
	wantParts := numericVersion(want)
	if len(gotParts) < 2 || len(wantParts) < 2 {
		return strings.Contains(got, want)
	}
	return gotParts[0] == wantParts[0] && gotParts[1] == wantParts[1]
}

var versionPattern = regexp.MustCompile(`\d+`)

func numericVersion(value string) []string {
	return versionPattern.FindAllString(value, -1)
}

func firstLine(value string) string {
	if before, _, ok := strings.Cut(value, "\n"); ok {
		return strings.TrimSpace(before)
	}
	return strings.TrimSpace(value)
}

func readTrimmed(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Sorted keys make future diagnostic output deterministic when tool-specific
// checks are added from the manifest.
func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
