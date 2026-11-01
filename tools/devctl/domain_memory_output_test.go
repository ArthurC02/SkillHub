package main

import (
	"errors"
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
