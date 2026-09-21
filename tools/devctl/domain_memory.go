package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var domainMemoryTool = func(root string, args ...string) (string, error) {
	command := exec.Command("uv", append([]string{"run", "--no-project", "python", filepath.Join(root, ".claude", "skills", "domain-memory", "scripts", "registry_tools.py")}, args...)...)
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

	checks := [][]string{
		{"validate", "--repo-root", root, "--registry-root", registryRoot, "--require-reviewed"},
		{"verify-sources", "--repo-root", root, "--source-map", filepath.Join(registryRoot, "source-map.json"), "--policy", filepath.Join(registryRoot, "domain-memory-policy.json")},
		{"verify-evidence", "--repo-root", root, "--registry-root", registryRoot},
		{"verify-audit", "--registry-root", registryRoot},
	}
	var problems []string
	for _, args := range checks {
		if output, err := domainMemoryTool(root, args...); err != nil {
			problems = append(problems, fmt.Sprintf("Domain Memory %s: %s", args[0], firstLine(output)))
		}
	}
	return problems
}
