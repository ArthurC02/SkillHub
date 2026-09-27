package mxcdrv

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	fakePrefix = "fake-mxc"

	fakeWorks         = ""
	fakeBackendFails  = "-backend-error"
	fakeRuntimeFails  = "-runtime-fails"
	fakeBackendReport = `{"error":{"code":"backend_error","message":"the backend could not create the container"}}`
)

func TestMain(m *testing.M) {
	if mode, ok := fakeMode(); ok {
		os.Exit(runFake(mode, os.Args[1]))
	}
	os.Exit(m.Run())
}

func fakeMode() (string, bool) {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	if len(os.Args) != 2 || !strings.HasPrefix(name, fakePrefix) {
		return "", false
	}
	return strings.TrimPrefix(name, fakePrefix), true
}

func runFake(mode, policyPath string) int {
	recordLaunch()
	switch mode {
	case fakeBackendFails:
		fmt.Println(fakeBackendReport)
		return 127
	case fakeRuntimeFails:
		return 1
	}
	p, err := readFakePolicy(policyPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	args := splitCommandLine(p.Process.CommandLine)
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "empty commandLine")
		return 2
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = p.Process.Cwd
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	err = cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return 0
}

func readFakePolicy(path string) (policy, error) {
	var p policy
	data, err := os.ReadFile(path)
	if err != nil {
		return p, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return p, dec.Decode(&p)
}

func recordLaunch() {
	f, err := os.OpenFile(os.Args[0]+".launches", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.WriteString("launch\n")
	_ = f.Close()
}

func splitCommandLine(line string) []string {
	var args []string
	var current strings.Builder
	quoted, inArg := false, false
	for _, r := range line {
		switch {
		case r == '"':
			quoted, inArg = !quoted, true
		case r == ' ' && !quoted:
			if inArg {
				args = append(args, current.String())
				current.Reset()
			}
			inArg = false
		default:
			current.WriteRune(r)
			inArg = true
		}
	}
	if inArg {
		args = append(args, current.String())
	}
	return args
}

func fakeMXC(t *testing.T, mode string) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("locate the test binary: %v", err)
	}
	name := fakePrefix + mode
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dst := filepath.Join(t.TempDir(), name)
	src, err := os.Open(self)
	if err != nil {
		t.Fatalf("open the test binary: %v", err)
	}
	defer src.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatalf("create the fake mxc: %v", err)
	}
	if _, err := io.Copy(out, src); err != nil {
		_ = out.Close()
		t.Fatalf("copy the fake mxc: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close the fake mxc: %v", err)
	}
	return dst
}

func fakeLaunches(t *testing.T, fake string) int {
	t.Helper()
	data, err := os.ReadFile(fake + ".launches")
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatalf("read the fake mxc launch log: %v", err)
	}
	return strings.Count(string(data), "launch\n")
}
