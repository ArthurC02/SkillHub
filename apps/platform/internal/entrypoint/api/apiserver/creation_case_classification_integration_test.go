package apiserver_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
)

type classificationRecorder struct {
	next creation.StepModel
	last []creation.ChallengeCase
}

func (r *classificationRecorder) CreationStep(ctx context.Context, req creation.StepRequest) (*creation.StepResult, error) {
	r.last = append([]creation.ChallengeCase(nil), req.ChallengeCases...)
	return r.next.CreationStep(ctx, req)
}

func assertStoredClassificationPair(t *testing.T, cases []creation.ChallengeCase) {
	t.Helper()
	if len(cases) != 2 || cases[0].ExpectedInteraction != "question" || cases[1].ExpectedInteraction != "direct_answer" {
		t.Fatalf("stored or forwarded classifications = %+v, want question and direct_answer", cases)
	}
}

func assertPublicClassificationDocument(t *testing.T, raw []byte) {
	t.Helper()
	var body struct {
		Snapshot struct {
			Cases []map[string]any `json:"challenge_cases"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	var want []map[string]any
	if err := json.Unmarshal([]byte(`[
		{"name":"缺少原文","prompt":"請整理摘要。","criteria":["詢問要整理的原文。"]},
		{"name":"完整原文","prompt":"請整理摘要：會議決定明天出貨。","criteria":["直接列出明天出貨的決議。"]}
	]`), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(body.Snapshot.Cases, want) {
		t.Fatalf("public cases changed or leaked classifications: %s", raw)
	}
}

func readClassificationResponse(t *testing.T, c *client, path string) []byte {
	t.Helper()
	res, err := c.Get(c.base + path)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status=%d", path, res.StatusCode)
	}
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func firstClassificationStreamDocument(t *testing.T, c *client, id string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/creation-sessions/"+id+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("stream status=%d", res.StatusCode)
	}
	scanner := bufio.NewScanner(res.Body)
	for scanner.Scan() {
		if line := scanner.Text(); strings.HasPrefix(line, "data: ") {
			return []byte(strings.TrimPrefix(line, "data: "))
		}
	}
	t.Fatalf("stream ended without a document: %v", scanner.Err())
	return nil
}

func TestCreationClassificationsSurviveReloadAndStayPrivateOnEveryTransport(t *testing.T) {
	a, service, _ := creationFixture(t)
	c := a.login(t, "creation-classification-transports")
	recorder := &classificationRecorder{next: service.LLM}
	service.LLM = recorder
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_credits": 650}, http.StatusOK)
	v = creationStep(t, service, v)
	stored, err := service.Get(context.Background(), identity.Workspace{ID: mustUUID(t, c.workspaceID)}, mustUUID(t, v.ID))
	if err != nil {
		t.Fatal(err)
	}
	assertStoredClassificationPair(t, stored.Snapshot.ChallengeCases)
	assertPublicClassificationDocument(t, readClassificationResponse(t, c, "/creation-sessions/"+v.ID))
	var list []json.RawMessage
	if err := json.Unmarshal(readClassificationResponse(t, c, "/creation-sessions"), &list); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, document := range list {
		var item struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(document, &item); err != nil {
			t.Fatal(err)
		}
		if item.ID == v.ID {
			assertPublicClassificationDocument(t, document)
			found = true
		}
	}
	if !found {
		t.Fatal("the scoped list omitted the current session")
	}
	assertPublicClassificationDocument(t, firstClassificationStreamDocument(t, c, v.ID))
	status, raw := creationPostStatus(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{"command_id": creationID(t), "expected_revision": v.Revision, "kind": "confirm_brief"})
	if status != http.StatusOK {
		t.Fatalf("confirm status=%d body=%s", status, raw)
	}
	assertPublicClassificationDocument(t, []byte(raw))
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	creationStep(t, service, v)
	assertStoredClassificationPair(t, recorder.last)
}

func TestCreationRepairsTwoLegacyInputsWhoseClassificationsAreMissingOrUnsupported(t *testing.T) {
	for _, kind := range []string{"", "unsupported"} {
		t.Run("kind_"+kind, func(t *testing.T) {
			a, service, _ := creationFixture(t)
			c := a.login(t, "creation-legacy-classification-"+kind)
			v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_credits": 650}, http.StatusOK)
			v = creationStep(t, service, v)
			cases := creationFixtureEvaluationCases()
			for i := range cases {
				cases[i].ExpectedInteraction = kind
			}
			raw, err := json.Marshal(cases)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := testPool.Exec(context.Background(), `UPDATE creation_sessions SET snapshot=jsonb_set(snapshot,'{snapshot,challenge_cases}',$3::jsonb) WHERE id=$1 AND workspace_id=$2`, mustUUID(t, v.ID), mustUUID(t, c.workspaceID), raw); err != nil {
				t.Fatal(err)
			}
			v = creationAct(t, c, v, "confirm_brief")
			assertCreationRepairsEvaluationCases(t, v)
			v = creationStep(t, service, v)
			if v.State != "waiting_confirmation" || v.Snapshot.BriefConfirmed {
				t.Fatalf("legacy classifications bypassed confirmation: %+v", v)
			}
			assertStoredClassificationPair(t, v.Snapshot.ChallengeCases)
		})
	}
}
