package main

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestDomainMemoryExecutablePicksTheCommittedBinaryForThePlatform(t *testing.T) {
	engine := filepath.Join("repo", ".claude", "skills", "domain-memory", "engine")
	cases := []struct {
		name, goos, goarch, want string
	}{
		{"linux on x86-64", "linux", "amd64", filepath.Join(engine, "bin", "linux-x86_64", "domain-memory")},
		{"linux on arm builds its own", "linux", "arm64", filepath.Join(engine, "target", "release", "domain-memory")},
		{"macOS on apple silicon", "darwin", "arm64", filepath.Join(engine, "bin", "macos-universal", "domain-memory")},
		{"macOS on intel", "darwin", "amd64", filepath.Join(engine, "bin", "macos-universal", "domain-memory")},
		{"windows on x86-64", "windows", "amd64", filepath.Join(engine, "bin", "windows-x86_64", "domain-memory.exe")},
		{"windows on arm builds its own", "windows", "arm64", filepath.Join(engine, "target", "release", "domain-memory")},
		{"another system builds its own", "freebsd", "amd64", filepath.Join(engine, "target", "release", "domain-memory")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := domainMemoryExecutable("repo", c.goos, c.goarch); got != c.want {
				t.Errorf("executable = %q, want %q", got, c.want)
			}
		})
	}
}

func TestDomainMemoryOutput(t *testing.T) {
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
