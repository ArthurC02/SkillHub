package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type devContainerConfig struct {
	Build struct {
		Dockerfile string `json:"dockerfile"`
	} `json:"build"`
	RemoteUser        string            `json:"remoteUser"`
	WaitFor           string            `json:"waitFor"`
	PostCreateCommand string            `json:"postCreateCommand"`
	PostStartCommand  string            `json:"postStartCommand"`
	Mounts            []string          `json:"mounts"`
	ForwardPorts      []int             `json:"forwardPorts"`
	ContainerEnv      map[string]string `json:"containerEnv"`
	Customizations    struct {
		VSCode struct {
			Extensions []string       `json:"extensions"`
			Settings   map[string]any `json:"settings"`
		} `json:"vscode"`
	} `json:"customizations"`
}

func readDevContainerConfig(t *testing.T) devContainerConfig {
	t.Helper()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".devcontainer", "devcontainer.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg devContainerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse devcontainer.json: %v", err)
	}
	return cfg
}

func TestDevContainerBootstrapsTheWorkspaceAndDockerDaemon(t *testing.T) {
	cfg := readDevContainerConfig(t)
	if cfg.Build.Dockerfile != "../infra/images/devtools/Dockerfile" {
		t.Fatalf("build.dockerfile = %q", cfg.Build.Dockerfile)
	}
	if cfg.RemoteUser != "vscode" {
		t.Fatalf("remoteUser = %q", cfg.RemoteUser)
	}
	if cfg.WaitFor != "postCreateCommand" {
		t.Fatalf("waitFor = %q", cfg.WaitFor)
	}
	if cfg.PostCreateCommand != "bash .devcontainer/post-create.sh" {
		t.Fatalf("postCreateCommand = %q", cfg.PostCreateCommand)
	}
	if cfg.PostStartCommand != "bash .devcontainer/post-start.sh" {
		t.Fatalf("postStartCommand = %q", cfg.PostStartCommand)
	}
	if !slices.Contains(cfg.Mounts, "source=skillhub-devcontainer-docker,target=/var/lib/docker,type=volume") {
		t.Fatalf("devcontainer mount list lost the dedicated Docker volume: %v", cfg.Mounts)
	}
	if cfg.ContainerEnv["UV_LINK_MODE"] != "copy" {
		t.Fatalf("UV_LINK_MODE = %q", cfg.ContainerEnv["UV_LINK_MODE"])
	}
	for _, port := range []int{5173, 8080, 4000, 8333, 5432} {
		if !slices.Contains(cfg.ForwardPorts, port) {
			t.Fatalf("forwardPorts is missing %d: %v", port, cfg.ForwardPorts)
		}
	}

	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	postCreate, err := os.ReadFile(filepath.Join(root, ".devcontainer", "post-create.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(postCreate), "env-init") || !strings.Contains(string(postCreate), "bootstrap") {
		t.Fatalf("post-create.sh no longer initializes .env and dependencies:\n%s", postCreate)
	}
	postStart, err := os.ReadFile(filepath.Join(root, ".devcontainer", "post-start.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(postStart), "dockerd") || !strings.Contains(string(postStart), "docker info") {
		t.Fatalf("post-start.sh no longer starts and verifies the nested Docker daemon:\n%s", postStart)
	}
}

func TestDevContainerEditorDefaultsCoverTheRepoToolchain(t *testing.T) {
	cfg := readDevContainerConfig(t)
	requiredExtensions := []string{
		"golang.go",
		"ms-python.python",
		"ms-python.vscode-pylance",
		"dbaeumer.vscode-eslint",
		"esbenp.prettier-vscode",
		"ms-azuretools.vscode-docker",
		"redhat.vscode-yaml",
	}
	for _, extension := range requiredExtensions {
		if !slices.Contains(cfg.Customizations.VSCode.Extensions, extension) {
			t.Fatalf("missing VS Code extension %q: %v", extension, cfg.Customizations.VSCode.Extensions)
		}
	}

	settings := cfg.Customizations.VSCode.Settings
	if settings["files.eol"] != "\n" {
		t.Fatalf("files.eol = %#v", settings["files.eol"])
	}
	if settings["go.toolsManagement.checkForUpdates"] != "off" {
		t.Fatalf("go.toolsManagement.checkForUpdates = %#v", settings["go.toolsManagement.checkForUpdates"])
	}
	if settings["python.defaultInterpreterPath"] != "${workspaceFolder}/apps/llm/.venv/bin/python" {
		t.Fatalf("python.defaultInterpreterPath = %#v", settings["python.defaultInterpreterPath"])
	}
	if settings["python.terminal.activateEnvironment"] != true {
		t.Fatalf("python.terminal.activateEnvironment = %#v", settings["python.terminal.activateEnvironment"])
	}
	if settings["python.testing.pytestEnabled"] != true {
		t.Fatalf("python.testing.pytestEnabled = %#v", settings["python.testing.pytestEnabled"])
	}
	if args, ok := settings["python.testing.pytestArgs"].([]any); !ok || len(args) != 1 || args[0] != "apps/llm" {
		t.Fatalf("python.testing.pytestArgs = %#v", settings["python.testing.pytestArgs"])
	}
	if dirs, ok := settings["eslint.workingDirectories"].([]any); !ok || len(dirs) != 2 || dirs[0] != "apps/web" || dirs[1] != "packages/api-client-ts" {
		t.Fatalf("eslint.workingDirectories = %#v", settings["eslint.workingDirectories"])
	}
}

func TestDevtoolsImageInstallsUvFromPinnedGitHubReleaseAssets(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "infra", "images", "devtools", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	required := []string{
		"ARG UV_X86_64_UNKNOWN_LINUX_GNU_SHA256=",
		"ARG UV_AARCH64_UNKNOWN_LINUX_GNU_SHA256=",
		"https://github.com/astral-sh/uv/releases/download/${UV_VERSION}/uv-${uv_arch}-unknown-linux-gnu.tar.gz",
		`install -m 0755 "/tmp/uv-${uv_arch}-unknown-linux-gnu/uv" /usr/local/bin/uv`,
		`install -m 0755 "/tmp/uv-${uv_arch}-unknown-linux-gnu/uvx" /usr/local/bin/uvx`,
	}
	for _, want := range required {
		if !strings.Contains(source, want) {
			t.Fatalf("devtools Dockerfile is missing %q", want)
		}
	}
	if strings.Contains(source, "astral.sh/uv/${UV_VERSION}/install.sh") {
		t.Fatalf("devtools Dockerfile still depends on the legacy uv installer host:\n%s", source)
	}
}
