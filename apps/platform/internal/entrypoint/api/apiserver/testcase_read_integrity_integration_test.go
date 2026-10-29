package apiserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

type testCaseProjectionScenario struct {
	name      string
	criteria  string
	rubric    any
	readable  bool
	count     float64
	hasRubric bool
}

func TestUnreadableTestCaseDesignIsNotReportedAsAbsent(t *testing.T) {
	for _, scenario := range []testCaseProjectionScenario{
		{"unreadable criteria", `{"private":"projection-private-canary"}`, nil, false, 0, false},
		{"null criteria", `null`, nil, false, 0, false},
		{"unreadable rubric", `[]`, `{"version":"v1","items":[{"id":"c1","text":"projection-private-canary","evidence_required":"projection-private-canary"}]}`, false, 0, false},
		{"absent rubric", `[]`, nil, true, 0, false},
		{"JSON null rubric", `[]`, `null`, true, 0, false},
		{"populated design", `[{"id":"c1","text":"complete","source":"user","confirmed_at":"2026-01-01T00:00:00Z"}]`, `{"version":"v1","items":[{"id":"c1","text":"complete","evidence_required":true}]}`, true, 1, true},
	} {
		for _, route := range []string{"detail", "list"} {
			t.Run(scenario.name+"/"+route, func(t *testing.T) {
				assertStoredTestCaseProjection(t, scenario, route)
			})
		}
	}
}

func assertStoredTestCaseProjection(t *testing.T, scenario testCaseProjectionScenario, route string) {
	t.Helper()
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, freshName("projection"))
	skillID, id := newTestCase(t, pool, a, owner, "read projection")
	if _, err := pool.Exec(t.Context(), `UPDATE test_cases SET acceptance_criteria = $1::jsonb, rubric = $2::jsonb WHERE id = $3`, scenario.criteria, scenario.rubric, id); err != nil {
		t.Fatalf("store fixture under current constraints: %v", err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `UPDATE test_cases SET acceptance_criteria = '[]'::jsonb, rubric = NULL WHERE id = $1`, id); err != nil {
			t.Error(err)
		}
	})
	path := "/test-cases/" + id
	message := "讀取失敗"
	if route == "list" {
		path = "/test-cases?skill_id=" + skillID
		message = "讀取清單失敗"
		if !scenario.readable {
			status, _ := owner.doJSON(t, http.MethodPost, "/test-cases", fmt.Sprintf(`{"skill_id":%q,"name":"healthy sibling","user_prompt":"complete"}`, skillID))
			if status != http.StatusCreated {
				t.Fatalf("create healthy sibling: %d", status)
			}
		}
	}
	status, body := owner.doJSON(t, http.MethodGet, path, "")
	assertTestCaseProjectionLog(t, logs.Bytes(), id, scenario)
	if !scenario.readable {
		if status != http.StatusInternalServerError || body["error"] != message || len(body) != 1 {
			t.Fatalf("unreadable design: status=%d body=%v, want a generic500 without design claims", status, body)
		}
		if strings.Contains(fmt.Sprint(body), "projection-private-canary") {
			t.Fatal("stored private content reached the error response")
		}
		var unchanged bool
		if err := pool.QueryRow(t.Context(), `SELECT acceptance_criteria = $1::jsonb AND rubric IS NOT DISTINCT FROM $2::jsonb FROM test_cases WHERE id = $3`, scenario.criteria, scenario.rubric, id).Scan(&unchanged); err != nil || !unchanged {
			t.Fatalf("read changed the stored design: unchanged=%v err=%v", unchanged, err)
		}
		return
	}
	assertReadableTestCaseProjection(t, scenario, route, status, body)
}

func assertTestCaseProjectionLog(t *testing.T, data []byte, id string, scenario testCaseProjectionScenario) {
	t.Helper()
	if bytes.Contains(data, []byte("projection-private-canary")) {
		t.Fatal("stored private content reached logs")
	}
	var projections []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatal(err)
		}
		if entry["msg"] == "test case design unreadable" {
			projections = append(projections, entry)
		}
	}
	if scenario.readable {
		if len(projections) != 0 {
			t.Fatalf("readable design produced diagnostics: %v", projections)
		}
		return
	}
	field := "acceptance_criteria"
	if scenario.name == "unreadable rubric" {
		field = "rubric"
	}
	if len(projections) != 1 || len(projections[0]) != 5 || projections[0]["test_case_id"] != id || projections[0]["field"] != field || projections[0]["level"] != "ERROR" {
		t.Fatalf("unreadable design diagnostic: %v, want one ERROR for %s/%s", projections, id, field)
	}
}

func assertReadableTestCaseProjection(t *testing.T, scenario testCaseProjectionScenario, route string, status int, body map[string]any) {
	t.Helper()
	if status != http.StatusOK {
		t.Fatalf("readable design: status=%d body=%v", status, body)
	}
	if route == "list" {
		rows, ok := body["test_cases"].([]any)
		if !ok || len(rows) != 1 {
			t.Fatalf("readable list: %v", body)
		}
		body, ok = rows[0].(map[string]any)
		if !ok {
			t.Fatalf("readable list row: %v", rows[0])
		}
		if body["criteria_total"] != scenario.count || body["criteria_confirmed"] != scenario.count || body["has_rubric"] != scenario.hasRubric {
			t.Fatalf("readable design facts: %v", body)
		}
	}
	criteria, ok := body["acceptance_criteria"].([]any)
	_, hasRubric := body["rubric"]
	if !ok || float64(len(criteria)) != scenario.count || hasRubric != scenario.hasRubric {
		t.Fatalf("readable design contents: %v", body)
	}
}

func TestUnreadableTestCaseWritesRequireReadingTheStoredOutcome(t *testing.T) {
	for _, edit := range []struct {
		name    string
		suffix  string
		body    string
		query   string
		want    string
		message string
	}{
		{"prompt", "", `{"name":"committed name"}`, `SELECT name FROM test_cases WHERE id = $1`, "committed name", "讀取失敗"},
		{"criterion", "/criteria/c1", `{"text":"edited criterion"}`, `SELECT acceptance_criteria -> 0 ->> 'text' FROM test_cases WHERE id = $1`, "complete", "更新驗收條件失敗"},
	} {
		t.Run(edit.name, func(t *testing.T) {
			pool := requireDB(t)
			a := newAPI(t, pool)
			owner := a.login(t, freshName("projection-edit"))
			_, id := newTestCase(t, pool, a, owner, "read projection edit")
			if _, err := pool.Exec(t.Context(), `UPDATE test_cases SET acceptance_criteria = '[{"id":"c1","text":"complete","source":"user","confirmed_at":null}]'::jsonb, rubric = '{"version":"v1","items":[{"id":"c1","text":"projection-private-canary","evidence_required":"projection-private-canary"}]}'::jsonb WHERE id = $1`, id); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := pool.Exec(context.Background(), `UPDATE test_cases SET acceptance_criteria = '[]'::jsonb, rubric = NULL WHERE id = $1`, id); err != nil {
					t.Error(err)
				}
			})
			status, body := owner.doJSON(t, http.MethodPatch, "/test-cases/"+id+edit.suffix, edit.body)
			if status != http.StatusInternalServerError || body["error"] != edit.message || len(body) != 1 {
				t.Fatalf("unreadable edit response: status=%d body=%v, want generic500", status, body)
			}
			var stored string
			if err := pool.QueryRow(t.Context(), edit.query, id).Scan(&stored); err != nil || stored != edit.want {
				t.Fatalf("write outcome: stored=%q err=%v, want %q after response failure", stored, err, edit.want)
			}
		})
	}
}
