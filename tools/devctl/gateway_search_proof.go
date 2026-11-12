package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type searchProof struct {
	root, dir, gateway, postgres, network string
	containers                            []string
	out                                   io.Writer
	networkCreated                        bool
	sourceHashes                          map[string]string
}

func gatewaySearchProof(root string, args []string, out io.Writer) (resultErr error) {
	if len(args) != 0 {
		return errors.New("usage: devctl gateway-search-proof")
	}
	parent := filepath.Join(root, ".devctl", "scratchpad")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(parent, "gateway-search-proof-")
	if err != nil {
		return err
	}
	p := &searchProof{root: root, dir: dir, out: out, network: "skillhub-search-proof-" + filepath.Base(dir)}
	fmt.Fprintln(out, "evidence:", dir)
	defer func() { resultErr = errors.Join(resultErr, p.clean()) }()
	if err := p.prepare(); err != nil {
		return err
	}
	return p.execute()
}

func (p *searchProof) command(label string, env []string, args ...string) ([]byte, error) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = p.root
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	data := stdout.Bytes()
	masked := regexp.MustCompile(`sk-[A-Za-z0-9_-]+`).ReplaceAll(append(bytes.Clone(data), stderr.Bytes()...), []byte("[MASKED]"))
	if writeErr := os.WriteFile(filepath.Join(p.dir, label+".log"), masked, 0o600); writeErr != nil {
		return nil, writeErr
	}
	if err != nil {
		return data, fmt.Errorf("%s: %w; see %s", label, err, p.dir)
	}
	return data, nil
}

func searchProofImage(root, name, repository string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		value := strings.TrimPrefix(strings.TrimSpace(line), "image: ")
		if strings.HasPrefix(value, repository+":") && strings.Contains(value, "@sha256:") {
			return value, nil
		}
	}
	return "", fmt.Errorf("no pinned %s image in %s", repository, name)
}

func (p *searchProof) prepare() error {
	if err := p.snapshotSources(); err != nil {
		return err
	}
	var err error
	p.gateway, err = searchProofImage(p.root, "infra/compose/gateway.yml", "ghcr.io/berriai/litellm")
	if err != nil {
		return err
	}
	p.postgres, err = searchProofImage(p.root, "infra/compose/docker-compose.yml", "pgvector/pgvector")
	if err != nil {
		return err
	}
	config, err := os.ReadFile(filepath.Join(p.root, "infra/compose/litellm-config.yaml"))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(p.dir, "production-config.yaml"), config, 0o600); err != nil {
		return err
	}
	convert := `import json,yaml;print(json.dumps(yaml.safe_load(open('/proof/production-config.yaml'))))`
	data, err := p.command("config-json", nil, "docker", "run", "--rm", "--network", "none", "--mount", p.mount(), "--entrypoint", "python", p.gateway, "-c", convert)
	if err != nil {
		return err
	}
	configJSON, coverage, err := searchProofConfig(data)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(p.dir, "gateway-config.yaml"), configJSON, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(p.dir, "native-search-coverage.json"), coverage, 0o600); err != nil {
		return err
	}
	buildEnv := []string{"GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0"}
	if _, err := p.command("provider-build", buildEnv, "go", "-C", "tools/devctl", "build", "-o", filepath.Join(p.dir, "search-proof-provider"), "./searchproof"); err != nil {
		return err
	}
	_, err = p.command("gateway-proof-build", buildEnv, "go", "-C", "apps/platform", "test", "-c", "-o", filepath.Join(p.dir, "gateway-search-proof.test"), "./internal/trial/execution")
	return err
}

func searchProofConfig(data []byte) ([]byte, []byte, error) {
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, nil, err
	}
	models, ok := config["model_list"].([]any)
	if !ok || len(models) == 0 {
		return nil, nil, errors.New("the gateway has no model routes to prove")
	}
	settings, _ := config["litellm_settings"].(map[string]any)
	interception, _ := settings["websearch_interception_params"].(map[string]any)
	providers, _ := interception["enabled_providers"].([]any)
	coverage := map[string][]string{"unguarded_models": {}, "unintercepted_providers": {}}
	for _, value := range models {
		model, _ := value.(map[string]any)
		params, _ := model["litellm_params"].(map[string]any)
		name, _ := model["model_name"].(string)
		providerModel, _ := params["model"].(string)
		if name == "" || providerModel == "" {
			return nil, nil, errors.New("a gateway route is missing its name or provider model")
		}
		if strings.Contains(providerModel, "text-embedding") {
			continue
		}
		drops, _ := params["additional_drop_params"].([]any)
		if !searchProofContains(drops, "web_search_options") {
			coverage["unguarded_models"] = append(coverage["unguarded_models"], name)
		}
		provider, _, _ := strings.Cut(providerModel, "/")
		if !searchProofContains(providers, provider) {
			coverage["unintercepted_providers"] = append(coverage["unintercepted_providers"], provider)
		}
	}
	first, _ := models[0].(map[string]any)
	params, ok := first["litellm_params"].(map[string]any)
	if !ok {
		return nil, nil, errors.New("the first model has no gateway parameters")
	}
	first["model_name"] = "trial-model"
	params["model"], params["api_key"], params["api_base"] = "openai/gpt-4o-mini", "local-model-only", "http://mock:8000/v1"
	config["model_list"] = models[:1]
	var tools []any
	for _, name := range []string{"skillhub-web-search", "other-search"} {
		tools = append(tools, map[string]any{"search_tool_name": name, "litellm_params": map[string]any{"search_provider": "tavily", "api_key": "local-search-only", "api_base": "http://mock:8000"}})
	}
	config["search_tools"] = tools
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	checked, err := json.MarshalIndent(coverage, "", "  ")
	return encoded, checked, err
}

func searchProofContains(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func (p *searchProof) mount() string {
	return "type=bind,src=" + filepath.ToSlash(p.dir) + ",dst=/proof,readonly"
}

func (p *searchProof) start(alias string, args ...string) (string, error) {
	name := p.network + "-" + alias
	base := []string{"docker", "run", "-d", "--network", p.network, "--network-alias", alias, "--name", name}
	_, err := p.command(alias+"-start", nil, append(base, args...)...)
	if err == nil {
		p.containers = append(p.containers, name)
	}
	return name, err
}

func (p *searchProof) wait(label string, timeout time.Duration, args ...string) error {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := p.command(label, nil, args...); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s did not become ready; see %s", label, p.dir)
		}
		time.Sleep(time.Second)
	}
}

func (p *searchProof) execute() error {
	if _, err := p.command("network-create", nil, "docker", "network", "create", "--internal", p.network); err != nil {
		return err
	}
	p.networkCreated = true
	pg, err := p.start("postgres", "-e", "POSTGRES_USER=proof", "-e", "POSTGRES_PASSWORD=proof", "-e", "POSTGRES_DB=litellm", p.postgres)
	if err != nil {
		return err
	}
	if err := p.wait("postgres-ready", time.Minute, "docker", "exec", pg, "pg_isready", "-U", "proof", "-d", "litellm"); err != nil {
		return err
	}
	mock, err := p.start("mock", "--mount", p.mount(), "--entrypoint", "sh", p.gateway, "-c", "cp /proof/search-proof-provider /tmp/search-proof-provider && chmod +x /tmp/search-proof-provider && exec /tmp/search-proof-provider")
	if err != nil {
		return err
	}
	if err := p.wait("mock-ready", time.Minute, "docker", "exec", mock, "/tmp/search-proof-provider", "--get", "http://127.0.0.1:8000/stats"); err != nil {
		return err
	}
	gateway, err := p.start("gateway", "--mount", p.mount(), "-e", "LITELLM_MASTER_KEY=sk-local-search-proof-master", "-e", "DATABASE_URL=postgresql://proof:proof@postgres:5432/litellm", "-e", "STORE_MODEL_IN_DB=False", "-e", "LITELLM_LOCAL_MODEL_COST_MAP=True", "-e", "DO_NOT_TRACK=True", "-e", "NO_PROXY=mock,postgres,localhost,127.0.0.1", p.gateway, "--config", "/proof/gateway-config.yaml", "--port", "4000")
	if err != nil {
		return err
	}
	if err := p.wait("gateway-ready", 3*time.Minute, "docker", "exec", mock, "/tmp/search-proof-provider", "--get", "http://gateway:4000/health/liveliness"); err != nil {
		return err
	}
	if _, err := p.command("gateway-proof-install", nil, "docker", "exec", gateway, "sh", "-c", "cp /proof/gateway-search-proof.test /tmp/gateway-search-proof.test && chmod +x /tmp/gateway-search-proof.test"); err != nil {
		return err
	}
	output, testErr := p.command("gateway-real-proof", nil, "docker", "exec", "-e", "SKILLHUB_TEST_SEARCH_GATEWAY_URL=http://127.0.0.1:4000", "-e", "SKILLHUB_REQUIRE_SEARCH_GATEWAY=1", gateway, "/tmp/gateway-search-proof.test", "-test.v", "-test.timeout=90s", "-test.run=^TestRealGatewaySearchUsesScopedKeysAndReportsGatewaySpend$")
	fmt.Fprint(p.out, regexp.MustCompile(`sk-[A-Za-z0-9_-]+`).ReplaceAllString(string(output), "[MASKED]"))
	stats, err := p.command("backend-stats", nil, "docker", "exec", mock, "/tmp/search-proof-provider", "--get", "http://127.0.0.1:8000/stats")
	if err != nil {
		return err
	}
	if err := p.checkStats(stats); err != nil {
		return errors.Join(testErr, err)
	}
	if testErr != nil {
		return testErr
	}
	return p.recordSuccess()
}

func (p *searchProof) checkStats(data []byte) error {
	var stats struct {
		Calls  []struct{ Query, Path string }
		Models []struct {
			Tools   []map[string]any
			Options any `json:"web_search_options"`
		} `json:"model_calls"`
	}
	if err := json.Unmarshal(data, &stats); err != nil {
		return err
	}
	if len(stats.Calls) != 1 || stats.Calls[0].Query != "local source proof" || stats.Calls[0].Path != "/search" {
		return fmt.Errorf("unexpected backend search calls: %+v", stats.Calls)
	}
	functionSeen := false
	for _, call := range stats.Models {
		if call.Options != nil {
			return errors.New("native search parameters reached the model provider")
		}
		for _, tool := range call.Tools {
			kind, _ := tool["type"].(string)
			if strings.HasPrefix(kind, "web_search") {
				return errors.New("native search parameters reached the model provider")
			}
			function, _ := tool["function"].(map[string]any)
			functionSeen = functionSeen || kind == "function" && function["name"] == "format_result"
		}
	}
	if !functionSeen {
		return errors.New("ordinary function tools did not reach the model provider")
	}
	coverage, err := os.ReadFile(filepath.Join(p.dir, "native-search-coverage.json"))
	if err != nil {
		return err
	}
	var missing map[string][]string
	if err := json.Unmarshal(coverage, &missing); err != nil {
		return err
	}
	for _, values := range missing {
		if len(values) != 0 {
			return fmt.Errorf("configured models lack native-search protection: %s", coverage)
		}
	}
	return nil
}

func (p *searchProof) snapshotSources() error {
	p.sourceHashes = make(map[string]string)
	for _, name := range []string{"apps/platform/internal/trial/execution/gateway.go", "apps/platform/internal/trial/execution/provider.go", "apps/platform/internal/trial/execution/gateway_search_integration_test.go", "apps/sandbox/internal/sandbox/contract.go", "contracts/openapi/sandbox-provider.yaml", "tools/devctl/main.go", "tools/devctl/gateway_search_proof.go", "tools/devctl/searchproof/main.go", "infra/compose/litellm-config.yaml"} {
		data, err := os.ReadFile(filepath.Join(p.root, name))
		if err != nil {
			return err
		}
		hash := sha256.Sum256(data)
		p.sourceHashes[name] = hex.EncodeToString(hash[:])
	}
	return nil
}

func (p *searchProof) recordSuccess() error {
	data, err := json.MarshalIndent(map[string]any{"test_exit": 0, "gateway": p.gateway, "postgres": p.postgres, "backend_calls": 1, "native_parameters_forwarded": 0, "function_tool_forwarded": true, "source_hashes": p.sourceHashes}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(p.dir, "gateway-real-proof-results.json"), data, 0o600); err != nil {
		return err
	}
	fmt.Fprintln(p.out, "gateway search proof: scoped access, budget, revocation, native interception and spend passed")
	return nil
}

func (p *searchProof) clean() (resultErr error) {
	for _, name := range []string{"search-proof-provider", "gateway-search-proof.test"} {
		if err := os.Remove(filepath.Join(p.dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, err)
		}
	}
	for i := len(p.containers) - 1; i >= 0; i-- {
		name := p.containers[i]
		_, _ = p.command(name+"-tail", nil, "docker", "logs", "--tail", "80", name)
		_, err := p.command(name+"-cleanup", nil, "docker", "rm", "-f", "-v", name)
		resultErr = errors.Join(resultErr, err)
	}
	if p.networkCreated {
		_, err := p.command("network-cleanup", nil, "docker", "network", "rm", p.network)
		resultErr = errors.Join(resultErr, err)
	}
	return resultErr
}
