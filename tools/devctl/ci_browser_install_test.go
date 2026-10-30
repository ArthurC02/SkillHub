package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func browserDependencyInstallScript(t *testing.T) string {
	t.Helper()
	job := browserWorkflowJob(t, "web-browser-shards")
	_, tail, ok := strings.Cut(job, "        name: Install Linux browser dependencies\n        run: |\n")
	if !ok {
		t.Fatal("missing bounded Linux browser dependency installation")
	}
	var lines []string
	for _, line := range strings.Split(tail, "\n") {
		if !strings.HasPrefix(line, "          ") {
			break
		}
		lines = append(lines, strings.TrimPrefix(line, "          "))
	}
	script := strings.Join(lines, "\n")
	script = strings.ReplaceAll(script, "${{ matrix.browser }}", "webkit")
	return strings.ReplaceAll(script, "~/playwright-debs", `"$SKILLHUB_TEST_CACHE"`)
}

func writeBrowserInstallStub(t *testing.T, directory, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name), []byte("#!/bin/bash\nset -eu\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestBrowserDependencyRetriesRepairThePackageState(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the dependency installer runs on Linux; execute this integration test on its CI platform")
	}
	for _, scenario := range []struct {
		name, mode, calls string
		fails             bool
	}{
		{"healthy", "healthy", "install-deps\ninstall-browser\n", false},
		{"interrupted configuration", "interrupted", "install-deps\ndpkg\ninstall-deps\ninstall-browser\n", false},
		{"missing dependencies", "dependencies", "install-deps\ndpkg\napt-fix\ninstall-deps\ninstall-browser\n", false},
		{"failed repair", "broken", "install-deps\ndpkg\napt-fix\n", true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			directory := t.TempDir()
			writeBrowserInstallStub(t, directory, "sudo", `case "$1" in
tee) cat >/dev/null ;;
cp) ;;
timeout|dpkg|apt-get) exec "$@" ;;
*) exit 99 ;;
esac`)
			writeBrowserInstallStub(t, directory, "find", "exit 0")
			writeBrowserInstallStub(t, directory, "npx", `case "$1:$2" in
playwright:install-deps)
  printf 'install-deps\n' >>"$SKILLHUB_TEST_LOG"
  if [ "$SKILLHUB_TEST_MODE" != healthy ] && [ ! -f "$SKILLHUB_TEST_REPAIRED" ]; then
    printf 'dpkg was interrupted\n' >&2
    exit 100
  fi
  ;;
playwright:install) printf 'install-browser\n' >>"$SKILLHUB_TEST_LOG" ;;
*) exit 99 ;;
esac`)
			writeBrowserInstallStub(t, directory, "dpkg", `test "$*" = '--configure -a'
printf 'dpkg\n' >>"$SKILLHUB_TEST_LOG"
if [ "$SKILLHUB_TEST_MODE" != interrupted ]; then exit 1; fi
touch "$SKILLHUB_TEST_REPAIRED"`)
			writeBrowserInstallStub(t, directory, "apt-get", `test "$*" = '--fix-broken install --yes'
printf 'apt-fix\n' >>"$SKILLHUB_TEST_LOG"
if [ "$SKILLHUB_TEST_MODE" = broken ]; then exit 1; fi
touch "$SKILLHUB_TEST_REPAIRED"`)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, "bash", "-e", "-o", "pipefail")
			command.Stdin = strings.NewReader(browserDependencyInstallScript(t))
			command.Env = append(os.Environ(),
				"PATH="+directory+":"+os.Getenv("PATH"),
				"SKILLHUB_TEST_MODE="+scenario.mode,
				"SKILLHUB_TEST_CACHE="+filepath.Join(directory, "cache"),
				"SKILLHUB_TEST_LOG="+filepath.Join(directory, "calls"),
				"SKILLHUB_TEST_REPAIRED="+filepath.Join(directory, "repaired"),
			)
			output, runErr := command.CombinedOutput()
			if (runErr != nil) != scenario.fails {
				t.Fatalf("installer error = %v, want failure %v; output:\n%s", runErr, scenario.fails, output)
			}
			calls, err := os.ReadFile(filepath.Join(directory, "calls"))
			if err != nil {
				t.Fatal(err)
			}
			if string(calls) != scenario.calls {
				t.Fatalf("command sequence = %q, want %q; output:\n%s", calls, scenario.calls, output)
			}
		})
	}
}
