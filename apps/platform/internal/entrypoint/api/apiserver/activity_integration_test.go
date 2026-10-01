package apiserver_test

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/product/activity"
)

func TestWorkspaceActivityReturnsACompleteEmptyPage(t *testing.T) {
	a := newAPI(t, requireDB(t))
	alice := a.login(t, "activity-empty")

	code, body := alice.doJSON(t, http.MethodGet, "/me/activity", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d, body = %v", code, body)
	}
	if complete, _ := body["complete"].(bool); !complete {
		t.Fatalf("complete = %v", body["complete"])
	}
	if items, ok := body["items"].([]any); !ok || len(items) != 0 {
		t.Fatalf("items = %#v", body["items"])
	}
	if sources, ok := body["sources"].([]any); !ok || len(sources) != 5 {
		t.Fatalf("sources = %#v", body["sources"])
	}
}

func TestACreationWaitingOnThePersonIsSummarisedByWhatItAwaits(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "activity-creation-pending")
	mustExec(t, pool, `
		INSERT INTO creation_sessions (id, workspace_id, state, revision, snapshot, expires_at)
		VALUES (gen_random_uuid(), $1, 'waiting_confirmation', 1,
		        '{"snapshot":{"pending_action":"confirm_brief","messages":[{"role":"user","content":"整理收支"}]}}',
		        now() + interval '1 day')`, mustUUID(t, alice.workspaceID))

	code, body := alice.doJSON(t, http.MethodGet, "/me/activity", "")
	items, _ := body["items"].([]any)
	if code != http.StatusOK || len(items) != 1 {
		t.Fatalf("status = %d, items = %#v", code, body["items"])
	}
	if summary := items[0].(map[string]any)["summary"]; summary != "確認 Skill 需求" {
		t.Errorf("summary = %v, want the brief confirmation it is waiting on", summary)
	}
}

func TestWorkspaceActivityScopesEveryOwnerReaderToTheSessionWorkspace(t *testing.T) {
	a := newAPI(t, requireDB(t))
	alice := a.login(t, "activity-scope-alice")
	bob := a.login(t, "activity-scope-bob")
	aliceWorkspace := mustUUID(t, alice.workspaceID)
	seen := map[activity.Source][]pgtype.UUID{}
	record := func(source activity.Source, workspaceID pgtype.UUID) bool {
		seen[source] = append(seen[source], workspaceID)
		return workspaceID == aliceWorkspace
	}
	a.app.ActivitySvc.ReadRuns = func(_ context.Context, workspaceID pgtype.UUID) ([]activity.RunFact, error) {
		if record(activity.SourceRun, workspaceID) {
			return []activity.RunFact{{RunID: "alice-run", SkillName: "Alice", Classification: activity.Recent}}, nil
		}
		return nil, nil
	}
	a.app.ActivitySvc.ReadEvaluations = func(_ context.Context, workspaceID pgtype.UUID) ([]activity.EvaluationFact, error) {
		if record(activity.SourceEvaluation, workspaceID) {
			return []activity.EvaluationFact{{RunID: "alice-run", Classification: activity.Recent}}, nil
		}
		return nil, nil
	}
	a.app.ActivitySvc.ReadCreations = func(_ context.Context, workspaceID pgtype.UUID) ([]activity.CreationFact, error) {
		if record(activity.SourceCreation, workspaceID) {
			return []activity.CreationFact{{SessionID: "alice-creation", Classification: activity.Recent}}, nil
		}
		return nil, nil
	}
	a.app.ActivitySvc.ReadPackaging = func(_ context.Context, workspaceID pgtype.UUID) ([]activity.PackagingFact, error) {
		if record(activity.SourcePackaging, workspaceID) {
			return []activity.PackagingFact{{ArtifactID: "alice-artifact", Classification: activity.Recent}}, nil
		}
		return nil, nil
	}
	a.app.ActivitySvc.ReadPublishing = func(_ context.Context, workspaceID pgtype.UUID) ([]activity.PublishingFact, error) {
		if record(activity.SourcePublishing, workspaceID) {
			return []activity.PublishingFact{{PublicationID: "alice-publication", Classification: activity.Recent}}, nil
		}
		return nil, nil
	}

	_, aliceBody := alice.doJSON(t, http.MethodGet, "/me/activity", "")
	_, bobBody := bob.doJSON(t, http.MethodGet, "/me/activity", "")
	if items := aliceBody["items"].([]any); len(items) != 4 {
		t.Fatalf("alice items = %d, want 4", len(items))
	}
	if items := bobBody["items"].([]any); len(items) != 0 {
		t.Fatalf("bob saw %d foreign items", len(items))
	}
	want := []pgtype.UUID{aliceWorkspace, mustUUID(t, bob.workspaceID)}
	for _, source := range activity.Sources {
		if !reflect.DeepEqual(seen[source], want) {
			t.Errorf("%s workspaces = %v, want %v", source, seen[source], want)
		}
	}
}

func TestWorkspaceActivityFailsClosedWhenAnOwnerReaderFails(t *testing.T) {
	a := newAPI(t, requireDB(t))
	alice := a.login(t, "activity-failure")
	a.app.ActivitySvc.ReadPublishing = func(context.Context, pgtype.UUID) ([]activity.PublishingFact, error) {
		return nil, errors.New("database unavailable")
	}

	code, body := alice.doJSON(t, http.MethodGet, "/me/activity", "")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %v", code, body)
	}
	if complete, _ := body["complete"].(bool); complete {
		t.Fatalf("complete = %v", body["complete"])
	}
	if _, exists := body["items"]; exists {
		t.Fatalf("partial items escaped: %v", body["items"])
	}
	sources, ok := body["unavailable_sources"].([]any)
	if !ok || len(sources) != 1 || sources[0] != "publishing" {
		t.Fatalf("unavailable_sources = %#v", body["unavailable_sources"])
	}
}

func TestWorkspaceActivityRejectsForeignCursorAndOutOfRangeLimit(t *testing.T) {
	a := newAPI(t, requireDB(t))
	alice := a.login(t, "activity-input")
	for _, path := range []string{"/me/activity?cursor=foreign", "/me/activity?limit=0", "/me/activity?limit=101"} {
		if code, body := alice.doJSON(t, http.MethodGet, path, ""); code != http.StatusBadRequest {
			t.Errorf("GET %s: status = %d, body = %v", path, code, body)
		}
	}
}
