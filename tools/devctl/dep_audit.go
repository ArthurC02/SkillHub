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
	{ecosystem: ecosystemNPM, dir: dirAppsWeb, shipped: true},
	{ecosystem: "go", dir: dirAppsPlatform, shipped: true},
	{ecosystem: "go", dir: dirAppsSandbox, shipped: true},
	{ecosystem: ecosystemPython, dir: dirAppsLLM, shipped: true},
	{ecosystem: ecosystemNPM, dir: dirAPIClientTS},
	{ecosystem: ecosystemNPM, dir: "tools/pglite"},
	{ecosystem: ecosystemNPM, dir: "infra/images/runtime-agent-sdk"},
	{ecosystem: "go", dir: "tools/devctl"},
	{ecosystem: ecosystemPython, dir: "tools/codegen/python"},
}

const (
	npmSeverityHighRank     = 3
	npmSeverityCriticalRank = 4
)

var npmSeverityRank = map[string]int{
	"info": 0, "low": 1, "moderate": 2, "high": npmSeverityHighRank, "critical": npmSeverityCriticalRank,
}

type auditScope struct {
	full bool
}

type vulnFinding struct {
	subject    string
	id         string
	fix        string
	unimported bool
}

func (finding vulnFinding) auditStatus() string {
	if finding.fix != "" {
		return statusFail
	}
	if finding.unimported {
		return "INFO"
	}
	return "NOTE"
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
	for _, key := range []string{"govulncheck", "pip_audit", "go_licenses", "zizmor"} {
		if toolchain[key] == "" {
			return fmt.Errorf("%s version is missing from tools/toolchain.yaml", key)
		}
	}
	scope := auditScope{full: full}
	var fail, note, info []string
	for _, target := range auditTargets {
		if !scope.full && !target.shipped {
			continue
		}
		findings, err := scanTarget(filepath.Join(root, filepath.FromSlash(target.dir)), target.ecosystem, scope, toolchain)
		if err != nil {
			return fmt.Errorf("%s %s: %w", target.ecosystem, target.dir, err)
		}
		fmt.Fprintf(out, "scanned %s %s\n", target.ecosystem, target.dir)
		findingFail, findingNote, findingInfo := classifyVulnerabilityFindings(target, findings)
		fail = append(fail, findingFail...)
		note = append(note, findingNote...)
		info = append(info, findingInfo...)
	}
	workflowFindings, err := scanWorkflows(root, toolchain)
	if err != nil {
		return fmt.Errorf("actions .github: %w", err)
	}
	fmt.Fprintln(out, "scanned actions .github")
	for _, finding := range workflowFindings {
		fail = append(fail, fmt.Sprintf("actions .github: %s %s (fix: %s)", finding.subject, finding.id, finding.fix))
	}
	licenseFail, licenseNote, err := licenseAudit(root, toolchain)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "checked the licenses of shipped dependencies")
	fail = append(fail, licenseFail...)
	note = append(note, licenseNote...)
	return reportAudit(fail, note, info, out)
}

func classifyVulnerabilityFindings(target auditTarget, findings []vulnFinding) (fail, note, info []string) {
	for _, finding := range findings {
		line := fmt.Sprintf("%s %s: %s %s", target.ecosystem, target.dir, finding.subject, finding.id)
		switch finding.auditStatus() {
		case "INFO":
			info = append(info, line+" (module advisory; affected packages not imported; no fix yet)")
		case "NOTE":
			note = append(note, line+" (no fix yet)")
		default:
			fail = append(fail, line+" (fix: "+finding.fix+")")
		}
	}
	return fail, note, info
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

func reportAudit(fail, note, info []string, out io.Writer) error {
	for _, line := range info {
		fmt.Fprintln(out, "INFO", line)
	}
	for _, line := range note {
		fmt.Fprintln(out, "NOTE", line)
	}
	for _, line := range fail {
		fmt.Fprintln(out, statusFail, line)
	}
	if len(fail) > 0 {
		return fmt.Errorf("%d problems to fix: upgrade to the fixed version, replace the dependency or fix the workflow", len(fail))
	}
	fmt.Fprintln(out, "no fixable vulnerabilities, disallowed licenses or workflow findings")
	return nil
}

func scanTarget(dir, ecosystem string, scope auditScope, toolchain map[string]string) ([]vulnFinding, error) {
	switch ecosystem {
	case ecosystemNPM:
		args := []string{"audit", "--json"}
		minSeverity := "moderate"
		if !scope.full {
			args = append(args, "--omit=dev")
			minSeverity = "high"
		}
		data, err := auditToolOutput(dir, ecosystemNPM, args...)
		if err != nil {
			return nil, err
		}
		return npmFindings(data, minSeverity)
	case "go":
		govulncheck := "golang.org/x/vuln/cmd/govulncheck@v" + toolchain["govulncheck"]
		data, err := auditToolOutput(dir, "go", cmdRun, govulncheck, "-format", "json", "./...")
		if err != nil {
			return nil, err
		}
		return govulncheckFindings(data, scope)
	case ecosystemPython:
		data, err := pipAuditOutput(dir, scope, toolchain["pip_audit"])
		if err != nil {
			return nil, err
		}
		return pipAuditFindings(data)
	}
	return nil, fmt.Errorf("unknown ecosystem %q", ecosystem)
}

func pipAuditOutput(dir string, scope auditScope, version string) ([]byte, error) {
	args := []string{"export", "--frozen", "--no-emit-project", "--no-emit-local", flagQuiet}
	if !scope.full {
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
	exitedWithAReport := name != "go" && errors.As(err, &exit) && len(bytes.TrimSpace(data)) > 0
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

type govulncheckConfig struct {
	ScanLevel string `json:"scan_level"`
	ScanMode  string `json:"scan_mode"`
}

type govulncheckFrame struct {
	Module   string          `json:"module"`
	Package  string          `json:"package"`
	Function string          `json:"function"`
	Receiver string          `json:"receiver"`
	Position json.RawMessage `json:"position"`
}

type govulncheckFinding struct {
	OSV          string             `json:"osv"`
	FixedVersion string             `json:"fixed_version"`
	Trace        []govulncheckFrame `json:"trace"`
}

func (config govulncheckConfig) confirmsUnimported(finding govulncheckFinding) bool {
	if config.ScanMode != "source" || (config.ScanLevel != "symbol" && config.ScanLevel != "package") || len(finding.Trace) != 1 {
		return false
	}
	frame := finding.Trace[0]
	return frame.Module != "" && frame.Package == "" && frame.Function == "" && frame.Receiver == "" && frame.Position == nil
}

func mergeVulnerabilityEvidence(previous, next vulnFinding) vulnFinding {
	if previous.id == "" {
		return next
	}
	next.unimported = next.unimported && previous.unimported
	if next.fix == "" {
		next.fix = previous.fix
	}
	return next
}

func govulncheckFindings(data []byte, scope auditScope) ([]vulnFinding, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	sawConfig := false
	var config govulncheckConfig
	byOSV := map[string]vulnFinding{}
	for {
		var message struct {
			Config  *govulncheckConfig  `json:"config"`
			Finding *govulncheckFinding `json:"finding"`
		}
		err := decoder.Decode(&message)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("govulncheck output: %w", err)
		}
		if message.Config != nil && sawConfig {
			return nil, errors.New("govulncheck output contains multiple scan configurations")
		}
		if message.Config != nil {
			sawConfig = true
			config = *message.Config
		}
		finding := message.Finding
		if finding == nil {
			continue
		}
		if len(finding.Trace) == 0 {
			return nil, errors.New("govulncheck finding has no trace")
		}
		if !scope.full && finding.Trace[0].Function == "" {
			continue
		}
		candidate := vulnFinding{subject: finding.Trace[0].Module, id: finding.OSV, fix: finding.FixedVersion, unimported: config.confirmsUnimported(*finding)}
		byOSV[finding.OSV] = mergeVulnerabilityEvidence(byOSV[finding.OSV], candidate)
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

func scanWorkflows(root string, toolchain map[string]string) ([]vulnFinding, error) {
	args := []string{"zizmor==" + toolchain["zizmor"], "--format", "json"}
	if os.Getenv("GH_TOKEN") == "" && os.Getenv("GITHUB_TOKEN") == "" {
		args = append(args, "--offline")
	}
	data, err := auditToolOutput(root, "uvx", append(args, ".github/workflows", ".github/actions")...)
	if err != nil {
		return nil, err
	}
	return zizmorFindings(data)
}

func zizmorFindings(data []byte) ([]vulnFinding, error) {
	if !bytes.HasPrefix(bytes.TrimSpace(data), []byte("[")) {
		return nil, fmt.Errorf("zizmor output is not a list of findings: %.300s", data)
	}
	var report []struct {
		Ident          string `json:"ident"`
		Ignored        bool   `json:"ignored"`
		Determinations struct {
			Severity string `json:"severity"`
		} `json:"determinations"`
		Locations []struct {
			Symbolic struct {
				Key struct {
					Local struct {
						Path string `json:"verbatim_path"`
					} `json:"Local"`
				} `json:"key"`
				Kind string `json:"kind"`
			} `json:"symbolic"`
			Concrete struct {
				Location struct {
					Start struct {
						Row int `json:"row"`
					} `json:"start_point"`
				} `json:"location"`
			} `json:"concrete"`
		} `json:"locations"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("zizmor output: %w", err)
	}
	var findings []vulnFinding
	for _, finding := range report {
		severity := finding.Determinations.Severity
		reportable := strings.EqualFold(severity, "Medium") || strings.EqualFold(severity, "High")
		if finding.Ignored || !reportable {
			continue
		}
		where := "(no location)"
		for i, location := range finding.Locations {
			primary := location.Symbolic.Kind == "Primary"
			if i == 0 || primary {
				where = fmt.Sprintf("%s:%d", location.Symbolic.Key.Local.Path, location.Concrete.Location.Start.Row+1)
			}
			if primary {
				break
			}
		}
		findings = append(findings, vulnFinding{subject: finding.Ident, id: where, fix: "https://docs.zizmor.sh/audits/#" + finding.Ident})
	}
	sortFindings(findings)
	return findings, nil
}
