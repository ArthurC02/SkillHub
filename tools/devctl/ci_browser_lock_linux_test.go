package main

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestBrowserDependencyRepairWaitsForThePreviousLockOwner(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"frontend-lock", "database-lock"} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			script := browserDependencyInstallScript(t, directory)
			lock, err := os.OpenFile(filepath.Join(directory, name), os.O_RDWR, 0o600)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Close()
			if err := syscall.FcntlFlock(lock.Fd(), syscall.F_SETLK, &syscall.Flock_t{Type: syscall.F_WRLCK}); err != nil {
				t.Fatal(err)
			}
			writeBrowserLockStubs(t, directory)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, "bash", "-e", "-o", "pipefail")
			command.Stdin = strings.NewReader(script)
			command.Env = append(os.Environ(),
				"PATH="+directory+":"+os.Getenv("PATH"),
				"SKILLHUB_TEST_CACHE="+filepath.Join(directory, "cache"),
				"SKILLHUB_TEST_LOG="+filepath.Join(directory, "calls"),
				"SKILLHUB_TEST_REPAIRED="+filepath.Join(directory, "repaired"),
				"SKILLHUB_TEST_HELD_LOCK="+lock.Name(),
				"SKILLHUB_TEST_FRONTEND_LOCK="+filepath.Join(directory, "frontend-lock"),
				"SKILLHUB_TEST_DATABASE_LOCK="+filepath.Join(directory, "database-lock"),
				"SKILLHUB_TEST_REAL_PYTHON="+python,
				"SKILLHUB_TEST_OBSERVER="+filepath.Join(directory, "observer.py"),
			)
			output, runErr, released := runBrowserLockRepair(t, command, lock)
			if runErr != nil {
				t.Fatalf("repair raced the previous lock owner: %v; output:\n%s", runErr, output)
			}
			if !released {
				t.Fatal("the previous lock owner was never observed before repair")
			}
			calls, err := os.ReadFile(filepath.Join(directory, "calls"))
			if err != nil {
				t.Fatal(err)
			}
			if string(calls) != "install-deps\nbarrier\ndpkg\ninstall-deps\ninstall-browser\n" {
				t.Fatalf("unexpected recovery sequence: %q", calls)
			}
		})
	}
}

func writeBrowserLockStubs(t *testing.T, directory string) {
	t.Helper()
	writeBrowserInstallStub(t, directory, "sudo", `case "$1" in
tee) cat >/dev/null ;;
cp) ;;
timeout|dpkg|apt-get|python3) exec "$@" ;;
*) exit 99 ;;
esac`)
	writeBrowserInstallStub(t, directory, "find", "exit 0")
	writeBrowserInstallStub(t, directory, "npx", `case "$2" in
install-deps)
  printf 'install-deps\n' >>"$SKILLHUB_TEST_LOG"
  if [ ! -f "$SKILLHUB_TEST_REPAIRED" ]; then exit 100; fi ;;
install) printf 'install-browser\n' >>"$SKILLHUB_TEST_LOG" ;;
*) exit 99 ;;
esac`)
	writeBrowserInstallStub(t, directory, "dpkg", `"$SKILLHUB_TEST_REAL_PYTHON" - <<'PY'
import fcntl, os
with open(os.environ['SKILLHUB_TEST_FRONTEND_LOCK'], 'r+') as frontend, open(os.environ['SKILLHUB_TEST_DATABASE_LOCK'], 'r+') as database:
    for lock in (frontend, database):
        fcntl.lockf(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
PY
printf 'dpkg\n' >>"$SKILLHUB_TEST_LOG"
touch "$SKILLHUB_TEST_REPAIRED"`)
	writeBrowserInstallStub(t, directory, "apt-get", "exit 100")
	writeBrowserInstallStub(t, directory, "python3", `exec "$SKILLHUB_TEST_REAL_PYTHON" "$SKILLHUB_TEST_OBSERVER"`)
	observer := `import fcntl, os, sys
original = fcntl.lockf
def observe(lock, operation):
    if lock.name == os.environ['SKILLHUB_TEST_HELD_LOCK']:
        if operation != fcntl.LOCK_EX:
            raise AssertionError('recovery must wait for the lock owner')
        with open(os.environ['SKILLHUB_TEST_LOG'], 'a') as log:
            log.write('barrier\n')
        os.write(3, b'waiting\n')
        if os.read(4, 1) != b'x':
            raise AssertionError('the lock owner did not release its lock')
    return original(lock, operation)
fcntl.lockf = observe
exec(compile(sys.stdin.read(), '<workflow>', 'exec'))
`
	if err := os.WriteFile(filepath.Join(directory, "observer.py"), []byte(observer), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runBrowserLockRepair(t *testing.T, command *exec.Cmd, lock *os.File) ([]byte, error, bool) {
	t.Helper()
	signalRead, signalWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer signalRead.Close()
	defer signalWrite.Close()
	ackRead, ackWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer ackRead.Close()
	defer ackWrite.Close()
	command.ExtraFiles = []*os.File{signalWrite, ackRead}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	signalWrite.Close()
	ackRead.Close()
	released := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(signalRead)
		observed := false
		for scanner.Scan() {
			if scanner.Text() == "waiting" {
				observed = lock.Close() == nil
				if _, err := ackWrite.Write([]byte("x")); err != nil {
					observed = false
				}
			}
		}
		released <- observed && scanner.Err() == nil
	}()
	err = command.Wait()
	return output.Bytes(), err, <-released
}
