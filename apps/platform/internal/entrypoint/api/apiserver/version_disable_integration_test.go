package apiserver_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func versionDisableCount(t *testing.T, versionID string) int {
	t.Helper()
	var count int
	if err := requireDB(t).QueryRow(t.Context(),
		`SELECT count(*) FROM skill_version_disables WHERE skill_version_id = $1`, mustUUID(t, versionID),
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func versionDisableAuditCount(t *testing.T, versionID, action string) int {
	t.Helper()
	var count int
	if err := requireDB(t).QueryRow(t.Context(),
		`SELECT count(*) FROM audit_events WHERE resource_id = $1 AND action = $2`,
		mustUUID(t, versionID), action,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestOperatorVersionDisableUsesExactMinimalStatusAndAuditsRepeats(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "version-disable-operator")
	operator := a.login(t, "version-disable-admin")
	a.auth.Operators = map[string]bool{operator.userID: true}
	path := "/admin/versions/" + f.versionID
	assertVersionOperatorOnly(t, a, f.client, path, f.versionID)
	assertVersionStatusAndReasonValidation(t, operator, path, f.versionID)
	code, after := operatorCall(t, operator, http.MethodPut, path+"/disable", `{"reason":"reviewed incident"}`)
	if code != http.StatusOK || len(after) != 3 || after["version_id"] != f.versionID || after["disabled"] != true {
		t.Fatalf("disable = %d %v, want only version ID, number and disabled=true", code, after)
	}
	assertVersionDisableAudit(t, pool, f.versionID, operator.userID)
	code, repeated := operatorCall(t, operator, http.MethodPut, path+"/disable", `{"reason":"same request repeated"}`)
	if code != http.StatusConflict || !strings.Contains(fmt.Sprint(repeated), "已停用") ||
		versionDisableCount(t, f.versionID) != 1 || versionDisableAuditCount(t, f.versionID, "skill.version_disable_attempt") != 1 {
		t.Fatalf("repeat = %d %v, want already-disabled with one attempt audit", code, repeated)
	}
	assertVersionDisableAttemptAudit(t, pool, f.versionID, operator.userID)
	if code, _ := operatorCall(t, operator, http.MethodPost, path+"/enable", ""); code != http.StatusNotFound {
		t.Errorf("restore route: got %d, want 404", code)
	}
}

func assertVersionStatusAndReasonValidation(t *testing.T, operator *client, path, versionID string) {
	t.Helper()
	code, before := operatorCall(t, operator, http.MethodGet, path, "")
	if code != http.StatusOK || len(before) != 3 || before["version_id"] != versionID || before["disabled"] != false || before["version_number"] != float64(1) {
		t.Fatalf("exact-ID status = %d %v, want only version ID, number 1 and disabled=false", code, before)
	}
	for _, body := range []string{`{}`, `{"reason":"  "}`, `{"reason":"` + strings.Repeat("x", 1001) + `"}`} {
		if code, _ := operatorCall(t, operator, http.MethodPut, path+"/disable", body); code != http.StatusBadRequest {
			t.Errorf("reason %s: got %d, want 400", body[:min(len(body), 24)], code)
		}
	}
	if versionDisableCount(t, versionID) != 0 {
		t.Fatal("an invalid reason changed the disable state")
	}
}

func assertVersionOperatorOnly(t *testing.T, a *api, member *client, path, versionID string) {
	t.Helper()

	for name, c := range map[string]*client{
		"anonymous": {Client: http.DefaultClient, base: a.URL},
		"member":    member,
	} {
		for _, method := range []string{http.MethodGet, http.MethodPut} {
			target := path
			if method == http.MethodPut {
				target += "/disable"
			}
			code, _ := operatorCall(t, c, method, target, `{"reason":"reviewed incident"}`)
			if code != http.StatusNotFound {
				t.Errorf("%s %s: got %d, want 404", name, method, code)
			}
		}
	}
	if versionDisableCount(t, versionID) != 0 {
		t.Fatal("a caller without the operator role disabled the version")
	}
}

func assertVersionDisableAudit(t *testing.T, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, versionID, actorID string) {
	t.Helper()
	if versionDisableCount(t, versionID) != 1 || versionDisableAuditCount(t, versionID, "skill.version_disable") != 1 {
		t.Fatal("the disable state and its audit did not commit together")
	}
	var actor, reason, beforeValue, afterValue string
	if err := pool.QueryRow(t.Context(), `
		SELECT actor_user_id::text, metadata->>'reason', metadata->>'previous_value', metadata->>'new_value'
		FROM audit_events WHERE resource_id = $1 AND action = 'skill.version_disable'`,
		mustUUID(t, versionID),
	).Scan(&actor, &reason, &beforeValue, &afterValue); err != nil {
		t.Fatal(err)
	}
	if actor != actorID || reason != "reviewed incident" || beforeValue != "false" || afterValue != "true" {
		t.Fatalf("audit = (%q,%q,%q,%q), want actor, reason, false, true", actor, reason, beforeValue, afterValue)
	}
}

func assertVersionDisableAttemptAudit(t *testing.T, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, versionID, actorID string) {
	t.Helper()
	var actor, reason, previous, next string
	if err := pool.QueryRow(t.Context(), `
		SELECT actor_user_id::text, metadata->>'reason', metadata->>'previous_value', metadata->>'new_value'
		FROM audit_events WHERE resource_id = $1 AND action = 'skill.version_disable_attempt'`,
		mustUUID(t, versionID),
	).Scan(&actor, &reason, &previous, &next); err != nil {
		t.Fatal(err)
	}
	if actor != actorID || reason != "same request repeated" || previous != "true" || next != "true" {
		t.Fatalf("repeat audit = (%q,%q,%q,%q), want actor, reason, true, true", actor, reason, previous, next)
	}
}

func TestDisablingVersionRefusesNewRunsWithoutChangingExistingRuns(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "version-disable-run")
	hash := f.confirmPermissions(t)
	operator := a.login(t, "version-disable-run-admin")
	a.auth.Operators = map[string]bool{operator.userID: true}
	code, _ := operatorCall(t, operator, http.MethodPut, "/admin/versions/"+f.versionID+"/disable", `{"reason":"unsafe for new use"}`)
	if code != http.StatusOK {
		t.Fatalf("disable: got %d, want 200", code)
	}
	if code, view := f.preflight(t); code != http.StatusOK || view.Blocked != run.ReasonVersionDisabled {
		t.Fatalf("preflight = %d blocked=%q, want version_disabled", code, view.Blocked)
	}
	if code, view := f.startWithHash(t, hash); code != http.StatusUnprocessableEntity || !strings.Contains(view.Error, "停用") {
		t.Fatalf("new Run = %d %q, want 422 naming disabled", code, view.Error)
	}
	var count int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM runs WHERE skill_version_id = $1`, mustUUID(t, f.versionID)).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("disabled version has %d new Runs, want zero", count)
	}
	second := seedVersion(t, pool, f.workspaceID, f.skillID, "second-version-after-disable")
	a.packages[second.PackageObjectKey] = cleanPackage(t)
	f.versionID = uuidText(second.ID)
	if code, _ := f.startWithHash(t, f.confirmPermissions(t)); code != http.StatusCreated {
		t.Fatalf("another version: got %d, want 201", code)
	}
}

func TestVersionDisablePreservesExistingRunVersionAndQueuedJob(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	history := newFixture(t, a, pool, "version-disable-history")
	previous := history.start(t)
	operator := a.login(t, "version-disable-history-admin")
	a.auth.Operators = map[string]bool{operator.userID: true}
	var jobID int64
	if err := pool.QueryRow(t.Context(), `
		INSERT INTO river_job (state, kind, args, scheduled_at)
		VALUES ('scheduled', 'evaluate_run', jsonb_build_object('run_id', $1::text), now() + interval '1 day')
		RETURNING id`, previous.RunID).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	var beforeHash, beforeStatus string
	if err := pool.QueryRow(t.Context(), `SELECT content_hash FROM skill_versions WHERE id = $1`, mustUUID(t, history.versionID)).Scan(&beforeHash); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT status FROM runs WHERE id = $1`, mustUUID(t, previous.RunID)).Scan(&beforeStatus); err != nil {
		t.Fatal(err)
	}
	if code, _ := operatorCall(t, operator, http.MethodPut, "/admin/versions/"+history.versionID+"/disable", `{"reason":"new starts stopped"}`); code != http.StatusOK {
		t.Fatalf("disable after existing Run: got %d", code)
	}
	var afterHash, afterStatus string
	if err := pool.QueryRow(t.Context(), `SELECT content_hash FROM skill_versions WHERE id = $1`, mustUUID(t, history.versionID)).Scan(&afterHash); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT status FROM runs WHERE id = $1`, mustUUID(t, previous.RunID)).Scan(&afterStatus); err != nil {
		t.Fatal(err)
	}
	if afterHash != beforeHash || afterStatus != beforeStatus {
		t.Fatalf("history changed: hash %q→%q, Run %q→%q", beforeHash, afterHash, beforeStatus, afterStatus)
	}
	var jobState, jobRunID string
	if err := pool.QueryRow(t.Context(), `SELECT state, args->>'run_id' FROM river_job WHERE id = $1`, jobID).Scan(&jobState, &jobRunID); err != nil {
		t.Fatal(err)
	}
	if jobState != "scheduled" || jobRunID != previous.RunID {
		t.Fatalf("historical job changed: state=%q run=%q", jobState, jobRunID)
	}
}

func TestVersionDisableAuditFailureRollsBackState(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "version-disable-audit")
	operator := a.login(t, "version-disable-audit-admin")
	a.auth.Operators = map[string]bool{operator.userID: true}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TRIGGER IF EXISTS reject_version_disable_audit ON audit_events`)
		_, _ = pool.Exec(context.Background(), `DROP FUNCTION IF EXISTS reject_version_disable_audit()`)
	})
	for _, statement := range []string{
		`CREATE FUNCTION reject_version_disable_audit() RETURNS trigger LANGUAGE plpgsql AS $$
         BEGIN IF NEW.action = 'skill.version_disable' THEN RAISE EXCEPTION 'audit refused'; END IF; RETURN NEW; END $$`,
		`CREATE TRIGGER reject_version_disable_audit BEFORE INSERT ON audit_events
         FOR EACH ROW EXECUTE FUNCTION reject_version_disable_audit()`,
	} {
		if _, err := pool.Exec(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	if code, _ := operatorCall(t, operator, http.MethodPut, "/admin/versions/"+f.versionID+"/disable", `{"reason":"must audit"}`); code != http.StatusInternalServerError {
		t.Fatalf("audit failure: got %d, want 500", code)
	}
	if versionDisableCount(t, f.versionID) != 0 || versionDisableAuditCount(t, f.versionID, "skill.version_disable") != 0 {
		t.Fatal("audit failure committed a disable without its event")
	}
}

type failingVersionAdmission struct{}

func (failingVersionAdmission) Disabled(context.Context, pgtype.UUID, pgtype.UUID) (bool, error) {
	return false, errors.New("owner read failed")
}

func (failingVersionAdmission) Admit(context.Context, pgx.Tx, pgtype.UUID, pgtype.UUID) error {
	return errors.New("owner read failed")
}

func TestVersionAdmissionReadFailureFailsClosed(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "version-disable-owner-failure")
	hash := f.confirmPermissions(t)
	a.runs.VersionAdmission = failingVersionAdmission{}
	if code, _ := f.preflight(t); code != http.StatusInternalServerError {
		t.Fatalf("preflight owner read failure: got %d, want 500", code)
	}
	if code, _ := f.startWithHash(t, hash); code != http.StatusInternalServerError {
		t.Fatalf("Run owner read failure: got %d, want 500", code)
	}
	var count int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM runs WHERE skill_version_id = $1`, mustUUID(t, f.versionID)).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("owner failure created %d Runs, want zero", count)
	}
}

func TestRunCreationWaitsForVersionDisableCommit(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "version-disable-race")
	hash := f.confirmPermissions(t)
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(t.Context(), `SELECT id FROM skill_versions WHERE id = $1 FOR UPDATE`, mustUUID(t, f.versionID)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `INSERT INTO skill_version_disables (skill_version_id) VALUES ($1)`, mustUUID(t, f.versionID)); err != nil {
		t.Fatal(err)
	}
	result := make(chan int, 1)
	go func() {
		body := fmt.Sprintf(`{"version_id":%q,"test_case_id":%q,"confirmed_summary_hash":%q}`, f.versionID, f.testCaseID, hash)
		resp, err := f.Post(f.base+"/skills/"+f.skillID+"/runs", "application/json", strings.NewReader(body))
		if err != nil {
			result <- 0
			return
		}
		defer resp.Body.Close()
		result <- resp.StatusCode
	}()
	waitForVersionAdmissionLock(t, pool, result)
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-result:
		if code != http.StatusUnprocessableEntity {
			t.Fatalf("Run after disable commit: got %d, want 422", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not finish after the disable committed")
	}
	var count int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM runs WHERE skill_version_id = $1`, mustUUID(t, f.versionID)).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("serialized disable still allowed %d new Runs", count)
	}
}

func waitForVersionAdmissionLock(t *testing.T, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, result <-chan int) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case code := <-result:
			t.Fatalf("Run finished with %d before the competing disable committed", code)
		case <-deadline:
			t.Fatal("Run never reached the version-row lock")
		default:
		}
		var waiting int
		if err := pool.QueryRow(t.Context(), `
			SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'
			  AND query LIKE '%skill_versions%'`,
		).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}
