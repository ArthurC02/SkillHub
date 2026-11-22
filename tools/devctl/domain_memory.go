package main

import (
	"bytes"
	"encoding/json"
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
				failures[i] = fmt.Sprintf("Domain Memory %s: %s", args[0], domainMemoryFailureReason(output, err))
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

const listedFailingCitations = 3

func domainMemoryFailureReason(output string, commandErr error) string {
	if start := strings.Index(output, "{"); start >= 0 {
		var raw json.RawMessage
		decoder := json.NewDecoder(strings.NewReader(output[start:]))
		if decoder.Decode(&raw) == nil {
			reason := jsonReportFailure(raw)
			if after := lastLine(output[start+int(decoder.InputOffset()):]); after != "" {
				reason += "; " + after
			}
			return reason
		}
	}
	if line := firstLine(output); line != "" {
		return line
	}
	return commandErr.Error()
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func jsonReportFailure(raw json.RawMessage) string {
	var report struct {
		Status  string `json:"status"`
		Results []struct {
			Asset      string `json:"asset"`
			ID         string `json:"id"`
			Path       string `json:"path"`
			Status     string `json:"status"`
			CitedLines struct {
				Start int `json:"start"`
				End   int `json:"end"`
			} `json:"cited_lines"`
		} `json:"results"`
	}
	if json.Unmarshal(raw, &report) != nil {
		return string(raw)
	}
	var parts []string
	failing := 0
	for _, result := range report.Results {
		if passingEvidence[result.Status] {
			continue
		}
		failing++
		if failing > listedFailingCitations {
			continue
		}
		location := result.Path
		if result.CitedLines.Start > 0 {
			location = fmt.Sprintf("%s:%d-%d", result.Path, result.CitedLines.Start, result.CitedLines.End)
		}
		parts = append(parts, fmt.Sprintf("%s/%s cites %s (%s)", result.Asset, result.ID, location, result.Status))
	}
	if failing > listedFailingCitations {
		parts = append(parts, fmt.Sprintf("and %d more", failing-listedFailingCitations))
	}
	if len(parts) == 0 && report.Status != "" {
		parts = append(parts, "status "+report.Status)
	}
	if len(parts) == 0 {
		var compact bytes.Buffer
		if json.Compact(&compact, raw) == nil {
			return compact.String()
		}
		return string(raw)
	}
	return strings.Join(parts, "; ")
}
