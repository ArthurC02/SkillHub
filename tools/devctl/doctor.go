package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

type checkResult struct {
	name     string
	status   string
	detail   string
	required bool
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
		checkRequiredVersion("go", []string{"version"}, "go"+goVersion),
		checkRequiredVersion("node", []string{"--version"}, "v"+nodeVersion),
		checkRequiredVersion("uv", []string{"--version"}, "uv "+toolchain["uv"]),
		checkRequiredVersion("task", []string{"--version"}, toolchain["task"]),
		checkRequiredVersion("docker", []string{"version", "--format", "{{.Client.Version}}"}, ""),
		checkOptionalVersion("golangci-lint", []string{"--version"}, toolchain["golangci_lint"]),
	}
	results = append(results, checkDockerCompose())
	results = append(results, checkDockerDaemon())
	results = append(results, checkPython(pythonVersion))
	results = append(results, checkEnv(root))
	results = append(results, checkPgliteInstall(root, toolchain)...)

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

func checkRequiredVersion(name string, args []string, want string) checkResult {
	result := checkVersion(name, args, want, "FAIL")
	result.required = true
	return result
}

func checkOptionalVersion(name string, args []string, want string) checkResult {
	return checkVersion(name, args, want, "WARN")
}

func checkVersion(name string, args []string, want, statusWhenMissing string) checkResult {
	path, err := exec.LookPath(name)
	if err != nil {
		return checkResult{name: name, status: statusWhenMissing, detail: "not found on PATH"}
	}
	output, err := exec.Command(path, args...).CombinedOutput()
	got := strings.TrimSpace(string(output))
	if err != nil {
		return checkResult{name: name, status: "FAIL", detail: got}
	}
	if want != "" && !compatibleVersion(got, want) {
		return checkResult{name: name, status: "FAIL", detail: fmt.Sprintf("got %q; want %q", firstLine(got), want)}
	}
	return checkResult{name: name, status: "PASS", detail: firstLine(got)}
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

// Reads the version actually installed under node_modules rather than the
// range in package.json.
func checkPgliteInstall(root string, toolchain map[string]string) []checkResult {
	packages := []struct {
		checkName    string
		toolchainKey string
		nodeModule   string
	}{
		{"pglite", "pglite", "@electric-sql/pglite"},
		{"pglite-socket", "pglite_socket", "@electric-sql/pglite-socket"},
		{"pglite-pgvector", "pglite_pgvector", "@electric-sql/pglite-pgvector"},
	}

	results := make([]checkResult, 0, len(packages))
	for _, pkg := range packages {
		want := toolchain[pkg.toolchainKey]
		pkgJSON := filepath.Join(root, "tools", "pglite", "node_modules", filepath.FromSlash(pkg.nodeModule), "package.json")
		got, err := readNodePackageVersion(pkgJSON)
		switch {
		case err != nil:
			results = append(results, checkResult{
				name:     pkg.checkName,
				status:   "WARN",
				detail:   fmt.Sprintf("not installed under tools/pglite (run npm install there); toolchain.yaml pins %q", want),
				required: false,
			})
		case want == "":
			results = append(results, checkResult{
				name:     pkg.checkName,
				status:   "WARN",
				detail:   fmt.Sprintf("installed %s but tools/toolchain.yaml has no %s pin", got, pkg.toolchainKey),
				required: false,
			})
		case got != want:
			results = append(results, checkResult{
				name:     pkg.checkName,
				status:   "FAIL",
				detail:   fmt.Sprintf("installed %s; tools/toolchain.yaml pins %s", got, want),
				required: true,
			})
		default:
			results = append(results, checkResult{name: pkg.checkName, status: "PASS", detail: got, required: true})
		}
	}
	return results
}

func readNodePackageVersion(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var meta struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	if meta.Version == "" {
		return "", fmt.Errorf("%s has no version field", path)
	}
	return meta.Version, nil
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
