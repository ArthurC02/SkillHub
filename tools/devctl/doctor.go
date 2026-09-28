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

const (
	statusPass = "PASS"
	statusFail = "FAIL"
	statusWarn = "WARN"

	versionFlag = "--version"

	checkNameDockerCompose = "docker compose"
	checkNameDockerDaemon  = "docker daemon"
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
		checkRequiredVersion("node", []string{versionFlag}, "v"+nodeVersion),
		checkRequiredVersion("uv", []string{versionFlag}, "uv "+toolchain["uv"]),
		checkRequiredVersion("task", []string{versionFlag}, toolchain["task"]),
		checkRequiredVersion("docker", []string{"version", "--format", "{{.Client.Version}}"}, ""),
		checkOptionalVersion("golangci-lint", []string{versionFlag}, toolchain["golangci_lint"]),
	}
	results = append(results, checkDockerCompose())
	results = append(results, checkDockerDaemon())
	results = append(results, checkPython(pythonVersion))
	results = append(results, checkEnv(root))
	results = append(results, checkPgliteInstall(root, toolchain)...)

	failed := false
	for _, result := range results {
		fmt.Fprintf(out, "%-5s %-18s %s\n", result.status, result.name, result.detail)
		if result.required && result.status == statusFail {
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
	result := checkVersion(name, args, want, statusFail)
	result.required = true
	return result
}

func checkOptionalVersion(name string, args []string, want string) checkResult {
	return checkVersion(name, args, want, statusWarn)
}

func checkVersion(name string, args []string, want, statusWhenMissing string) checkResult {
	path, err := exec.LookPath(name)
	if err != nil {
		return checkResult{name: name, status: statusWhenMissing, detail: "not found on PATH"}
	}
	output, err := exec.Command(path, args...).CombinedOutput()
	got := strings.TrimSpace(string(output))
	if err != nil {
		return checkResult{name: name, status: statusFail, detail: got}
	}
	if want != "" && !compatibleVersion(got, want) {
		return checkResult{name: name, status: statusFail, detail: fmt.Sprintf("got %q; want %q", firstLine(got), want)}
	}
	return checkResult{name: name, status: statusPass, detail: firstLine(got)}
}

func checkDockerCompose() checkResult {
	if _, err := exec.LookPath("docker"); err != nil {
		return checkResult{name: checkNameDockerCompose, status: statusFail, detail: "docker not found", required: true}
	}
	output, err := exec.Command("docker", "compose", "version").CombinedOutput()
	if err != nil {
		return checkResult{name: checkNameDockerCompose, status: statusFail, detail: strings.TrimSpace(string(output)), required: true}
	}
	return checkResult{name: checkNameDockerCompose, status: statusPass, detail: firstLine(strings.TrimSpace(string(output))), required: true}
}

func checkDockerDaemon() checkResult {
	if _, err := exec.LookPath("docker"); err != nil {
		return checkResult{name: checkNameDockerDaemon, status: statusFail, detail: "docker not found", required: true}
	}
	output, err := exec.Command("docker", "info", "--format", "{{.ServerVersion}}").CombinedOutput()
	if err != nil {
		return checkResult{name: checkNameDockerDaemon, status: statusFail, detail: firstLine(strings.TrimSpace(string(output))), required: true}
	}
	return checkResult{name: checkNameDockerDaemon, status: statusPass, detail: "server " + firstLine(strings.TrimSpace(string(output))), required: true}
}

func checkPython(want string) checkResult {
	if _, err := exec.LookPath("uv"); err != nil {
		return checkResult{name: ecosystemPython, status: statusFail, detail: "uv not found", required: true}
	}
	find := exec.Command("uv", ecosystemPython, "find", want)
	find.Env = append(os.Environ(), "UV_LINK_MODE=copy")
	pathOutput, err := find.CombinedOutput()
	if err != nil {
		return checkResult{name: ecosystemPython, status: statusFail, detail: strings.TrimSpace(string(pathOutput)), required: true}
	}
	pythonPath := strings.TrimSpace(string(pathOutput))
	output, err := exec.Command(pythonPath, versionFlag).CombinedOutput()
	got := strings.TrimSpace(string(output))
	if err != nil {
		return checkResult{name: ecosystemPython, status: statusFail, detail: got, required: true}
	}
	if !compatibleVersion(got, "Python "+want) {
		return checkResult{name: ecosystemPython, status: statusFail, detail: fmt.Sprintf("got %q; want Python %s", firstLine(got), want), required: true}
	}
	return checkResult{name: ecosystemPython, status: statusPass, detail: firstLine(got), required: true}
}

func checkEnv(root string) checkResult {
	if fileExists(filepath.Join(root, ".env")) {
		return checkResult{name: ".env", status: statusPass, detail: "present (values intentionally not inspected)", required: false}
	}
	return checkResult{name: ".env", status: statusWarn, detail: "missing; run task env:init", required: false}
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
				status:   statusWarn,
				detail:   fmt.Sprintf("not installed under tools/pglite (run npm install there); toolchain.yaml pins %q", want),
				required: false,
			})
		case want == "":
			results = append(results, checkResult{
				name:     pkg.checkName,
				status:   statusWarn,
				detail:   fmt.Sprintf("installed %s but tools/toolchain.yaml has no %s pin", got, pkg.toolchainKey),
				required: false,
			})
		case got != want:
			results = append(results, checkResult{
				name:     pkg.checkName,
				status:   statusFail,
				detail:   fmt.Sprintf("installed %s; tools/toolchain.yaml pins %s", got, want),
				required: true,
			})
		default:
			results = append(results, checkResult{name: pkg.checkName, status: statusPass, detail: got, required: true})
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
