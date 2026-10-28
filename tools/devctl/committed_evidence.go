package main

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	domainMemoryRoot  = "docs/domain-memory"
	registryDirectory = domainMemoryRoot + "/registry"
	sourceMapFile     = domainMemoryRoot + "/source-map.json"
)

var passingEvidence = map[string]bool{"current": true, "shifted": true}

type evidenceReport struct {
	Results *[]struct {
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

func committedEvidenceProblems(root, rangeSpec string) (problems, warnings []string, err error) {
	end := rangeEnd(rangeSpec)
	cited, err := citedChanges(root, rangeSpec, end)
	if unreadable := (*exportError)(nil); errors.As(err, &unreadable) {
		return nil, []string{fmt.Sprintf("Domain Memory evidence skipped (%s); CI still checks it", unreadable.reason)}, nil
	}
	if err != nil || len(cited) == 0 {
		return nil, nil, err
	}
	snapshot, err := os.MkdirTemp("", "devctl-evidence-")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(snapshot)
	if err := extractCommitted(root, end, snapshot, append([]string{registryDirectory, sourceMapFile}, cited...)); err != nil {
		return nil, nil, err
	}
	args := []string{"verify-evidence", "--repo-root", snapshot, "--registry-root", filepath.Join(snapshot, filepath.FromSlash(domainMemoryRoot))}
	for _, file := range cited {
		args = append(args, "--path", file)
	}
	output, runErr := domainMemoryTool(root, args...)
	report, ok := parsedEvidenceReport(output)
	if !ok {
		reason := firstLine(output)
		if reason == "" && runErr != nil {
			reason = runErr.Error()
		}
		return nil, []string{fmt.Sprintf("Domain Memory evidence skipped (%s); CI still checks it", reason)}, nil
	}
	for _, result := range *report.Results {
		if !passingEvidence[result.Status] {
			location := result.Path
			if result.CitedLines.Start > 0 {
				location = fmt.Sprintf("%s:%d-%d", result.Path, result.CitedLines.Start, result.CitedLines.End)
			}
			problems = append(problems, fmt.Sprintf(
				"Domain Memory evidence: %s/%s cites %s, which is %s as committed; check the fact still holds, then re-cite it (verify-evidence --path %s)",
				result.Asset, result.ID, location, result.Status, result.Path))
		}
	}
	if len(problems) == 0 && runErr != nil {
		problems = append(problems, fmt.Sprintf("Domain Memory evidence: verify-evidence failed on the committed bytes (%v); run it with --path on the files you changed", runErr))
	}
	return problems, nil, nil
}

func parsedEvidenceReport(output string) (evidenceReport, bool) {
	var report evidenceReport
	start := strings.Index(output, "{")
	return report, start >= 0 && json.NewDecoder(strings.NewReader(output[start:])).Decode(&report) == nil && report.Results != nil
}

func citedChanges(root, rangeSpec, end string) ([]string, error) {
	hasRegistry, err := committedPathExists(root, end, registryDirectory)
	if err != nil || !hasRegistry {
		return nil, err
	}
	citedPaths, err := committedCitedPaths(root, end)
	if err != nil {
		return nil, err
	}
	changed, err := gitOutput(root, "-c", "core.quotepath=false", "diff", "--name-only", "--no-renames", rangeSpec)
	if err != nil {
		return nil, err
	}
	var cited []string
	for _, file := range lines(changed) {
		if citedPaths[file] {
			cited = append(cited, file)
		}
	}
	return cited, nil
}

func committedPathExists(root, commit, path string) (bool, error) {
	listing, err := gitOutput(root, "ls-tree", "--name-only", commit, "--", path)
	return strings.TrimSpace(listing) != "", err
}

func committedCitedPaths(root, commit string) (map[string]bool, error) {
	assets, err := exportRegistry(root, "--repo-root", root, "--commit", commit)
	if err != nil {
		return nil, err
	}
	paths := map[string]bool{}
	for _, document := range assets {
		var value any
		if err := json.Unmarshal(document, &value); err != nil {
			return nil, fmt.Errorf("the Domain Memory export at %s is not JSON: %w", commit, err)
		}
		collectCitedPaths(value, paths)
	}
	return paths, nil
}

func collectCitedPaths(value any, paths map[string]bool) {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			if references, ok := child.([]any); ok && key == "evidence" {
				addCitationPaths(references, paths)
			}
			collectCitedPaths(child, paths)
		}
	case []any:
		for _, child := range node {
			collectCitedPaths(child, paths)
		}
	}
}

func extractCommitted(root, commit, target string, paths []string) error {
	present := []string{}
	for _, path := range paths {
		exists, err := committedPathExists(root, commit, path)
		if err != nil {
			return err
		}
		if exists {
			present = append(present, path)
		}
	}
	command := exec.Command("git", append([]string{"-C", root, "-c", "core.autocrlf=false", "archive", "--format=tar", commit, "--"}, present...)...)
	var archive, stderr bytes.Buffer
	command.Stdout, command.Stderr = &archive, &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("git archive: %w: %s", err, firstLine(stderr.String()))
	}
	reader := tar.NewReader(&archive)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if err := writeArchived(target, header.Name, reader); err != nil {
			return err
		}
	}
}

func writeArchived(target, name string, body io.Reader) error {
	destination := filepath.Join(target, filepath.FromSlash(name))
	if !strings.HasPrefix(destination, filepath.Clean(target)+string(filepath.Separator)) {
		return fmt.Errorf("git archive entry escapes the snapshot: %s", name)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	content, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	return os.WriteFile(destination, content, 0o644)
}

func addCitationPaths(references []any, paths map[string]bool) {
	for _, reference := range references {
		citation, _ := reference.(map[string]any)
		if path, ok := citation["path"].(string); ok {
			paths[path] = true
		}
	}
}
