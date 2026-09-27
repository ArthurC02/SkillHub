package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"testing"
)

const devctlMainArgs = "DEVCTL_CHARACTERIZATION_MAIN_ARGS"

func TestMain(m *testing.M) {
	if raw, ok := os.LookupEnv(devctlMainArgs); ok {
		var args []string
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			os.Exit(99)
		}
		os.Args = append([]string{"devctl"}, args...)
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func runDevctlMain(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	encoded, err := json.Marshal(append([]string{}, args...))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), devctlMainArgs+"="+string(encoded))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		return exit.ExitCode(), stdout.String(), stderr.String()
	case err != nil:
		t.Fatal(err)
	}
	return 0, stdout.String(), stderr.String()
}

func TestDevctlDispatchesItsOwnArgumentsWithoutDelegating(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name           string
		args           []string
		code           int
		stdout, stderr string
	}{
		{"no command prints the usage and exits 2", nil, 2, "", usage},
		{"an unknown command is named and exits 2", []string{"no-such-command"}, 2, "",
			"unknown command \"no-such-command\"\n\n" + usage},
		{"help prints the usage to stdout", []string{"help"}, 0, usage, ""},
		{"-h prints the usage to stdout", []string{"-h"}, 0, usage, ""},
		{"--help prints the usage to stdout", []string{"--help"}, 0, usage, ""},
		{"profile-check with no profile is a usage error", []string{"profile-check"}, 1, "",
			"devctl: usage: devctl profile-check model\n"},
		{"profile-check with one profile reaches the profile check", []string{"profile-check", "no-such-profile"}, 1, "",
			"devctl: unknown profile \"no-such-profile\"\n"},
		{"profile-check with two profiles is a usage error", []string{"profile-check", "model", "extra"}, 1, "",
			"devctl: usage: devctl profile-check model\n"},
		{"test-report with no directory is a usage error", []string{"test-report"}, 1, "",
			"devctl: usage: devctl test-report dir [go test args]\n"},
		{"gen receives the arguments after the command", []string{"gen", "--bogus"}, 1, "",
			"devctl: unknown gen option \"--bogus\"\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			code, stdout, stderr := runDevctlMain(t, c.args...)
			if code != c.code || stdout != c.stdout || stderr != c.stderr {
				t.Fatalf("devctl %q = exit %d, stdout %q, stderr %q; want exit %d, stdout %q, stderr %q",
					c.args, code, stdout, stderr, c.code, c.stdout, c.stderr)
			}
		})
	}
}
