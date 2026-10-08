package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/providertest"
)

func TestADispatchedRunRecordsItsPinnedRuntimeItsAttemptAndWhoStartedIt(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-pinned-runtime")
	withProvider(t, a, pool, providertest.Plan{CreatingPolls: 1, RunningPolls: 1})

	created := f.start(t)
	waitForStatus(t, f.client, created.RunID, string(gen.RunStatusSucceeded))
	runID := mustUUID(t, created.RunID)

	var raw []byte
	if err := pool.QueryRow(context.Background(),
		"SELECT runtime_snapshot FROM run_snapshots WHERE run_id = $1", runID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Provider string `json:"provider"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil || snapshot.Provider != "fake_sandbox" {
		t.Errorf("runtime snapshot = %s, want the provider the run was pinned to", raw)
	}

	var attributed, foreign int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FILTER (WHERE t.run_attempt_id = a.id),
		       count(*) FILTER (WHERE t.run_attempt_id IS NOT NULL AND t.run_attempt_id <> a.id)
		FROM run_status_transitions t JOIN run_attempts a ON a.run_id = t.run_id
		WHERE t.run_id = $1`, runID).Scan(&attributed, &foreign); err != nil {
		t.Fatal(err)
	}
	if attributed == 0 || foreign != 0 {
		t.Errorf("transitions attributed to the attempt = %d, to another = %d; want some and none", attributed, foreign)
	}

	var byStarter int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_events
		WHERE action = 'run.create' AND resource_id = $1 AND actor_user_id = $2`,
		runID, mustUUID(t, f.userID)).Scan(&byStarter); err != nil {
		t.Fatal(err)
	}
	if byStarter != 1 {
		t.Errorf("run.create audit rows attributed to the member who started it = %d, want 1", byStarter)
	}
}

func TestAFailedRunLeavesAnOrchestratorErrorEventCarryingItsReason(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-failure-event")
	withProvider(t, a, pool, providertest.Plan{
		FinalState: run.ProviderStateCompleted, ResultStatus: "failed", ErrorClass: "execution",
	})

	created := f.start(t)
	waitForStatus(t, f.client, created.RunID, string(gen.RunStatusFailed))
	runID := mustUUID(t, created.RunID)

	var reason string
	if err := pool.QueryRow(context.Background(),
		"SELECT status_reason FROM runs WHERE id = $1", runID).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	var attempt int
	var code, message string
	if err := pool.QueryRow(context.Background(), `
		SELECT attempt, payload->>'code', payload->>'message' FROM trace_events
		WHERE run_id = $1 AND source = 'orchestrator' AND event_type = 'error'`, runID).Scan(&attempt, &code, &message); err != nil {
		t.Fatal(err)
	}
	if attempt != 1 || code != "workload_error" || message == "" || message != reason {
		t.Errorf("error event = attempt %d, code %q, message %q; want attempt 1, workload_error, the run's reason %q",
			attempt, code, message, reason)
	}
}

func TestARunBorrowingAnotherSkillsVersionOrTestCaseIsNotFoundAndNothingIsCreated(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-borrowed-target")
	hash := f.confirmPermissions(t)

	other := seedSkill(t, pool, f.workspaceID, "alice-borrowed-target-other-skill")
	borrowedVersion := seedVersion(t, pool, f.workspaceID, other, "hash-alice-borrowed-target-other")
	a.packages[borrowedVersion.PackageObjectKey] = cleanPackage(t)
	borrowedTestCase := seedTestCase(t, pool, f.workspaceID, other)

	for _, tc := range []struct {
		name, versionID, testCaseID string
	}{
		{"another skill's version", uuidText(borrowedVersion.ID), f.testCaseID},
		{"another skill's test case", f.versionID, borrowedTestCase},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, view := f.postJSON(t, "/skills/"+f.skillID+"/runs",
				`{"version_id":"`+tc.versionID+`","test_case_id":"`+tc.testCaseID+
					`","confirmed_summary_hash":"`+hash+`"}`)
			if code != http.StatusNotFound {
				t.Errorf("POST run: got %d (%s), want 404", code, view.Error)
			}
		})
	}

	var runs int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM runs WHERE workspace_id = $1", mustUUID(t, f.workspaceID)).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if runs != 0 {
		t.Errorf("%d runs were created from a borrowed target", runs)
	}
}
