package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var domainMemoryTool = func(root string, args ...string) (string, error) {
	pin, err := domainMemoryPin(root)
	if err != nil {
		return "", err
	}
	asset, err := domainMemoryAsset(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	if problem := installedDomainMemoryProblem(root, pin, asset); problem != "" {
		return "", fmt.Errorf("the Domain Memory plugin %s is not installed in %s (%s); run %s", pin.tag, domainMemoryInstallDir, problem, domainMemoryInstallCmd)
	}
	command := exec.Command(domainMemoryExecutable(root, runtime.GOOS), args...)
	command.Dir = root
	output, err := command.CombinedOutput()
	return domainMemoryOutput(string(output), err), err
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
	failures := make([]string, len(checks))
	var wg sync.WaitGroup
	for i, args := range checks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if output, err := domainMemoryTool(root, args...); err != nil {
				reason := firstLine(output)
				if reason == "" {
					reason = err.Error()
				}
				failures[i] = fmt.Sprintf("Domain Memory %s: %s", args[0], reason)
			}
		}()
	}
	wg.Wait()
	var problems []string
	for _, failure := range failures {
		if failure != "" {
			problems = append(problems, failure)
		}
	}
	return problems
}
