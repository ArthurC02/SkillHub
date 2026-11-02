package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const fakePsql = `#!/usr/bin/env bash
cat >"$FAKE_PSQL_STDIN"
printf '%s\n' "$@" >"$FAKE_PSQL_ARGS"
if [ -n "${FAKE_PSQL_UNREACHABLE:-}" ]; then
	exit 2
fi
while IFS= read -r line; do
	case "$line" in
	"SELECT 'apply-sql: "*) marker="${line#SELECT \'}"; echo " ${marker%\';} " ;;
	*BREAKS*) exit 3 ;;
	esac
done <"$FAKE_PSQL_STDIN"
`

type applySQLRun struct {
	exit        int
	stderr      string
	stdin, args string
	ranPsql     bool
	paths       []string
}

type sqlFile struct{ name, body string }

func runApplySQL(t *testing.T, layout []string, files []sqlFile, env ...string) applySQLRun {
	t.Helper()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatal("bash is required to run tools/ci/apply-sql.sh")
	}
	work := t.TempDir()
	var paths []string
	for _, file := range files {
		path := filepath.ToSlash(filepath.Join(work, file.name))
		if err := os.WriteFile(path, []byte(file.body), 0o600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	fake := filepath.ToSlash(filepath.Join(work, "fake-psql.sh"))
	if err := os.WriteFile(fake, []byte(fakePsql), 0o600); err != nil {
		t.Fatal(err)
	}
	stdinRecord := filepath.ToSlash(filepath.Join(work, "stdin"))
	argsRecord := filepath.ToSlash(filepath.Join(work, "args"))
	args := []string{filepath.ToSlash(filepath.Join(root, "tools", "ci", "apply-sql.sh"))}
	for _, item := range layout {
		switch item {
		case "FILES":
			args = append(args, paths...)
		case "PSQL":
			args = append(args, "bash", fake)
		default:
			args = append(args, item)
		}
	}
	command := exec.Command(bash, args...)
	command.Env = append(os.Environ(), append([]string{"BASH_ENV=", "FAKE_PSQL_STDIN=" + stdinRecord, "FAKE_PSQL_ARGS=" + argsRecord}, env...)...)
	var stderr strings.Builder
	command.Stderr = &stderr
	run := applySQLRun{paths: paths}
	if err := command.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatal(err)
		}
		run.exit = exitErr.ExitCode()
	}
	run.stderr = strings.ReplaceAll(stderr.String(), "\r\n", "\n")
	if data, err := os.ReadFile(stdinRecord); err == nil {
		run.ranPsql = true
		run.stdin = string(data)
	}
	if data, err := os.ReadFile(argsRecord); err == nil {
		run.args = string(data)
	}
	return run
}

func TestApplySQLRunsEachFileInItsOwnTransactionInOrderThroughOneSession(t *testing.T) {
	t.Parallel()
	run := runApplySQL(t, []string{"FILES", "--", "PSQL", "postgres://db"},
		[]sqlFile{{"0001_a.sql", "CREATE TABLE a();\n"}, {"0002_b.sql", "CREATE TABLE b();\n"}})
	if run.exit != 0 {
		t.Fatalf("exit %d: %s", run.exit, run.stderr)
	}
	if run.args != "postgres://db\n-q\n-v\nON_ERROR_STOP=1\n" {
		t.Errorf("psql args = %q", run.args)
	}
	want := "SELECT 'apply-sql: 0';\nBEGIN;\nCREATE TABLE a();\n\nCOMMIT;\n" +
		"SELECT 'apply-sql: 1';\nBEGIN;\nCREATE TABLE b();\n\nCOMMIT;\n"
	if run.stdin != want {
		t.Errorf("psql stdin = %q, want %q", run.stdin, want)
	}
}

func TestApplySQLEndsAFileWithoutAFinalNewlineBeforeItsCommit(t *testing.T) {
	t.Parallel()
	run := runApplySQL(t, []string{"FILES", "--", "PSQL"}, []sqlFile{{"0001_a.sql", "SELECT 1;"}})
	if run.exit != 0 || !strings.HasSuffix(run.stdin, "SELECT 1;\nCOMMIT;\n") {
		t.Errorf("exit %d, stdin %q", run.exit, run.stdin)
	}
}

func TestApplySQLKeepsFilePathsOutOfPsqlWhereABackslashCouldReadAsACommand(t *testing.T) {
	t.Parallel()
	run := runApplySQL(t, []string{"FILES", "--", "PSQL"}, []sqlFile{{"0001_a.sql", "SELECT 1;\n"}})
	if run.exit != 0 || strings.Contains(run.stdin, run.paths[0]) {
		t.Errorf("exit %d, stdin %q carries %q", run.exit, run.stdin, run.paths[0])
	}
}

func TestApplySQLNamesTheFailingFile(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		files  []sqlFile
		failed int
	}{
		"first file": {[]sqlFile{{"0001_a.sql", "BREAKS;\n"}, {"0002_b.sql", "SELECT 2;\n"}}, 0},
		"later file": {[]sqlFile{{"0001_a.sql", "SELECT 1;\n"}, {"0002_b.sql", "BREAKS;\n"}, {"0003_c.sql", "SELECT 3;\n"}}, 1},
	}
	for name, tc := range cases {
		run := runApplySQL(t, []string{"FILES", "--", "PSQL"}, tc.files)
		want := "::error file=" + run.paths[tc.failed] + "::" + tc.files[tc.failed].name + " failed to apply\n"
		if run.exit != 1 || run.stderr != want {
			t.Errorf("%s: exit %d, stderr %q, want %q", name, run.exit, run.stderr, want)
		}
	}
}

func TestApplySQLBlamesNoFileWhenTheSessionFailsBeforeAnyRan(t *testing.T) {
	t.Parallel()
	run := runApplySQL(t, []string{"FILES", "--", "PSQL"}, []sqlFile{{"0001_a.sql", "SELECT 1;\n"}}, "FAKE_PSQL_UNREACHABLE=1")
	if run.exit != 1 || run.stderr != "::error file=unknown::unknown failed to apply\n" {
		t.Errorf("exit %d, stderr %q", run.exit, run.stderr)
	}
}

func TestApplySQLRefusesAMissingFileListSeparatorOrCommandWithoutRunningPsql(t *testing.T) {
	t.Parallel()
	for name, layout := range map[string][]string{
		"no files":     {"--", "PSQL"},
		"no separator": {"FILES", "PSQL"},
		"no command":   {"FILES", "--"},
	} {
		run := runApplySQL(t, layout, []sqlFile{{"0001_a.sql", "SELECT 1;\n"}})
		if run.exit != 2 || !strings.HasPrefix(run.stderr, "usage: apply-sql.sh") || run.ranPsql {
			t.Errorf("%s: exit %d, stderr %q, psql ran %v", name, run.exit, run.stderr, run.ranPsql)
		}
	}
}
