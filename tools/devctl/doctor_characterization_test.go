package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAMissingRequiredToolFailsTheDoctor(t *testing.T) {
	t.Parallel()
	result := checkRequiredVersion("skillhub-no-such-tool", nil, "")
	if result.status != "FAIL" || !result.required || result.detail != "not found on PATH" {
		t.Fatalf("got %+v, want a required FAIL saying the tool is not on PATH", result)
	}
}

func TestAMissingOptionalToolOnlyWarns(t *testing.T) {
	t.Parallel()
	result := checkOptionalVersion("skillhub-no-such-tool", nil, "")
	if result.status != "WARN" || result.required || result.detail != "not found on PATH" {
		t.Fatalf("got %+v, want an optional WARN saying the tool is not on PATH", result)
	}
}

func TestARequiredToolAtTheWrongVersionFailsTheDoctor(t *testing.T) {
	t.Parallel()
	result := checkRequiredVersion("go", []string{"version"}, "go0.1")
	if result.status != "FAIL" || !result.required {
		t.Fatalf("got %+v, want a required FAIL for a version mismatch", result)
	}
}

func TestBashTaskCapability(t *testing.T) {
	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	program := `package main
import ("fmt"; "os")
func main() {
 if len(os.Args) == 2 && os.Args[1] == "--version" { fmt.Println("fixture version"); return }
 if len(os.Args) != 3 || os.Args[1] != "-c" || os.Args[2] != "builtin mapfile -t lines <<< ready && [[ ${lines[0]} == ready ]]" { fmt.Fprintln(os.Stderr,"unexpected capability probe"); os.Exit(2) }
 if os.Getenv("BASH_ENV") != "" { fmt.Fprintln(os.Stderr,"startup environment not cleared"); os.Exit(3) }
 mode := os.Getenv("SKILLHUB_BASH_FIXTURE")
 if mode == "usable" { return }
 fmt.Fprintln(os.Stderr,mode)
 os.Exit(1)
}
`
	file := filepath.Join(dir, "fixture.go")
	if err := os.WriteFile(file, []byte(program), 0o600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "bash")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	if output, err := exec.Command(goPath, "build", "-o", binary, file).CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v: %s", err, output)
	}
	for _, test := range []struct {
		name   string
		path   string
		mode   string
		status string
		detail string
	}{
		{"missing", t.TempDir(), "", "WARN", "not found on PATH"},
		{"unusable", dir, "shell cannot start", "WARN", "shell cannot start"},
		{"version available but mapfile unsupported", dir, "mapfile unsupported", "WARN", "mapfile unsupported"},
		{"silent failure", dir, "", "WARN", "exit status 1"},
		{"usable", dir, "usable", "PASS", "mapfile available"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("PATH", test.path)
			t.Setenv("SKILLHUB_BASH_FIXTURE", test.mode)
			t.Setenv("BASH_ENV", "must-not-load")
			result := checkBash()
			if result.name != "bash (shell tasks)" || result.status != test.status || result.required || !strings.Contains(result.detail, test.detail) {
				t.Fatalf("got %+v, want optional %s with %q", result, test.status, test.detail)
			}
			if test.status == "WARN" && !strings.Contains(result.detail, "Bash with mapfile on PATH") {
				t.Fatalf("warning has no actionable shell-task hint: %+v", result)
			}
		})
	}
}

func TestDoctorReportsShellTaskWarning(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := doctor(root, &output); err == nil {
		t.Fatal("missing required tools must still fail doctor")
	}
	if !strings.Contains(output.String(), "WARN  bash (shell tasks)") || !strings.Contains(output.String(), "Bash with mapfile on PATH") {
		t.Fatalf("doctor omitted the shell-task diagnostic: %s", output.String())
	}
}
