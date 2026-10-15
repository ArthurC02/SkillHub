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

var failingEvidence = map[string]bool{"stale": true, "unhashed": true, "missing": true, "invalid": true}

type evidenceReport struct {
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

func committedEvidenceProblems(root, rangeSpec string) (problems, warnings []string, err error) {
	end := rangeEnd(rangeSpec)
	cited, err := citedChanges(root, rangeSpec, end)
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
	for _, result := range report.Results {
		if failingEvidence[result.Status] {
			problems = append(problems, fmt.Sprintf(
				"Domain Memory evidence: %s/%s cites %s:%d-%d, which is %s as committed; check the fact still holds, then re-cite it (verify-evidence --path %s)",
				result.Asset, result.ID, result.Path, result.CitedLines.Start, result.CitedLines.End, result.Status, result.Path))
		}
	}
	return problems, nil, nil
}

func parsedEvidenceReport(output string) (evidenceReport, bool) {
	var report evidenceReport
	start := strings.Index(output, "{")
	return report, start >= 0 && json.Unmarshal([]byte(output[start:]), &report) == nil
}

func citedChanges(root, rangeSpec, end string) ([]string, error) {
	if !committedPathExists(root, end, registryDirectory) {
		return nil, nil
	}
	registry, err := committedRegistryText(root, end)
	if err != nil {
		return nil, err
	}
	changed, err := gitOutput(root, "diff", "--name-only", rangeSpec)
	if err != nil {
		return nil, err
	}
	var cited []string
	for _, file := range lines(changed) {
		if strings.Contains(registry, `"path": "`+file+`"`) {
			cited = append(cited, file)
		}
	}
	return cited, nil
}

func committedPathExists(root, commit, path string) bool {
	_, err := gitOutput(root, "cat-file", "-e", commit+":"+path)
	return err == nil
}

func committedRegistryText(root, commit string) (string, error) {
	listing, err := gitOutput(root, "ls-tree", "--name-only", commit+":"+registryDirectory)
	if err != nil {
		return "", err
	}
	var text strings.Builder
	for _, name := range lines(listing) {
		body, err := gitOutput(root, "show", commit+":"+registryDirectory+"/"+name)
		if err != nil {
			return "", err
		}
		text.WriteString(body)
	}
	return text.String(), nil
}

func extractCommitted(root, commit, target string, paths []string) error {
	present := []string{}
	for _, path := range paths {
		if committedPathExists(root, commit, path) {
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
