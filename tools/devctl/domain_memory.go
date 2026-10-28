package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var domainMemoryTool = func(root string, args ...string) (string, error) {
	command := exec.Command(domainMemoryExecutable(root, runtime.GOOS, runtime.GOARCH), args...)
	command.Dir = root
	output, err := command.CombinedOutput()
	return domainMemoryOutput(string(output), err), err
}

func domainMemoryExecutable(root, goos, goarch string) string {
	engine := filepath.Join(root, ".claude", "skills", "domain-memory", "engine")
	switch {
	case goos == "linux" && goarch == "amd64":
		return filepath.Join(engine, "bin", "linux-x86_64", "domain-memory")
	case goos == "darwin":
		return filepath.Join(engine, "bin", "macos-universal", "domain-memory")
	case goos == "windows" && goarch == "amd64":
		return filepath.Join(engine, "bin", "windows-x86_64", "domain-memory.exe")
	}
	return filepath.Join(engine, "target", "release", "domain-memory")
}

func domainMemoryOutput(output string, commandErr error) string {
	if commandErr != nil {
		for _, line := range strings.Split(output, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "ERROR: ") {
				return line
			}
		}
	}
	return strings.TrimSpace(output)
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
