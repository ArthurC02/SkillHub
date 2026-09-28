package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var domainMemoryTool = func(root string, args ...string) (string, error) {
	command := exec.Command("uv", append([]string{cmdRun, "--no-project", ecosystemPython, filepath.Join(root, ".claude", "skills", "domain-memory", "scripts", "registry_tools.py")}, args...)...)
	command.Dir = root
	output, err := command.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

func domainMemoryProblems(root string) []string {
	registryRoot := filepath.Join(root, "docs", "domain-memory")
	if _, err := os.Stat(registryRoot); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return []string{fmt.Sprintf("Domain Memory: inspect registry: %v", err)}
	}

	const (
		flagRepoRoot     = "--repo-root"
		flagRegistryRoot = "--registry-root"
	)
	checks := [][]string{
		{"validate", flagRepoRoot, root, flagRegistryRoot, registryRoot},
		{"verify-sources", flagRepoRoot, root, "--source-map", filepath.Join(registryRoot, "source-map.json"), "--policy", filepath.Join(registryRoot, "domain-memory-policy.json")},
		{"verify-evidence", flagRepoRoot, root, flagRegistryRoot, registryRoot},
		{"verify-audit", flagRegistryRoot, registryRoot},
	}
	var problems []string
	for _, args := range checks {
		if output, err := domainMemoryTool(root, args...); err != nil {
			reason := firstLine(output)
			if reason == "" {
				reason = err.Error()
			}
			problems = append(problems, fmt.Sprintf("Domain Memory %s: %s", args[0], reason))
		}
	}
	return problems
}
