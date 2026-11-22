package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDomainMemoryAssetIsTheReleaseExecutableForThePlatform(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, goos, goarch, want string
	}{
		{"linux on x86-64", "linux", "amd64", "domain-memory-linux-x86_64"},
		{"macOS on apple silicon", "darwin", "arm64", "domain-memory-macos-universal"},
		{"macOS on intel", "darwin", "amd64", "domain-memory-macos-universal"},
		{"windows on x86-64", "windows", "amd64", "domain-memory-windows-x86_64.exe"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got, err := domainMemoryAsset(c.goos, c.goarch); err != nil || got != c.want {
				t.Errorf("asset = %q, %v; want %q", got, err, c.want)
			}
		})
	}
}

func TestDomainMemoryAssetRefusesAPlatformNoReleaseCarries(t *testing.T) {
	t.Parallel()
	for _, platform := range [][2]string{{"linux", "arm64"}, {"windows", "arm64"}, {"freebsd", "amd64"}} {
		_, err := domainMemoryAsset(platform[0], platform[1])
		if err == nil || !strings.Contains(err.Error(), "carry no executable for "+platform[0]+"/"+platform[1]) {
			t.Errorf("%s/%s: err = %v, want the platform named", platform[0], platform[1], err)
		}
	}
}

func TestDomainMemoryExecutableIsInTheInstallDirectory(t *testing.T) {
	t.Parallel()
	if got, want := domainMemoryExecutable("repo", "windows"), filepath.Join("repo", ".domain-memory", "domain-memory.exe"); got != want {
		t.Errorf("windows executable = %q, want %q", got, want)
	}
	if got, want := domainMemoryExecutable("repo", "linux"), filepath.Join("repo", ".domain-memory", "domain-memory"); got != want {
		t.Errorf("linux executable = %q, want %q", got, want)
	}
}

func TestDomainMemoryOutput(t *testing.T) {
	t.Parallel()
	commandErr := errors.New("exit status 1")
	cases := []struct {
		name   string
		output string
		err    error
		want   string
	}{
		{
			name:   "script refusal after launcher warning",
			output: "WARN launcher notice\r\nERROR: rules/rule.md: malformed concept\r\n",
			err:    commandErr,
			want:   "ERROR: rules/rule.md: malformed concept",
		},
		{
			name:   "successful JSON has surrounding whitespace removed",
			output: " \n{\"results\": []}\n ",
			want:   "{\"results\": []}",
		},
		{
			name:   "failed evidence JSON remains available to its parser",
			output: "WARN launcher notice\n{\"results\":[]}",
			err:    commandErr,
			want:   "WARN launcher notice\n{\"results\":[]}",
		},
		{
			name:   "launcher failure keeps its diagnostics",
			output: "error: Python could not start\nhint: select interpreter",
			err:    commandErr,
			want:   "error: Python could not start\nhint: select interpreter",
		},
		{
			name: "empty failure output lets the caller use the process error",
			err:  commandErr,
		},
		{
			name:   "successful content containing an error line stays intact",
			output: "header\nERROR: quoted skill text\nfooter",
			want:   "header\nERROR: quoted skill text\nfooter",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := domainMemoryOutput(c.output, c.err); got != c.want {
				t.Errorf("output = %q, want %q", got, c.want)
			}
		})
	}
}

func TestDomainMemoryFailureReasonNamesWhatFailedInsideAJSONReport(t *testing.T) {
	t.Parallel()
	commandErr := errors.New("exit status 1")
	staleEvidence := `{"results": [` +
		`{"asset": "contexts", "id": "activity", "path": "a.go", "status": "current", "cited_lines": {"start": 1, "end": 2}},` +
		`{"asset": "rules", "id": "run-budget", "path": "b.go", "status": "stale", "cited_lines": {"start": 10, "end": 14}}` +
		`], "summary": {"current": 1, "stale": 1}}`
	cases := []struct {
		name, output, want string
	}{
		{"a failing citation among passing ones", staleEvidence, "rules/run-budget cites b.go:10-14 (stale)"},
		{"a launcher notice before the report", "WARN launcher notice\n" + staleEvidence, "rules/run-budget cites b.go:10-14 (stale)"},
		{"a citation without line numbers", `{"results": [{"asset": "rules", "id": "x", "path": "c.go", "status": "missing"}]}`, "rules/x cites c.go (missing)"},
		{"more failures than are listed", `{"results": [` +
			`{"asset": "r", "id": "a", "path": "1.go", "status": "stale"},` +
			`{"asset": "r", "id": "b", "path": "2.go", "status": "stale"},` +
			`{"asset": "r", "id": "c", "path": "3.go", "status": "stale"},` +
			`{"asset": "r", "id": "d", "path": "4.go", "status": "stale"}]}`,
			"r/a cites 1.go (stale); r/b cites 2.go (stale); r/c cites 3.go (stale); and 1 more"},
		{"a report that carries only a status", `{"status": "broken", "events": 3}`, "status broken"},
		{"a summary line after the report is kept", staleEvidence + "\nverify-evidence failed: 1 stale citations\n",
			"rules/run-budget cites b.go:10-14 (stale); verify-evidence failed: 1 stale citations"},
		{"the cause written only after a passing-looking report", "{\"status\": \"valid\"}\nnote\nverify-audit failed: chain broken at event 12",
			"status valid; verify-audit failed: chain broken at event 12"},
		{"a JSON report with no field it recognises", "{\n  \"oops\": true\n}", `{"oops":true}`},
		{"plain text keeps its first line", "Registry is invalid.\nsecond line", "Registry is invalid."},
		{"no output falls back to the process error", "", "exit status 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := domainMemoryFailureReason(c.output, commandErr); got != c.want {
				t.Errorf("reason = %q, want %q", got, c.want)
			}
		})
	}
}

func TestDomainMemoryProblemsReportsTheFailingCitationNotItsOpeningBrace(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "domain-memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	original := domainMemoryTool
	t.Cleanup(func() { domainMemoryTool = original })
	domainMemoryTool = func(_ string, args ...string) (string, error) {
		if args[0] != "verify-evidence" {
			return "", nil
		}
		return "{\n  \"results\": [{\"asset\": \"rules\", \"id\": \"run-budget\", \"path\": \"b.go\", \"status\": \"stale\"}]\n}", errors.New("exit status 1")
	}
	problems := domainMemoryProblems(root)
	if len(problems) != 1 || problems[0] != "Domain Memory verify-evidence: rules/run-budget cites b.go (stale)" {
		t.Fatalf("problems = %q, want the stale citation named", problems)
	}
}
