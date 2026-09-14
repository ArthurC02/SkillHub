package apiserver_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func failSearchReferences(t *testing.T) func(context.Context, identity.Workspace, string) ([]creation.Reference, error) {
	return func(context.Context, identity.Workspace, string) ([]creation.Reference, error) {
		t.Fatal("SearchReferences called")
		return nil, nil
	}
}

func okIssueKey(context.Context, string, string, float64, time.Duration) (string, error) {
	return "test-attempt-key", nil
}

func okRevokeKey(context.Context, string) error { return nil }

func failIssueKey(t *testing.T) func(context.Context, string, string, float64, time.Duration) (string, error) {
	return func(context.Context, string, string, float64, time.Duration) (string, error) {
		t.Fatal("IssueKey called")
		return "", nil
	}
}

func failRevokeKey(t *testing.T) func(context.Context, string) error {
	return func(context.Context, string) error {
		t.Fatal("RevokeKey called")
		return nil
	}
}

func failLLM(t *testing.T) creationStepFunc {
	return creationStepFunc(func(context.Context, llmclient.CreationStepRequest) (*llmclient.CreationStepResponse, error) {
		t.Fatal("LLM called")
		return nil, nil
	})
}

func int64Ptr(v int64) *int64 { return &v }

func TestStepRequiresLLMAndKeyCallbacks(t *testing.T) {
	llmOK := creationStepFunc(func(context.Context, llmclient.CreationStepRequest) (*llmclient.CreationStepResponse, error) {
		return &llmclient.CreationStepResponse{}, nil
	})
	cases := []struct {
		name string
		svc  creation.Service
	}{
		{"LLM nil", creation.Service{IssueKey: okIssueKey, RevokeKey: okRevokeKey}},
		{"IssueKey nil", creation.Service{LLM: llmOK, RevokeKey: okRevokeKey}},
		{"RevokeKey nil", creation.Service{LLM: llmOK, IssueKey: okIssueKey}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.svc.Step(context.Background(), creation.JobArgs{}, nil)
			if !errors.Is(err, creation.ErrUnavailable) {
				t.Fatalf("got %v, want ErrUnavailable", err)
			}
		})
	}
}

func TestStepOnAnUnknownSessionIsANoop(t *testing.T) {
	pool := requireDB(t)
	svc := &creation.Service{Pool: pool, LLM: failLLM(t), IssueKey: failIssueKey(t), RevokeKey: failRevokeKey(t)}
	var ghost pgtype.UUID
	if err := pool.QueryRow(context.Background(), "SELECT gen_random_uuid()").Scan(&ghost); err != nil {
		t.Fatal(err)
	}
	err := svc.Step(context.Background(), creation.JobArgs{SessionID: ghost, WorkspaceID: ghost, Revision: 1, ReceiptID: ghost}, nil)
	if err != nil {
		t.Fatalf("got %v, want nil", err)
	}
}

func TestStepRejectsATransientDiagramThatDoesNotMatchTheStored(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	svc := &creation.Service{Pool: pool, Limits: creationLimits(), LLM: failLLM(t), IssueKey: failIssueKey(t), RevokeKey: failRevokeKey(t)}
	id := creationID(t)
	v, err := svc.Create(context.Background(), ws, id, "", .5)
	if err != nil {
		t.Fatal(err)
	}
	storedB64 := base64.StdEncoding.EncodeToString([]byte("stored-diagram-bytes"))
	_, job, err := svc.Act(context.Background(), ws, id, creation.Command{
		ID: creationID(t), ExpectedRevision: v.Revision, Kind: "diagram",
		Diagram: &llmclient.GenerateDiagram{MediaType: "image/png", Data: storedB64},
	})
	if err != nil {
		t.Fatal(err)
	}
	if job == nil {
		t.Fatal("diagram command did not return a transient job")
	}
	other := &llmclient.GenerateDiagram{MediaType: "image/png", Data: base64.StdEncoding.EncodeToString([]byte("different-bytes"))}
	err = svc.Step(context.Background(), *job, other)
	if !errors.Is(err, creation.ErrInvalidCommand) {
		t.Fatalf("got %v, want ErrInvalidCommand", err)
	}
}

func TestStepFailsWhenTheSessionDeadlineHasPassed(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	rec := &jobRecorder{}
	svc := &creation.Service{Pool: pool, Limits: creationLimits(), Insert: rec.insert, LLM: failLLM(t), IssueKey: failIssueKey(t), RevokeKey: failRevokeKey(t)}
	id := creationID(t)
	if _, err := svc.Create(context.Background(), ws, id, "開始創作", .5); err != nil {
		t.Fatal(err)
	}
	job := rec.calls[0]
	past := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	if _, err := pool.Exec(context.Background(), `UPDATE creation_sessions SET snapshot = jsonb_set(snapshot, '{deadline}', to_jsonb($2::text)) WHERE id=$1`, id, past); err != nil {
		t.Fatal(err)
	}
	if err := svc.Step(context.Background(), job, nil); err != nil {
		t.Fatalf("got %v, want nil", err)
	}
	v, err := svc.Get(context.Background(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.State != "failed" {
		t.Fatalf("state = %q, want failed", v.State)
	}
	last := v.Snapshot.Messages[len(v.Snapshot.Messages)-1]
	if last.Role != "assistant" || last.Content != "創作已達這次核准的限制，請開始新的創作。" {
		t.Fatalf("last message = %+v, want the deadline sentence", last)
	}
	var status string
	if err := pool.QueryRow(context.Background(), "SELECT status FROM creation_receipts WHERE id=$1", job.ReceiptID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "failed" {
		t.Fatalf("receipt status = %q, want failed", status)
	}
}

func TestStepFailsQueuedWhenStepsReachTheLimit(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	rec := &jobRecorder{}
	svc := &creation.Service{Pool: pool, Limits: creationLimits(), Insert: rec.insert, LLM: failLLM(t), IssueKey: failIssueKey(t), RevokeKey: failRevokeKey(t)}
	id := creationID(t)
	if _, err := svc.Create(context.Background(), ws, id, "開始創作", .5); err != nil {
		t.Fatal(err)
	}
	job := rec.calls[0]
	if _, err := pool.Exec(context.Background(), `UPDATE creation_sessions SET snapshot = jsonb_set(snapshot, '{snapshot,steps}', $2::jsonb) WHERE id=$1`, id, fmt.Sprintf("%d", svc.Limits.MaxSteps)); err != nil {
		t.Fatal(err)
	}
	if err := svc.Step(context.Background(), job, nil); err != nil {
		t.Fatalf("got %v, want nil", err)
	}
	v, err := svc.Get(context.Background(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.State != "failed" {
		t.Fatalf("state = %q, want failed", v.State)
	}
	last := v.Snapshot.Messages[len(v.Snapshot.Messages)-1]
	if last.Content != "已達這次核准的步數上限，請開始新的創作。" {
		t.Fatalf("last message = %q, want the step-limit sentence", last.Content)
	}
}

func TestStepNeedsReuploadWhenDiagramUnderstandingIsMissing(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	rec := &jobRecorder{}
	svc := &creation.Service{Pool: pool, Limits: creationLimits(), Insert: rec.insert, LLM: failLLM(t), IssueKey: failIssueKey(t), RevokeKey: failRevokeKey(t)}
	id := creationID(t)
	if _, err := svc.Create(context.Background(), ws, id, "開始創作", .5); err != nil {
		t.Fatal(err)
	}
	job := rec.calls[0]
	if _, err := pool.Exec(context.Background(), `UPDATE creation_sessions SET snapshot = jsonb_set(snapshot, '{snapshot,diagram_fingerprint}', to_jsonb($2::text)) WHERE id=$1`, id, "deadbeef"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Step(context.Background(), job, nil); err != nil {
		t.Fatalf("got %v, want nil", err)
	}
	v, err := svc.Get(context.Background(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.State != "needs_reupload" {
		t.Fatalf("state = %q, want needs_reupload", v.State)
	}
	last := v.Snapshot.Messages[len(v.Snapshot.Messages)-1]
	if last.Content != "流程圖需要重新上傳。" {
		t.Fatalf("last message = %q, want the reupload sentence", last.Content)
	}
}

func TestStepFetchesThePendingURLBeforeCallingTheModel(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	rec := &jobRecorder{}
	var fetchCalls []string
	const url = "https://example.test/doc"
	svc := &creation.Service{
		Pool: pool, Limits: creationLimits(), Insert: rec.insert,
		Fetch: func(_ context.Context, u string) (creation.Fetch, string) {
			fetchCalls = append(fetchCalls, u)
			return creation.Fetch{URL: u, Status: "ok"}, "頁面內容"
		},
		IssueKey: okIssueKey, RevokeKey: okRevokeKey,
	}
	var captured llmclient.CreationStepRequest
	svc.LLM = creationStepFunc(func(_ context.Context, r llmclient.CreationStepRequest) (*llmclient.CreationStepResponse, error) {
		captured = r
		return &llmclient.CreationStepResponse{Outcome: "clarification", Message: "好的"}, nil
	})
	id := creationID(t)
	if _, err := svc.Create(context.Background(), ws, id, "開始創作", .5); err != nil {
		t.Fatal(err)
	}
	job := rec.calls[0]
	if _, err := pool.Exec(context.Background(), `UPDATE creation_sessions SET snapshot = jsonb_set(snapshot, '{snapshot,pending_fetch_url}', to_jsonb($2::text)) WHERE id=$1`, id, url); err != nil {
		t.Fatal(err)
	}
	if err := svc.Step(context.Background(), job, nil); err != nil {
		t.Fatal(err)
	}
	if len(fetchCalls) != 1 || fetchCalls[0] != url {
		t.Fatalf("fetch calls = %v, want exactly one call to %s", fetchCalls, url)
	}
	v, err := svc.Get(context.Background(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.Snapshot.PendingFetchURL != "" {
		t.Fatalf("pending fetch url = %q, want cleared", v.Snapshot.PendingFetchURL)
	}
	if len(v.Snapshot.Fetches) != 1 || v.Snapshot.Fetches[0].URL != url || v.Snapshot.Fetches[0].Status != "ok" {
		t.Fatalf("fetches = %+v, want one ok record for %s", v.Snapshot.Fetches, url)
	}
	found := false
	for _, m := range captured.Messages {
		if m.Role == "tool" && strings.Contains(m.Content, url) && strings.Contains(m.Content, "已讀取") {
			found = true
		}
	}
	if !found {
		t.Fatalf("request messages = %+v, want a tool message built from the fetch record", captured.Messages)
	}
}

func TestStepCreditReserveFailureSkipsTheModelCallAndFails(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	rec := &jobRecorder{}
	reserveErr := errors.New("reserve down")
	svc := &creation.Service{
		Pool: pool, Limits: creationLimits(), Insert: rec.insert,
		LLM: failLLM(t), IssueKey: failIssueKey(t), RevokeKey: okRevokeKey,
		CreditReserve: func(context.Context, pgtype.UUID, int64) (bool, error) { return false, reserveErr },
	}
	id := creationID(t)
	if _, err := svc.Create(context.Background(), ws, id, "開始創作", .5); err != nil {
		t.Fatal(err)
	}
	job := rec.calls[0]
	if err := svc.Step(context.Background(), job, nil); err != nil {
		t.Fatal(err)
	}
	v, err := svc.Get(context.Background(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.State != "failed" {
		t.Fatalf("state = %q, want failed", v.State)
	}
	last := v.Snapshot.Messages[len(v.Snapshot.Messages)-1]
	if !strings.Contains(last.Content, "平台這一側沒能完成這次模型呼叫") {
		t.Fatalf("last message = %q, want it to name the platform-side failure", last.Content)
	}
}

func TestStepIssueKeyFailureStillRevokesOnce(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	rec := &jobRecorder{}
	issueErr := errors.New("issue down")
	var revokeCalls int
	svc := &creation.Service{
		Pool: pool, Limits: creationLimits(), Insert: rec.insert,
		LLM: failLLM(t),
		IssueKey: func(context.Context, string, string, float64, time.Duration) (string, error) {
			return "", issueErr
		},
		RevokeKey: func(context.Context, string) error { revokeCalls++; return nil },
	}
	id := creationID(t)
	if _, err := svc.Create(context.Background(), ws, id, "開始創作", .5); err != nil {
		t.Fatal(err)
	}
	job := rec.calls[0]
	if err := svc.Step(context.Background(), job, nil); err != nil {
		t.Fatal(err)
	}
	if revokeCalls != 1 {
		t.Fatalf("revoke calls = %d, want 1", revokeCalls)
	}
	v, err := svc.Get(context.Background(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.State != "failed" {
		t.Fatalf("state = %q, want failed", v.State)
	}
}

func TestFinishWhenTheReceiptWasAlreadyMarkedFailedIsANoop(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	rec := &jobRecorder{}
	var settleCalls int
	svc := &creation.Service{
		Pool: pool, Limits: creationLimits(), Insert: rec.insert,
		IssueKey: okIssueKey, RevokeKey: okRevokeKey,
		CreditSettle: func(context.Context, pgx.Tx, pgtype.UUID, pgtype.UUID, int64, *int64, int64) error {
			settleCalls++
			return nil
		},
	}
	id := creationID(t)
	if _, err := svc.Create(context.Background(), ws, id, "開始創作", .5); err != nil {
		t.Fatal(err)
	}
	job := rec.calls[0]
	svc.LLM = creationStepFunc(func(context.Context, llmclient.CreationStepRequest) (*llmclient.CreationStepResponse, error) {
		if _, err := pool.Exec(context.Background(), "UPDATE creation_receipts SET status='failed' WHERE id=$1", job.ReceiptID); err != nil {
			t.Fatal(err)
		}
		return &llmclient.CreationStepResponse{Outcome: "clarification", Message: "好的"}, nil
	})
	before, err := svc.Get(context.Background(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Step(context.Background(), job, nil); err != nil {
		t.Fatal(err)
	}
	after, err := svc.Get(context.Background(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	if after.State != "working" || after.Revision != before.Revision+1 {
		t.Fatalf("after step: state=%q revision=%d, want working at exactly one revision past the pre-step view (%d)", after.State, after.Revision, before.Revision)
	}
	var eventType string
	if err := pool.QueryRow(context.Background(), "SELECT event_type FROM creation_session_events WHERE session_id=$1 ORDER BY revision DESC LIMIT 1", id).Scan(&eventType); err != nil {
		t.Fatal(err)
	}
	if eventType != "attempt_started" {
		t.Fatalf("last event = %q, want attempt_started (no attempt_settled)", eventType)
	}
	if settleCalls != 0 {
		t.Fatalf("CreditSettle called %d times, want 0", settleCalls)
	}
}

func TestFinishWhenTheReceiptWasRecoveredAsUnknownStillSettlesTheKnownCost(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	rec := &jobRecorder{}
	type settleCall struct {
		usdMicros         *int64
		reservedUSDMicros int64
	}
	var settles []settleCall
	svc := &creation.Service{
		Pool: pool, Limits: creationLimits(), Insert: rec.insert,
		IssueKey: okIssueKey, RevokeKey: okRevokeKey,
		CreditSettle: func(_ context.Context, _ pgx.Tx, _, _ pgtype.UUID, _ int64, usdMicros *int64, reservedUSDMicros int64) error {
			settles = append(settles, settleCall{usdMicros, reservedUSDMicros})
			return nil
		},
	}
	id := creationID(t)
	if _, err := svc.Create(context.Background(), ws, id, "開始創作", .5); err != nil {
		t.Fatal(err)
	}
	job := rec.calls[0]
	cost := .02
	svc.LLM = creationStepFunc(func(context.Context, llmclient.CreationStepRequest) (*llmclient.CreationStepResponse, error) {
		if _, err := pool.Exec(context.Background(), "UPDATE creation_receipts SET status='unknown' WHERE id=$1", job.ReceiptID); err != nil {
			t.Fatal(err)
		}
		return &llmclient.CreationStepResponse{Outcome: "clarification", Message: "好的", Usage: &llmclient.GatewayUsage{CostUSD: &cost}}, nil
	})
	before, err := svc.Get(context.Background(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Step(context.Background(), job, nil); err != nil {
		t.Fatal(err)
	}
	after, err := svc.Get(context.Background(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision+1 {
		t.Fatalf("session revision moved to %d, want unchanged from step's own advance (%d)", after.Revision, before.Revision+1)
	}
	var status string
	var usage []byte
	if err := pool.QueryRow(context.Background(), "SELECT status, usage FROM creation_receipts WHERE id=$1", job.ReceiptID).Scan(&status, &usage); err != nil {
		t.Fatal(err)
	}
	if status != "finished" {
		t.Fatalf("receipt status = %q, want finished", status)
	}
	var storedUsage llmclient.GatewayUsage
	if err := json.Unmarshal(usage, &storedUsage); err != nil {
		t.Fatal(err)
	}
	if storedUsage.CostUSD == nil || *storedUsage.CostUSD != cost {
		t.Fatalf("stored usage = %+v, want cost %v", storedUsage, cost)
	}
	if len(settles) != 1 {
		t.Fatalf("CreditSettle calls = %d, want 1", len(settles))
	}
	wantMicros := int64(20000)
	if settles[0].usdMicros == nil || *settles[0].usdMicros != wantMicros {
		t.Fatalf("settled micros = %v, want %d", settles[0].usdMicros, wantMicros)
	}
	wantReserved := int64(100000)
	if settles[0].reservedUSDMicros != wantReserved {
		t.Fatalf("reserved micros = %d, want %d (MaxCallCostUSD)", settles[0].reservedUSDMicros, wantReserved)
	}
}

func TestFinishNormalPathRecordsCreditSettleCostFromUsage(t *testing.T) {
	cost := .04
	cases := []struct {
		name  string
		usage *llmclient.GatewayUsage
		want  *int64
	}{
		{"usage with a finite non-negative cost", &llmclient.GatewayUsage{CostUSD: &cost}, int64Ptr(40000)},
		{"response with no usage", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool := requireDB(t)
			ws := newCreationWorkspace(t, pool)
			rec := &jobRecorder{}
			var got *int64
			var settleCalls int
			svc := &creation.Service{
				Pool: pool, Limits: creationLimits(), Insert: rec.insert,
				IssueKey: okIssueKey, RevokeKey: okRevokeKey,
				CreditSettle: func(_ context.Context, _ pgx.Tx, _, _ pgtype.UUID, _ int64, usdMicros *int64, _ int64) error {
					settleCalls++
					got = usdMicros
					return nil
				},
			}
			svc.LLM = creationStepFunc(func(context.Context, llmclient.CreationStepRequest) (*llmclient.CreationStepResponse, error) {
				return &llmclient.CreationStepResponse{Outcome: "clarification", Message: "好的", Usage: tc.usage}, nil
			})
			id := creationID(t)
			if _, err := svc.Create(context.Background(), ws, id, "開始創作", .5); err != nil {
				t.Fatal(err)
			}
			job := rec.calls[0]
			if err := svc.Step(context.Background(), job, nil); err != nil {
				t.Fatal(err)
			}
			if settleCalls != 1 {
				t.Fatalf("CreditSettle calls = %d, want 1", settleCalls)
			}
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Fatalf("settled micros = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFinishWhenCreditSettleFailsStepReturnsItAndTheSessionStaysWorking(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	rec := &jobRecorder{}
	settleErr := errors.New("ledger down")
	svc := &creation.Service{
		Pool: pool, Limits: creationLimits(), Insert: rec.insert,
		IssueKey: okIssueKey, RevokeKey: okRevokeKey,
		LLM: creationStepFunc(func(context.Context, llmclient.CreationStepRequest) (*llmclient.CreationStepResponse, error) {
			return &llmclient.CreationStepResponse{Outcome: "clarification", Message: "好的"}, nil
		}),
		CreditSettle: func(context.Context, pgx.Tx, pgtype.UUID, pgtype.UUID, int64, *int64, int64) error {
			return settleErr
		},
	}
	id := creationID(t)
	if _, err := svc.Create(context.Background(), ws, id, "開始創作", .5); err != nil {
		t.Fatal(err)
	}
	job := rec.calls[0]
	before, err := svc.Get(context.Background(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	err = svc.Step(context.Background(), job, nil)
	if !errors.Is(err, settleErr) {
		t.Fatalf("got %v, want %v", err, settleErr)
	}
	after, err := svc.Get(context.Background(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	if after.State != "working" || after.Revision != before.Revision+1 {
		t.Fatalf("after failed settle: state=%q revision=%d, want working at %d", after.State, after.Revision, before.Revision+1)
	}
	var status string
	if err := pool.QueryRow(context.Background(), "SELECT status FROM creation_receipts WHERE id=$1", job.ReceiptID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "running" {
		t.Fatalf("receipt status = %q, want running (finish rolled back)", status)
	}
}

func TestFinishTransientDiagramWithNoUnderstandingNeedsReupload(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	svc := &creation.Service{Pool: pool, Limits: creationLimits(), IssueKey: okIssueKey, RevokeKey: okRevokeKey}
	svc.LLM = creationStepFunc(func(context.Context, llmclient.CreationStepRequest) (*llmclient.CreationStepResponse, error) {
		return &llmclient.CreationStepResponse{Outcome: "clarification", Message: "好的", DiagramUnderstanding: ""}, nil
	})
	id := creationID(t)
	v, err := svc.Create(context.Background(), ws, id, "", .5)
	if err != nil {
		t.Fatal(err)
	}
	b64 := base64.StdEncoding.EncodeToString([]byte("diagram-bytes"))
	_, job, err := svc.Act(context.Background(), ws, id, creation.Command{
		ID: creationID(t), ExpectedRevision: v.Revision, Kind: "diagram",
		Diagram: &llmclient.GenerateDiagram{MediaType: "image/png", Data: b64},
	})
	if err != nil {
		t.Fatal(err)
	}
	if job == nil {
		t.Fatal("diagram command did not return a transient job")
	}
	if err := svc.Step(context.Background(), *job, &llmclient.GenerateDiagram{MediaType: "image/png", Data: b64}); err != nil {
		t.Fatal(err)
	}
	final, err := svc.Get(context.Background(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	if final.State != "needs_reupload" {
		t.Fatalf("state = %q, want needs_reupload", final.State)
	}
	last := final.Snapshot.Messages[len(final.Snapshot.Messages)-1]
	if !strings.Contains(last.Content, "模型的回覆不符合會話規則") {
		t.Fatalf("last message = %q, want it to name the rule violation", last.Content)
	}
}

func TestFinishCreditFloorRefusalShowsBothSentencesAndWaitsForInput(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	rec := &jobRecorder{}
	svc := &creation.Service{
		Pool: pool, Limits: creationLimits(), Insert: rec.insert,
		LLM: failLLM(t), IssueKey: failIssueKey(t), RevokeKey: okRevokeKey,
		CreditReserve: func(context.Context, pgtype.UUID, int64) (bool, error) { return false, nil },
	}
	id := creationID(t)
	if _, err := svc.Create(context.Background(), ws, id, "開始創作", .5); err != nil {
		t.Fatal(err)
	}
	job := rec.calls[0]
	if err := svc.Step(context.Background(), job, nil); err != nil {
		t.Fatal(err)
	}
	v, err := svc.Get(context.Background(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.State != "waiting_input" || v.Snapshot.PendingAction != "" {
		t.Fatalf("state=%q pendingAction=%q, want waiting_input/empty", v.State, v.Snapshot.PendingAction)
	}
	n := len(v.Snapshot.Messages)
	if n < 2 {
		t.Fatalf("messages = %+v, want at least two assistant messages", v.Snapshot.Messages)
	}
	generic, floor := v.Snapshot.Messages[n-2], v.Snapshot.Messages[n-1]
	if !strings.Contains(generic.Content, "這一步未完成；") {
		t.Fatalf("second-to-last message = %q, want the generic failure sentence", generic.Content)
	}
	if floor.Content != "帳戶餘額已達可容忍的欠款上限，請充值後再繼續這場創作。" {
		t.Fatalf("last message = %q, want the credit-floor sentence", floor.Content)
	}
}

func TestFinishWhenTheModelAsksForAnotherStepEnqueuesIt(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	rec := &jobRecorder{}
	svc := &creation.Service{
		Pool: pool, Limits: creationLimits(), Insert: rec.insert,
		IssueKey: okIssueKey, RevokeKey: okRevokeKey, SearchReferences: failSearchReferences(t),
	}
	cost := .01
	svc.LLM = creationStepFunc(func(context.Context, llmclient.CreationStepRequest) (*llmclient.CreationStepResponse, error) {
		return &llmclient.CreationStepResponse{
			Outcome: "tool_intent", Message: "讓我先查查目錄。",
			ToolIntent: &llmclient.CreationToolIntent{Kind: "search_catalog", Query: ""},
			Usage:      &llmclient.GatewayUsage{CostUSD: &cost},
		}, nil
	})
	id := creationID(t)
	if _, err := svc.Create(context.Background(), ws, id, "開始創作", .5); err != nil {
		t.Fatal(err)
	}
	job := rec.calls[0]
	if err := svc.Step(context.Background(), job, nil); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 2 {
		t.Fatalf("insert calls = %d, want 2 (the original queue plus the follow-up step)", len(rec.calls))
	}
	v, err := svc.Get(context.Background(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.State != "queued" {
		t.Fatalf("state = %q, want queued", v.State)
	}
	newJob := rec.calls[1]
	var status string
	if err := pool.QueryRow(context.Background(), "SELECT status FROM creation_receipts WHERE id=$1", newJob.ReceiptID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "queued" {
		t.Fatalf("new receipt status = %q, want queued", status)
	}
}

func TestFinishWhenTheModelAsksForAnotherStepButTheSessionCannotSpendWaitsForInput(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	rec := &jobRecorder{}
	svc := &creation.Service{
		Pool: pool, Limits: creationLimits(), Insert: rec.insert,
		IssueKey: okIssueKey, RevokeKey: okRevokeKey, SearchReferences: failSearchReferences(t),
	}
	svc.LLM = creationStepFunc(func(context.Context, llmclient.CreationStepRequest) (*llmclient.CreationStepResponse, error) {
		return &llmclient.CreationStepResponse{
			Outcome: "tool_intent", Message: "讓我先查查目錄。",
			ToolIntent: &llmclient.CreationToolIntent{Kind: "search_catalog", Query: ""},
		}, nil
	})
	id := creationID(t)
	if _, err := svc.Create(context.Background(), ws, id, "開始創作", .5); err != nil {
		t.Fatal(err)
	}
	job := rec.calls[0]
	if _, err := pool.Exec(context.Background(), `UPDATE creation_sessions SET snapshot = jsonb_set(snapshot, '{snapshot,steps}', $2::jsonb) WHERE id=$1`, id, fmt.Sprintf("%d", svc.Limits.MaxSteps-1)); err != nil {
		t.Fatal(err)
	}
	if err := svc.Step(context.Background(), job, nil); err != nil {
		t.Fatal(err)
	}
	v, err := svc.Get(context.Background(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.State != "waiting_input" {
		t.Fatalf("state = %q, want waiting_input", v.State)
	}
	last := v.Snapshot.Messages[len(v.Snapshot.Messages)-1]
	if last.Content != "已達這次核准的步數上限，請開始新的創作。" {
		t.Fatalf("last message = %q, want the step-limit sentence", last.Content)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("insert calls = %d, want 1 (no follow-up enqueued)", len(rec.calls))
	}
}
