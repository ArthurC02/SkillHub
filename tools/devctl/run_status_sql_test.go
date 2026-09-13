package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestTheRealRepositoryHasNoRunStatusSQLDrift(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := runStatusSQLProblems(root); len(problems) > 0 {
		t.Fatalf("the Go table and its SQL copies drifted:\n%s", strings.Join(problems, "\n"))
	}

	successors, err := goIdentListMap(
		filepath.Join(root, filepath.FromSlash(runStateMachineFile)), "successors",
		filepath.Join(root, filepath.FromSlash(runStatusConstantFile)), "RunStatus")
	if err != nil {
		t.Fatal(err)
	}
	if len(successors) < 5 {
		t.Fatalf("the Go table has %d ongoing statuses; a run passes through more than that, so the comparison above compared too little", len(successors))
	}
	if !successors["queued"]["provisioning"] || !successors["evaluating"]["succeeded"] {
		t.Fatalf("the table reads as %v, which no longer looks like the run lifecycle", successors)
	}

	known := map[string]bool{}
	for status := range successors {
		known[status] = true
	}
	for _, status := range []string{"succeeded", "failed", "cancelled", "timed_out"} {
		known[status] = true
	}
	wrong := map[string]bool{"succeeded": true, "failed": true, "cancelled": true}
	if problems := runTerminalListProblems(root, known, wrong); len(problems) == 0 {
		t.Fatal("a deliberately wrong terminal set drew no complaint, so the scan is visiting no SQL at all")
	}
}

func TestRunStatusSQLNamesATriggerRowThatDisagrees(t *testing.T) {
	t.Parallel()
	root := runStatusFixture(t, `CREATE FUNCTION enforce_run_status_transition() RETURNS trigger AS $$
BEGIN
    IF NOT (CASE OLD.status
        WHEN 'queued' THEN NEW.status IN ('running', 'failed', 'timed_out')
        WHEN 'running' THEN NEW.status IN ('succeeded', 'failed', 'timed_out')
        ELSE false
    END) THEN
        RAISE EXCEPTION 'no';
    END IF;
    RETURN NEW;
END;
$$;
`)
	problems := runStatusSQLProblems(root)
	if len(problems) != 2 {
		t.Fatalf("expected the two statuses the trigger row and the Go table disagree on, got %d:\n%s", len(problems), strings.Join(problems, "\n"))
	}
	joined := strings.Join(problems, "\n")
	if !strings.Contains(joined, `"cancelled"`) || !strings.Contains(joined, `successors of "queued"`) {
		t.Fatalf("the report must name the status and the row it belongs to:\n%s", joined)
	}
}

func TestRunStatusSQLNamesAStatusTheTriggerNeverLetsMove(t *testing.T) {
	t.Parallel()
	root := runStatusFixture(t, `CREATE FUNCTION enforce_run_status_transition() RETURNS trigger AS $$
BEGIN
    IF NOT (CASE OLD.status
        WHEN 'queued' THEN NEW.status IN ('running', 'failed', 'cancelled', 'timed_out')
        ELSE false
    END) THEN
        RAISE EXCEPTION 'no';
    END IF;
    RETURN NEW;
END;
$$;
`)
	problems := runStatusSQLProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], `lets "running" move on`) {
		t.Fatalf("a status the Go table advances but the trigger has no row for must be reported:\n%s", strings.Join(problems, "\n"))
	}
}

func TestRunStatusSQLNamesATerminalListThatDisagrees(t *testing.T) {
	t.Parallel()
	root := runStatusFixture(t, `CREATE FUNCTION enforce_run_status_transition() RETURNS trigger AS $$
BEGIN
    IF NOT (CASE OLD.status
        WHEN 'queued' THEN NEW.status IN ('running', 'failed', 'cancelled', 'timed_out')
        WHEN 'running' THEN NEW.status IN ('succeeded', 'failed', 'cancelled', 'timed_out')
        ELSE false
    END) THEN
        RAISE EXCEPTION 'no';
    END IF;
    RETURN NEW;
END;
$$;
`)
	writeAt(t, root, "db/queries/runs.sql", `-- name: ActiveRuns :many
SELECT * FROM runs WHERE status NOT IN ('succeeded', 'cancelled', 'timed_out');
`)
	problems := runStatusSQLProblems(root)
	if len(problems) != 1 {
		t.Fatalf("expected the one terminal status the query omits, got %d:\n%s", len(problems), strings.Join(problems, "\n"))
	}
	if !strings.Contains(problems[0], `"failed"`) || !strings.Contains(problems[0], "db/queries/runs.sql:2") {
		t.Fatalf("the report must name the status and the line that omits it: %s", problems[0])
	}
}

func TestRunStatusSQLLeavesOtherVocabulariesAlone(t *testing.T) {
	t.Parallel()
	root := runStatusFixture(t, `CREATE FUNCTION enforce_run_status_transition() RETURNS trigger AS $$
BEGIN
    IF NOT (CASE OLD.status
        WHEN 'queued' THEN NEW.status IN ('running', 'failed', 'cancelled', 'timed_out')
        WHEN 'running' THEN NEW.status IN ('succeeded', 'failed', 'cancelled', 'timed_out')
        ELSE false
    END) THEN
        RAISE EXCEPTION 'no';
    END IF;
    RETURN NEW;
END;
$$;
`)
	writeAt(t, root, "db/queries/creation.sql", `-- name: StalledSessions :many
SELECT * FROM creation_sessions WHERE state IN ('working', 'queued');

-- name: OpenEvaluations :many
SELECT * FROM evaluations WHERE status IN ('pending', 'completed', 'failed');
`)
	if problems := runStatusSQLProblems(root); len(problems) != 0 {
		t.Fatalf("lists belonging to other vocabularies must not be judged against the run lifecycle:\n%s", strings.Join(problems, "\n"))
	}
}

func runStatusFixture(t *testing.T, trigger string) string {
	t.Helper()
	root := t.TempDir()
	writeAt(t, root, runStateMachineFile, `package run

import "github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"

var successors = map[gen.RunStatus][]gen.RunStatus{
	gen.RunStatusQueued:  {gen.RunStatusRunning, gen.RunStatusFailed, gen.RunStatusCancelled, gen.RunStatusTimedOut},
	gen.RunStatusRunning: {gen.RunStatusSucceeded, gen.RunStatusFailed, gen.RunStatusCancelled, gen.RunStatusTimedOut},
}

var AllStatuses = []gen.RunStatus{
	gen.RunStatusQueued,
	gen.RunStatusRunning,
	gen.RunStatusSucceeded,
	gen.RunStatusFailed,
	gen.RunStatusCancelled,
	gen.RunStatusTimedOut,
}
`)
	writeAt(t, root, runStatusConstantFile, `package gen

type RunStatus string

const (
	RunStatusQueued    RunStatus = "queued"
	RunStatusRunning   RunStatus = "running"
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCancelled RunStatus = "cancelled"
	RunStatusTimedOut  RunStatus = "timed_out"
)
`)
	writeAt(t, root, runTransitionTrigger, trigger)
	return root
}
