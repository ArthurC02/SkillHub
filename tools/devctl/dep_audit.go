package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type auditTarget struct {
	ecosystem string
	dir       string
	shipped   bool
}

var auditTargets = []auditTarget{
	{ecosystem: "npm", dir: "apps/web", shipped: true},
	{ecosystem: "go", dir: "apps/platform", shipped: true},
	{ecosystem: "go", dir: "apps/sandbox", shipped: true},
	{ecosystem: "python", dir: "apps/llm", shipped: true},
	{ecosystem: "npm", dir: "packages/api-client-ts"},
	{ecosystem: "npm", dir: "tools/pglite"},
	{ecosystem: "npm", dir: "infra/images/runtime-agent-sdk"},
	{ecosystem: "go", dir: "tools/devctl"},
	{ecosystem: "python", dir: "tools/codegen/python"},
}

var npmSeverityRank = map[string]int{"info": 0, "low": 1, "moderate": 2, "high": 3, "critical": 4}

type vulnFinding struct {
	subject string
	id      string
	fix     string
}

func depAudit(root string, args []string, out io.Writer) error {
	full, err := parseFullFlag(args)
	if err != nil {
		return err
	}
	toolchain, err := parseToolchain(filepath.Join(root, "tools", "toolchain.yaml"))
	if err != nil {
		return err
	}
	if toolchain["govulncheck"] == "" || toolchain["pip_audit"] == "" {
		return errors.New("govulncheck and pip_audit versions are missing from tools/toolchain.yaml")
	}
	var fail, note []string
	for _, target := range auditTargets {
		if !full && !target.shipped {
			continue
		}
		findings, err := scanTarget(filepath.Join(root, filepath.FromSlash(target.dir)), target.ecosystem, full, toolchain)
		if err != nil {
			return fmt.Errorf("%s %s: %w", target.ecosystem, target.dir, err)
		}
		fmt.Fprintf(out, "scanned %s %s\n", target.ecosystem, target.dir)
		for _, finding := range findings {
			line := fmt.Sprintf("%s %s: %s %s", target.ecosystem, target.dir, finding.subject, finding.id)
			if finding.fix == "" {
				note = append(note, line+" (no fix yet)")
			} else {
				fail = append(fail, line+" (fix: "+finding.fix+")")
			}
		}
	}
	return reportAudit(fail, note, out)
}

func parseFullFlag(args []string) (bool, error) {
	switch {
	case len(args) == 0:
		return false, nil
	case len(args) == 1 && args[0] == "--full":
		return true, nil
	}
	return false, errors.New("usage: devctl dep-audit [--full]")
}

func reportAudit(fail, note []string, out io.Writer) error {
	for _, line := range note {
		fmt.Fprintln(out, "NOTE", line)
	}
	for _, line := range fail {
		fmt.Fprintln(out, "FAIL", line)
	}
	if len(fail) > 0 {
		return fmt.Errorf("%d fixable vulnerabilities; upgrade to the fixed versions", len(fail))
	}
	fmt.Fprintln(out, "no fixable vulnerabilities")
	return nil
}

func scanTarget(dir, ecosystem string, full bool, toolchain map[string]string) ([]vulnFinding, error) {
	switch ecosystem {
	case "npm":
		args := []string{"audit", "--json"}
		minSeverity := "moderate"
		if !full {
			args = append(args, "--omit=dev")
			minSeverity = "high"
		}
		data, err := auditToolOutput(dir, "npm", args...)
		if err != nil {
			return nil, err
		}
		return npmFindings(data, minSeverity)
	case "go":
		govulncheck := "golang.org/x/vuln/cmd/govulncheck@v" + toolchain["govulncheck"]
		data, err := auditToolOutput(dir, "go", "run", govulncheck, "-format", "json", "./...")
		if err != nil {
			return nil, err
		}
		return govulncheckFindings(data, !full)
	case "python":
		data, err := pipAuditOutput(dir, full, toolchain["pip_audit"])
		if err != nil {
			return nil, err
		}
		return pipAuditFindings(data)
	}
	return nil, fmt.Errorf("unknown ecosystem %q", ecosystem)
}

func pipAuditOutput(dir string, full bool, version string) ([]byte, error) {
	args := []string{"export", "--frozen", "--no-emit-project", "--no-emit-local", "--quiet"}
	if !full {
		args = append(args, "--no-dev")
	}
	requirements, err := auditToolOutput(dir, "uv", args...)
	if err != nil {
		return nil, err
	}
	file, err := os.CreateTemp("", "dep-audit-*.txt")
	if err != nil {
		return nil, err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(requirements); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	return auditToolOutput(dir, "uvx", "pip-audit=="+version,
		"--requirement", file.Name(), "--disable-pip", "--require-hashes", "--format", "json")
}

func auditToolOutput(dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "UV_LINK_MODE=copy")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	var exit *exec.ExitError
	exitedWithAReport := errors.As(err, &exit) && len(bytes.TrimSpace(data)) > 0
	if err != nil && !exitedWithAReport {
		return nil, fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, stderr.String())
	}
	return data, nil
}

func npmFindings(data []byte, minSeverity string) ([]vulnFinding, error) {
	var report struct {
		Vulnerabilities *map[string]struct {
			Severity     string            `json:"severity"`
			FixAvailable json.RawMessage   `json:"fixAvailable"`
			Via          []json.RawMessage `json:"via"`
		} `json:"vulnerabilities"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("npm audit output: %w", err)
	}
	if report.Vulnerabilities == nil {
		return nil, fmt.Errorf("npm audit output has no vulnerabilities report: %.300s", data)
	}
	var findings []vulnFinding
	for name, vuln := range *report.Vulnerabilities {
		rank, known := npmSeverityRank[vuln.Severity]
		if known && rank < npmSeverityRank[minSeverity] {
			continue
		}
		finding := vulnFinding{subject: name, id: vuln.Severity + " " + npmAdvisories(vuln.Via)}
		if string(vuln.FixAvailable) != "false" {
			finding.fix = "npm audit fix"
		}
		findings = append(findings, finding)
	}
	sortFindings(findings)
	return findings, nil
}

func npmAdvisories(via []json.RawMessage) string {
	var urls []string
	for _, raw := range via {
		var advisory struct {
			URL string `json:"url"`
		}
		if json.Unmarshal(raw, &advisory) == nil && advisory.URL != "" {
			urls = append(urls, advisory.URL)
		}
	}
	if len(urls) == 0 {
		return "via a vulnerable dependency"
	}
	return strings.Join(urls, " ")
}

func govulncheckFindings(data []byte, calledOnly bool) ([]vulnFinding, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	sawConfig := false
	byOSV := map[string]vulnFinding{}
	for {
		var message struct {
			Config  json.RawMessage `json:"config"`
			Finding *struct {
				OSV          string `json:"osv"`
				FixedVersion string `json:"fixed_version"`
				Trace        []struct {
					Module   string `json:"module"`
					Function string `json:"function"`
				} `json:"trace"`
			} `json:"finding"`
		}
		err := decoder.Decode(&message)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("govulncheck output: %w", err)
		}
		if message.Config != nil {
			sawConfig = true
		}
		finding := message.Finding
		if finding == nil || len(finding.Trace) == 0 {
			continue
		}
		if calledOnly && finding.Trace[0].Function == "" {
			continue
		}
		byOSV[finding.OSV] = vulnFinding{subject: finding.Trace[0].Module, id: finding.OSV, fix: finding.FixedVersion}
	}
	if !sawConfig {
		return nil, errors.New("govulncheck output has no config message, so no scan ran")
	}
	findings := make([]vulnFinding, 0, len(byOSV))
	for _, finding := range byOSV {
		findings = append(findings, finding)
	}
	sortFindings(findings)
	return findings, nil
}

func pipAuditFindings(data []byte) ([]vulnFinding, error) {
	var report struct {
		Dependencies *[]struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Vulns   []struct {
				ID          string   `json:"id"`
				FixVersions []string `json:"fix_versions"`
			} `json:"vulns"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("pip-audit output: %w", err)
	}
	if report.Dependencies == nil {
		return nil, fmt.Errorf("pip-audit output has no dependencies list: %.300s", data)
	}
	var findings []vulnFinding
	seen := map[string]bool{}
	for _, dependency := range *report.Dependencies {
		for _, vuln := range dependency.Vulns {
			finding := vulnFinding{
				subject: dependency.Name + " " + dependency.Version,
				id:      vuln.ID,
				fix:     strings.Join(vuln.FixVersions, " or "),
			}
			if seen[finding.subject+" "+finding.id] {
				continue
			}
			seen[finding.subject+" "+finding.id] = true
			findings = append(findings, finding)
		}
	}
	sortFindings(findings)
	return findings, nil
}

func sortFindings(findings []vulnFinding) {
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].subject != findings[j].subject {
			return findings[i].subject < findings[j].subject
		}
		return findings[i].id < findings[j].id
	})
}
