package apiserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation/webfetch"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func assertStoredFetchTextAbsent(t *testing.T, pool *pgxpool.Pool, id pgtype.UUID, marker string) {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(t.Context(), `SELECT snapshot FROM creation_sessions WHERE id = $1`, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), marker) {
		t.Error("fetched page text was retained in the creation snapshot")
	}
	var leaked bool
	if err := pool.QueryRow(t.Context(), `SELECT EXISTS (SELECT 1 FROM creation_receipts WHERE session_id = $1 AND position($2 in result::text) > 0)`, id, marker).Scan(&leaked); err != nil {
		t.Fatal(err)
	}
	if leaked {
		t.Error("fetched page text was retained in a command receipt")
	}
}

func TestFetchedPageTextIsTransientDuringAndAfterTheModelCall(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "answered", true: "model failed"}[failed], func(t *testing.T) {
			checkTransientFetch(t, failed)
		})
	}
}

func checkTransientFetch(t *testing.T, failed bool) {
	t.Helper()
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	jobs := &jobRecorder{}
	svc := newCreateService(pool, jobs)
	const url = "https://example.test/private-rule"
	const marker = "PRIVATE_PAGE_BODY_7b62c"
	const body = marker + " Math 40%, science 60%. secret"
	fetches, calls := 0, 0
	svc.Fetch = func(_ context.Context, requested string) (creation.Fetch, string) {
		fetches++
		if requested != url {
			t.Errorf("fetched %q, want %q", requested, url)
		}
		return creation.Fetch{URL: url, Status: "ok", SHA256: "page-hash", Bytes: len(body)}, body
	}
	svc.Mask = func(text string) string { return strings.ReplaceAll(text, "secret", "***") }
	svc.IssueKey, svc.RevokeKey = okIssueKey, okRevokeKey
	view, id := newActSession(t, svc, ws)
	svc.LLM = creationStepFunc(func(_ context.Context, req creation.StepRequest) (*creation.StepResult, error) {
		calls++
		encoded, err := json.Marshal(req.Messages)
		if err != nil || !strings.Contains(string(encoded), marker) || strings.Contains(string(encoded), "secret") {
			t.Errorf("the current model call did not receive the masked page: %s, %v", encoded, err)
		}
		assertStoredFetchTextAbsent(t, pool, id, marker)
		if failed {
			return nil, errors.New("model unavailable")
		}
		zero := 0.0
		return &creation.StepResult{Outcome: "clarification", Message: "請補充資料。", Usage: &creation.ModelUsage{CostUSD: &zero, CostReported: true}}, nil
	})
	setCreationSnapshotField(t, pool, id, "pending_action", "confirm_fetch")
	setCreationSnapshotField(t, pool, id, "pending_fetch_url", url)
	if _, _, err := svc.Act(t.Context(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: view.Revision, Kind: "confirm_fetch"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Step(t.Context(), jobs.calls[0], nil); err != nil {
		t.Fatal(err)
	}
	assertStoredFetchTextAbsent(t, pool, id, marker)
	current, err := svc.Get(t.Context(), ws, id)
	if err != nil || fetches != 1 || calls != 1 {
		t.Fatalf("fetch metadata or call count changed: view=%+v fetches=%d calls=%d err=%v", current, fetches, calls, err)
	}
	assertCreationFetchRecord(t, current.Snapshot.Fetches, creation.Fetch{URL: url, Status: "ok", SHA256: "page-hash", Bytes: len(body)})
	public, err := json.Marshal(current)
	if err != nil || strings.Contains(string(public), marker) {
		t.Errorf("fetched body leaked in the public view: %s, %v", public, err)
	}
	if !failed {
		assertNextStepHasOnlyFetchMetadata(t, svc, ws, id, jobs, marker)
		if fetches != 1 {
			t.Errorf("the next step fetched again without permission: %d calls", fetches)
		}
	}
}

func assertCreationFetchRecord(t *testing.T, got []creation.Fetch, want creation.Fetch) {
	t.Helper()
	if !reflect.DeepEqual(got, []creation.Fetch{want}) {
		t.Fatalf("fetch records = %+v, want exactly %+v", got, want)
	}
}

func assertNextStepHasOnlyFetchMetadata(t *testing.T, svc *creation.Service, ws identity.Workspace, id pgtype.UUID, jobs *jobRecorder, marker string) {
	t.Helper()
	svc.LLM = creationStepFunc(func(_ context.Context, req creation.StepRequest) (*creation.StepResult, error) {
		encoded, _ := json.Marshal(req.Messages)
		if strings.Contains(string(encoded), marker) {
			t.Error("the next model call reused the previous page text")
		}
		zero := 0.0
		return &creation.StepResult{Outcome: "clarification", Message: "請貼上規則。", Usage: &creation.ModelUsage{CostUSD: &zero, CostReported: true}}, nil
	})
	current, err := svc.Get(t.Context(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Act(t.Context(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: current.Revision, Kind: "message", Message: "繼續"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Step(t.Context(), jobs.calls[len(jobs.calls)-1], nil); err != nil {
		t.Fatal(err)
	}
	assertStoredFetchTextAbsent(t, svc.Pool, id, marker)
}

func TestCurrentFetchedRulesReachTheSourceGateWithoutBeingPersisted(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	jobs := &jobRecorder{}
	svc := newCreateService(pool, jobs)
	const url = "https://example.test/weights"
	const quote = "Math 40%, science 60%."
	svc.Fetch = func(context.Context, string) (creation.Fetch, string) {
		return creation.Fetch{URL: url, Status: "ok", SHA256: "weights-hash", Bytes: len(quote)}, quote
	}
	svc.IssueKey, svc.RevokeKey = okIssueKey, okRevokeKey
	svc.Drafts.Validate = func(context.Context, creation.GeneratedSkill) (string, string, bool, error) {
		return "checked-hash", "{}", false, nil
	}
	svc.LLM = creationStepFunc(func(_ context.Context, req creation.StepRequest) (*creation.StepResult, error) {
		index, name := len(req.Messages)-1, "references/weights.md"
		quoted := quote
		zero := 0.0
		return &creation.StepResult{Outcome: "draft", Message: "已依規則起草。",
			Draft:            &creation.GeneratedSkill{Name: "weighted-score", Body: "Read " + name + ".", Files: []creation.GeneratedFile{{Path: name, Content: quote}}},
			SourceAssessment: &creation.SourceAssessment{Rules: []creation.SourceRule{{Rule: "Weights", SourceMessageIndex: &index, Quote: &quoted, Path: &name}}},
			Usage:            &creation.ModelUsage{CostUSD: &zero, CostReported: true}}, nil
	})
	view, id := newActSession(t, svc, ws)
	setCreationSnapshotField(t, pool, id, "brief", "Compute weighted scores")
	setCreationSnapshotField(t, pool, id, "brief_confirmed", true)
	setCreationSnapshotField(t, pool, id, "challenge_cases", creationFixtureEvaluationCases())
	setCreationSnapshotField(t, pool, id, "pending_action", "confirm_fetch")
	setCreationSnapshotField(t, pool, id, "pending_fetch_url", url)
	if _, _, err := svc.Act(t.Context(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: view.Revision, Kind: "confirm_fetch"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Step(t.Context(), jobs.calls[0], nil); err != nil {
		t.Fatal(err)
	}
	current, err := svc.Get(t.Context(), ws, id)
	if err != nil || current.State != "draft_ready" || current.Snapshot.Draft == nil {
		t.Fatalf("the source gate lost the current page: view=%+v err=%v", current, err)
	}
	if strings.Contains(current.Snapshot.Messages[0].Content, quote) {
		t.Error("the source gate copied page text into the stored tool message")
	}
}

func TestCancellingDuringTheModelCallDoesNotRetainTheFetchedPage(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	jobs := &jobRecorder{}
	svc := newCreateService(pool, jobs)
	const marker = "CANCELLED_PAGE_BODY_8e0f"
	const url = "https://example.test/cancelled"
	svc.Fetch = func(context.Context, string) (creation.Fetch, string) {
		return creation.Fetch{URL: url, Status: "ok", SHA256: "cancelled-hash", Bytes: len(marker)}, marker
	}
	svc.IssueKey, svc.RevokeKey = okIssueKey, okRevokeKey
	view, id := newActSession(t, svc, ws)
	svc.LLM = creationStepFunc(func(_ context.Context, req creation.StepRequest) (*creation.StepResult, error) {
		encoded, _ := json.Marshal(req.Messages)
		if !strings.Contains(string(encoded), marker) {
			t.Error("the cancelled attempt never received its approved page")
		}
		current, err := svc.Get(t.Context(), ws, id)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := svc.Act(t.Context(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: current.Revision, Kind: "cancel"}); err != nil {
			t.Fatal(err)
		}
		zero := 0.0
		return &creation.StepResult{Outcome: "clarification", Message: "late response", Usage: &creation.ModelUsage{CostUSD: &zero, CostReported: true}}, nil
	})
	setCreationSnapshotField(t, pool, id, "pending_action", "confirm_fetch")
	setCreationSnapshotField(t, pool, id, "pending_fetch_url", url)
	if _, _, err := svc.Act(t.Context(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: view.Revision, Kind: "confirm_fetch"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Step(t.Context(), jobs.calls[0], nil); err != nil {
		t.Fatal(err)
	}
	current, err := svc.Get(t.Context(), ws, id)
	if err != nil || current.State != "cancelled" {
		t.Fatalf("late response revived the cancelled creation: %+v, %v", current, err)
	}
	assertStoredFetchTextAbsent(t, pool, id, marker)
}

func TestFetchTextMigrationErasesLegacySnapshotsAndReceiptsWithoutChangingAuthoredContent(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	svc := newCreateService(pool, &jobRecorder{})
	view, id := newActSession(t, svc, ws)
	record := creation.Fetch{URL: `https://example.test/legacy\u0000`, Status: "ok", SHA256: "legacy-hash", Bytes: 100}
	body := webfetch.Observation(record, strings.Join([]string{"private legacy page", "\x00", "\\\x00", "\\\\\x00", "\\\\\\\x00"}, " "))
	messages := []creation.Message{{Role: "user", Content: body}, {Role: "tool", Content: body}, {Role: "tool", Content: "{"}, {Role: "tool", Content: `{"search":[]}`}, {Role: "tool", Content: webfetch.Observation(record, "private legacy page")}}
	view.Snapshot.Messages = messages
	encoded, _ := json.Marshal(messages)
	result, _ := json.Marshal(view)
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	if _, err := tx.Exec(t.Context(), `UPDATE creation_sessions SET snapshot = jsonb_set(snapshot, '{snapshot,messages}', $2::jsonb) WHERE id = $1`, id, encoded); err != nil {
		t.Fatal(err)
	}
	receipt := creationID(t)
	if _, err := tx.Exec(t.Context(), `INSERT INTO creation_receipts(id,session_id,workspace_id,kind,status,expected_revision,request_hash,result) VALUES($1,$2,$3,'command','finished',1,'hash',$4)`, receipt, id, ws.ID, result); err != nil {
		t.Fatal(err)
	}
	before := readCreationDataOutsideMessages(t, tx, id, receipt)
	if _, err := tx.Exec(t.Context(), `UPDATE users SET deleted_at = now() WHERE id = (SELECT owner_user_id FROM workspaces WHERE id = $1)`, ws.ID); err != nil {
		t.Fatal(err)
	}
	sql, err := os.ReadFile("../../../../../../db/migrations/0153_creation_fetch_text_erasure.sql")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := tx.Exec(t.Context(), string(sql)); err != nil {
			t.Fatal(err)
		}
	}
	var stored, replayed []byte
	var revision int64
	if err := tx.QueryRow(t.Context(), `SELECT snapshot #> '{snapshot,messages}', revision FROM creation_sessions WHERE id = $1`, id).Scan(&stored, &revision); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(t.Context(), `SELECT result #> '{snapshot,messages}' FROM creation_receipts WHERE id = $1`, receipt).Scan(&replayed); err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{stored, replayed} {
		assertMigrationFetchMessages(t, raw, messages, record)
	}
	if revision != view.Revision {
		t.Fatalf("privacy erasure changed the session revision: %d", revision)
	}
	if readCreationDataOutsideMessages(t, tx, id, receipt) != before {
		t.Fatal("privacy erasure changed data outside the tool messages")
	}
}

func readCreationDataOutsideMessages(t *testing.T, tx pgx.Tx, id, receipt pgtype.UUID) string {
	t.Helper()
	var session, command []byte
	if err := tx.QueryRow(t.Context(), `SELECT to_jsonb(s) #- '{snapshot,snapshot,messages}' FROM creation_sessions s WHERE id = $1`, id).Scan(&session); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(t.Context(), `SELECT to_jsonb(r) #- '{result,snapshot,messages}' FROM creation_receipts r WHERE id = $1`, receipt).Scan(&command); err != nil {
		t.Fatal(err)
	}
	return string(session) + string(command)
}

func assertMigrationFetchMessages(t *testing.T, raw []byte, messages []creation.Message, record creation.Fetch) {
	t.Helper()
	var got []creation.Message
	if err := json.Unmarshal(raw, &got); err != nil || len(got) != len(messages) || got[0] != messages[0] || got[2] != messages[2] || got[3] != messages[3] {
		t.Fatalf("migration lost authored content or retained fetched text: %s, %v", raw, err)
	}
	for _, index := range []int{1, 4} {
		var observation struct {
			Fetch creation.Fetch `json:"fetch"`
		}
		if err := json.Unmarshal([]byte(got[index].Content), &observation); err != nil || strings.Contains(got[index].Content, "private legacy page") || observation.Fetch != record {
			t.Fatalf("migration lost authored content or retained fetched text: %s", raw)
		}
	}
}

func TestLegacyPageTextIsHiddenWhenReadingOrReplayingAnOldSession(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	svc := newCreateService(pool, &jobRecorder{})
	view, id := newActSession(t, svc, ws)
	command := creation.Command{ID: creationID(t), ExpectedRevision: view.Revision, Kind: "raise_budget", BudgetUSD: .6}
	current, _, err := svc.Act(t.Context(), ws, id, command)
	if err != nil {
		t.Fatal(err)
	}
	const marker = "LEGACY_FETCHED_BODY_1c3a"
	record := creation.Fetch{URL: "https://example.test/old", Status: "ok", SHA256: "old-hash", Bytes: len(marker)}
	current.Snapshot.Messages = []creation.Message{{Role: "tool", Content: webfetch.Observation(record, marker)}}
	setCreationSnapshotField(t, pool, id, "messages", current.Snapshot.Messages)
	result, _ := json.Marshal(current)
	if _, err := pool.Exec(t.Context(), `UPDATE creation_receipts SET result = $2 WHERE id = $1`, command.ID, result); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(t.Context(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	replayed, _, err := svc.Act(t.Context(), ws, id, command)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []creation.View{got, replayed} {
		encoded, _ := json.Marshal(value)
		if strings.Contains(string(encoded), marker) || !strings.Contains(string(encoded), "old-hash") {
			t.Fatalf("legacy read or replay exposed text or dropped metadata: %s", encoded)
		}
	}
}
