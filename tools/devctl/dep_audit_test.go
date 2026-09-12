package main

import (
	"bytes"
	"os/exec"
	"path"
	"strings"
	"testing"
)

const npmAuditReport = `{"auditReportVersion":2,"vulnerabilities":{
"lodash":{"severity":"high","fixAvailable":true,"via":[{"url":"https://github.com/advisories/GHSA-high"}]},
"major-only":{"severity":"critical","fixAvailable":{"name":"x","version":"2.0.0","isSemVerMajor":true},"via":["lodash"]},
"stuck":{"severity":"high","fixAvailable":false,"via":[{"url":"https://github.com/advisories/GHSA-stuck"}]},
"mild":{"severity":"moderate","fixAvailable":true,"via":[{"url":"https://github.com/advisories/GHSA-mild"}]},
"quiet":{"severity":"low","fixAvailable":true,"via":[]},
"unrated":{"severity":"brand-new","fixAvailable":true,"via":[]}
}}`

const govulncheckStream = `{"config":{"protocol_version":"v1.0.0","scanner_name":"govulncheck"}}
{"osv":{"id":"GO-1"}}
{"finding":{"osv":"GO-1","fixed_version":"v0.56.0","trace":[{"module":"golang.org/x/crypto","version":"v0.54.0"}]}}
{"finding":{"osv":"GO-2","fixed_version":"v1.2.0","trace":[{"module":"example.com/lib","package":"example.com/lib/p","function":"Parse"},{"module":"skillhub","function":"main"}]}}
{"finding":{"osv":"GO-2","fixed_version":"v1.2.0","trace":[{"module":"example.com/lib","package":"example.com/lib/p"}]}}
{"finding":{"osv":"GO-3","trace":[{"module":"github.com/docker/docker","package":"github.com/docker/docker/client","function":"Do"}]}}
`

const pipAuditReport = `{"dependencies":[
{"name":"datamodel-code-generator","version":"0.35.0","vulns":[{"id":"PYSEC-1","fix_versions":["0.60.1"]},{"id":"PYSEC-2","fix_versions":[]}]},
{"name":"datamodel-code-generator","version":"0.35.0","vulns":[{"id":"PYSEC-1","fix_versions":["0.60.1"]},{"id":"PYSEC-2","fix_versions":[]}]},
{"name":"click","version":"8.4.2","vulns":[]}],"fixes":[]}`

func findingsBySubject(findings []vulnFinding) map[string]vulnFinding {
	bySubject := map[string]vulnFinding{}
	for _, finding := range findings {
		bySubject[finding.subject+" "+finding.id] = finding
	}
	return bySubject
}

func TestNpmAuditCountsTheThresholdSeverityAndAboveAndUnknownSeverities(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		minSeverity string
		want        []string
	}{
		{"the push gate starts at high", "high", []string{"lodash", "major-only", "stuck", "unrated"}},
		{"the weekly scan starts at moderate", "moderate", []string{"lodash", "major-only", "mild", "stuck", "unrated"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			findings, err := npmFindings([]byte(npmAuditReport), tc.minSeverity)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, finding := range findings {
				got = append(got, finding.subject)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNpmAuditTreatsOnlyFixAvailableFalseAsUnfixable(t *testing.T) {
	t.Parallel()
	findings, err := npmFindings([]byte(npmAuditReport), "high")
	if err != nil {
		t.Fatal(err)
	}
	fixes := map[string]string{}
	for _, finding := range findings {
		fixes[finding.subject] = finding.fix
	}
	if fixes["lodash"] == "" || fixes["major-only"] == "" {
		t.Fatalf("fixAvailable true and a semver-major fix object must both count as fixable: %v", fixes)
	}
	if fixes["stuck"] != "" {
		t.Fatalf("fixAvailable false must be unfixable, got %q", fixes["stuck"])
	}
}

func TestNpmAuditNamesTheAdvisory(t *testing.T) {
	t.Parallel()
	findings, err := npmFindings([]byte(npmAuditReport), "high")
	if err != nil {
		t.Fatal(err)
	}
	if got := findings[0]; got.subject != "lodash" || got.id != "high https://github.com/advisories/GHSA-high" {
		t.Fatalf("got %+v", got)
	}
}

func TestNpmAuditErrorOutputIsNotACleanReport(t *testing.T) {
	t.Parallel()
	if _, err := npmFindings([]byte(`{"error":{"code":"ENOLOCK","summary":"no lockfile"}}`), "high"); err == nil {
		t.Fatal("an npm error object parsed as zero vulnerabilities")
	}
}

func TestGovulncheckGateCountsOnlyCalledCodeAndTheWeeklyScanCountsModules(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		calledOnly bool
		want       map[string]string
	}{
		{"the push gate", true, map[string]string{"GO-2": "v1.2.0", "GO-3": ""}},
		{"the weekly scan", false, map[string]string{"GO-1": "v0.56.0", "GO-2": "v1.2.0", "GO-3": ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			findings, err := govulncheckFindings([]byte(govulncheckStream), tc.calledOnly)
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]string{}
			for _, finding := range findings {
				got[finding.id] = finding.fix
			}
			if len(got) != len(tc.want) || len(findings) != len(tc.want) {
				t.Fatalf("got %v (%d findings), want %v", got, len(findings), tc.want)
			}
			for id, fix := range tc.want {
				if actual, ok := got[id]; !ok || actual != fix {
					t.Fatalf("%s: got fix %q (present %v), want %q", id, actual, ok, fix)
				}
			}
		})
	}
}

func TestGovulncheckOutputWithoutAConfigMessageIsNotAScan(t *testing.T) {
	t.Parallel()
	if _, err := govulncheckFindings([]byte(""), true); err == nil {
		t.Fatal("empty govulncheck output parsed as a clean scan")
	}
}

func TestPipAuditReportsEachAdvisoryOnceWithItsFixVersion(t *testing.T) {
	t.Parallel()
	findings, err := pipAuditFindings([]byte(pipAuditReport))
	if err != nil {
		t.Fatal(err)
	}
	got := findingsBySubject(findings)
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2: %+v", len(findings), findings)
	}
	if got["datamodel-code-generator 0.35.0 PYSEC-1"].fix != "0.60.1" {
		t.Fatalf("PYSEC-1 lost its fix version: %+v", findings)
	}
	if finding, ok := got["datamodel-code-generator 0.35.0 PYSEC-2"]; !ok || finding.fix != "" {
		t.Fatalf("PYSEC-2 has no fix version and must be reported as such: %+v", findings)
	}
}

func TestPipAuditOutputWithoutADependencyListIsNotAScan(t *testing.T) {
	t.Parallel()
	if _, err := pipAuditFindings([]byte(`{"error":"resolution failed"}`)); err == nil {
		t.Fatal("pip-audit output without dependencies parsed as a clean scan")
	}
}

func TestTheAuditFailsOnlyOnFixableFindings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		fail     []string
		note     []string
		wantErr  bool
		wantText string
	}{
		{"nothing found", nil, nil, false, "no fixable vulnerabilities"},
		{"only findings without a fix", nil, []string{"go apps/sandbox: docker GO-3 (no fix yet)"}, false, "NOTE go apps/sandbox"},
		{"one fixable finding", []string{"npm apps/web: lodash high (fix: npm audit fix)"}, nil, true, "FAIL npm apps/web"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			err := reportAudit(tc.fail, tc.note, &out)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if !strings.Contains(out.String(), tc.wantText) {
				t.Fatalf("output %q does not contain %q", out.String(), tc.wantText)
			}
		})
	}
}

func TestDepAuditAcceptsOnlyTheFullFlag(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		args     []string
		wantFull bool
		wantErr  bool
	}{
		{"no flag is the push gate", nil, false, false},
		{"--full is the weekly scan", []string{"--full"}, true, false},
		{"an unknown flag", []string{"--all"}, false, true},
		{"a second argument", []string{"--full", "apps/web"}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			full, err := parseFullFlag(tc.args)
			if full != tc.wantFull || (err != nil) != tc.wantErr {
				t.Fatalf("got full=%v err=%v", full, err)
			}
		})
	}
}

func TestEveryLockedProjectIsAudited(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "-C", root, "ls-files", "--", "*package-lock.json", "*uv.lock", "*go.mod").Output()
	if err != nil {
		t.Fatal(err)
	}
	audited := map[string]bool{}
	for _, target := range auditTargets {
		audited[target.dir] = true
	}
	locked := map[string]bool{}
	for _, file := range strings.Fields(string(out)) {
		dir := path.Dir(file)
		if path.Base(file) == "go.mod" {
			goFiles, err := exec.Command("git", "-C", root, "ls-files", "--", dir+"/*.go").Output()
			if err != nil {
				t.Fatal(err)
			}
			if len(bytes.TrimSpace(goFiles)) == 0 {
				continue
			}
		}
		locked[dir] = true
		if !audited[dir] {
			t.Errorf("%s has a lockfile that devctl dep-audit does not scan", dir)
		}
	}
	if len(locked) < 5 {
		t.Fatalf("found only %d locked projects; the git ls-files pathspecs no longer match", len(locked))
	}
	for dir := range audited {
		if !locked[dir] {
			t.Errorf("dep-audit scans %s, which has no tracked lockfile", dir)
		}
	}
}
