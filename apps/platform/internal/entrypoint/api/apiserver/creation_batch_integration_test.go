package apiserver_test

import (
	"context"
	"encoding/json"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"io"
	"strings"
	"testing"
)

// TestCreationBatchCandidateSurvivesAMessage: a user message after a candidate
// exists must not throw away the materialized Draft/Candidate (creation.go's
// case "message" no longer calls invalidate). Re-materializing the same
// content hash afterwards must select the same version, not create a second one.
func TestCreationBatchCandidateSurvivesAMessage(t *testing.T) {
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-batch-message")
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_usd": .5}, 200)
	v = creationStep(t, s, v)
	v = creationAct(t, c, v, "confirm_brief")
	v = creationStep(t, s, v)
	if v.State != "draft_ready" || v.Snapshot.Draft == nil || v.Snapshot.Draft.Blocked {
		t.Fatalf("no validated draft: %+v", v)
	}
	draftHash := v.Snapshot.Draft.ContentHash
	v = creationAct(t, c, v, "materialize")
	if v.Snapshot.Candidate == nil {
		t.Fatal("candidate not committed")
	}
	candidate := *v.Snapshot.Candidate
	v = creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{"command_id": creationID(t), "expected_revision": v.Revision, "kind": "message", "message": "很好，就存這一份。"}, 200)
	if v.Snapshot.Candidate == nil || *v.Snapshot.Candidate != candidate {
		t.Fatalf("message discarded the materialized candidate: %+v", v)
	}
	// Since 2026-09-06 a message keeps the confirmed brief (proposal() still
	// un-confirms when the model changes it), so the next step drafts directly.
	v = creationStep(t, s, v)
	if !v.Snapshot.BriefConfirmed {
		t.Fatalf("an ordinary message un-confirmed the brief: %+v", v)
	}
	if v.Snapshot.Draft == nil || v.Snapshot.Draft.ContentHash != draftHash {
		t.Fatalf("draft changed after an ordinary message: %+v", v)
	}
	v = creationAct(t, c, v, "finalize")
	if v.State != "saved" || v.Snapshot.Candidate == nil || v.Snapshot.Candidate.VersionID != candidate.VersionID {
		t.Fatalf("finalize did not reuse the same version: %+v", v)
	}
	var versions int
	if err := testPool.QueryRow(context.Background(), "SELECT count(*) FROM skill_versions WHERE skill_id=$1", candidate.SkillID).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if versions != 1 {
		t.Fatalf("re-materializing an unchanged draft created %d versions", versions)
	}
}

// TestCreationBatchConfirmReferencesRestoresAvailable: a successful re-resolve
// on confirm_references must flip Available back to true, not just Confirmed
// (a transient lookup outage must not permanently refuse the reference).
// Driven directly through the Service: a session is created for a real
// workspace, then its snapshot is put into the exact state a failed step
// leaves behind (Available=false, Confirmed=false, pending confirm_references)
// without going through the model at all.
func TestCreationBatchConfirmReferencesRestoresAvailable(t *testing.T) {
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-batch-refs")
	ws, err := creation.ParseID(c.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Create(context.Background(), identity.Workspace{ID: ws}, creationID(t), "", .5)
	if err != nil {
		t.Fatal(err)
	}
	id, err := creation.ParseID(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = testPool.Exec(context.Background(), `UPDATE creation_sessions SET state='waiting_confirmation', snapshot = jsonb_set(
	 jsonb_set(snapshot, '{snapshot,references}', '[{"skill_id":"ref-skill","version_id":"ref-version","name":"Ref","confirmed":false,"available":false}]'::jsonb),
	 '{snapshot,pending_action}', '"confirm_references"')
	 WHERE id=$1`, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	s.ResolveReference = func(context.Context, identity.Workspace, string, string) (creation.Reference, llmclient.GenerateReference, error) {
		return creation.Reference{}, llmclient.GenerateReference{}, nil
	}
	out, _, err := s.Act(context.Background(), identity.Workspace{ID: ws}, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "confirm_references"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Snapshot.References) != 1 || !out.Snapshot.References[0].Confirmed || !out.Snapshot.References[0].Available {
		t.Fatalf("confirm_references did not restore availability: %+v", out.Snapshot.References)
	}
}

// TestCreationBatchGenerationInputsShape: generation_inputs for a text-only
// session must carry no "diagram" key, and a reference's manifest content
// (description/compatibility/allowed_tools) must not be stored a second time.
func TestCreationBatchGenerationInputsShape(t *testing.T) {
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-batch-inputs")
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立摘要 Skill。", "budget_usd": .5}, 200)
	v = creationStep(t, s, v)
	v = creationAct(t, c, v, "confirm_brief")
	v = creationStep(t, s, v)
	if v.Snapshot.Draft == nil || v.Snapshot.Draft.Blocked {
		t.Fatalf("no validated draft: %+v", v)
	}
	// Inject a synthetic confirmed reference directly (a real second generated
	// skill in this workspace would collide on the fixture's fixed draft name),
	// then stub ResolveReference so materialize's per-reference revalidation
	// (creation.go, case "materialize") accepts it.
	const refSkillID = "11111111-1111-1111-1111-111111111111"
	_, err := testPool.Exec(context.Background(), `UPDATE creation_sessions SET snapshot = jsonb_set(snapshot, '{snapshot,references}',
	 '[{"skill_id":"`+refSkillID+`","version_id":"22222222-2222-2222-2222-222222222222","name":"Ref","confirmed":true,"available":true}]'::jsonb)
	 WHERE id=$1`, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	stubResolve := func(context.Context, identity.Workspace, string, string) (creation.Reference, llmclient.GenerateReference, error) {
		return creation.Reference{}, llmclient.GenerateReference{}, nil
	}
	s.ResolveReference = stubResolve
	a.app.CreationSvc.ResolveReference = stubResolve
	v = creationAct(t, c, v, "materialize")
	if v.Snapshot.Candidate == nil {
		t.Fatal("candidate not committed")
	}

	var raw []byte
	err = testPool.QueryRow(context.Background(), `SELECT s.generation_inputs FROM skill_sources s
	 JOIN skill_versions ver ON ver.source_id = s.id WHERE ver.id = $1`, v.Snapshot.Candidate.VersionID).Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	var inputs map[string]any
	if err := json.Unmarshal(raw, &inputs); err != nil {
		t.Fatal(err)
	}
	if _, ok := inputs["diagram"]; ok {
		t.Fatalf("text-only session recorded a diagram: %s", raw)
	}
	refs, ok := inputs["references"].([]any)
	if !ok || len(refs) != 1 {
		t.Fatalf("expected one reference in generation_inputs: %s", raw)
	}
	ref, ok := refs[0].(map[string]any)
	if !ok {
		t.Fatalf("reference is not an object: %s", raw)
	}
	if _, ok := ref["description"]; ok {
		t.Fatalf("reference manifest content stored a second time: %s", raw)
	}
	if ref["skill_id"] != refSkillID {
		t.Fatalf("reference skill_id not recorded: %s", raw)
	}
}

func creationStatus(t *testing.T, c *client, body any) int {
	t.Helper()
	code, _ := creationStatusBody(t, c, body)
	return code
}
func creationStatusBody(t *testing.T, c *client, body any) (int, string) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Post(c.base+"/creation-sessions", "application/json", strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// TestCreationBatchForeignSessionIDIsNotAnOracle: GEN-011 — a session id is
// client-supplied, so reusing one that belongs to another workspace must
// behave exactly like using a fresh one (both 200, migration 0057's composite
// key makes them independent rows), never a status that reveals the foreign
// id already exists.
func TestCreationBatchForeignSessionIDIsNotAnOracle(t *testing.T) {
	a, _, _ := creationFixture(t)
	alice := a.login(t, "creation-batch-oracle-alice")
	bob := a.login(t, "creation-batch-oracle-bob")
	aliceID := creationID(t)
	creationPost(t, alice, "/creation-sessions", map[string]any{"id": aliceID, "message": "", "budget_usd": .5}, 200)
	if bob.status(t, "GET", "/creation-sessions/"+creation.UUID(aliceID)) != 404 {
		t.Fatal("bob could see alice's session before reusing its id")
	}
	reused, reusedBody := creationStatusBody(t, bob, map[string]any{"id": aliceID, "message": "", "budget_usd": .5})
	fresh := creationStatus(t, bob, map[string]any{"id": creationID(t), "message": "", "budget_usd": .5})
	if reused != fresh {
		t.Fatalf("reusing a foreign id gave a different status than a fresh one (GEN-011 oracle): reused=%d body=%s fresh=%d", reused, reusedBody, fresh)
	}
	if reused != 200 {
		t.Fatalf("want 200, got %d", reused)
	}
}

// TestCreationBatchAMaterialCarriesItsSentence: 2026-09-08. Until this, the
// `diagram` and `select_references` actions took no text, so 「這是我的流程，我想
// 把它變成一個 Skill」 could not be said in the same turn as the picture — the web
// composer's 「一次只能送一種素材」 was that gap surfacing, and the person paid for
// it with an extra round in which the model read a picture with no question
// attached. Both actions now append the sentence as the same user message the
// `message` action appends, before the material is applied.
//
// Driven over HTTP, not through the Service: the point includes that the route
// and its generated request type carry `message` alongside a diagram.
func TestCreationBatchAMaterialCarriesItsSentence(t *testing.T) {
	a, _, _ := creationFixture(t)
	c := a.login(t, "creation-batch-note")

	// A diagram and its sentence, one turn. The session is created unbilled
	// (empty message) exactly as the composer creates it when the first thing
	// the person sends is a picture.
	const withDiagram = "這是我的流程，我想把它變成待辦清單 Skill。"
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "", "budget_usd": .5}, 200)
	if len(v.Snapshot.Messages) != 0 {
		t.Fatalf("an unbilled session started with messages: %+v", v.Snapshot.Messages)
	}
	v = creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{
		"command_id": creationID(t), "expected_revision": v.Revision, "kind": "diagram",
		"message": withDiagram,
		"diagram": map[string]any{"media_type": "image/png", "data": "cG5n"},
	}, 200)
	if v.Snapshot.DiagramFingerprint == "" {
		t.Fatalf("the diagram itself was not accepted: %+v", v.Snapshot)
	}
	// The diagram step is transient - the API handler runs it inline - so the
	// model's own reply is already here too. What this asserts is that the
	// person's sentence is the FIRST thing in the history: the model read the
	// picture with the question attached, not after it.
	if len(v.Snapshot.Messages) == 0 || v.Snapshot.Messages[0].Role != "user" || v.Snapshot.Messages[0].Content != withDiagram {
		t.Fatalf("the sentence that came with the diagram is not in the history: %+v", v.Snapshot.Messages)
	}

	// The same for a reference selection. A fresh session, because the diagram
	// above left that one queued.
	const withRefs = "我想要和這個很像，但是輸出成表格。"
	v = creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "", "budget_usd": .5}, 200)
	a.app.CreationSvc.ResolveReference = func(context.Context, identity.Workspace, string, string) (creation.Reference, llmclient.GenerateReference, error) {
		return creation.Reference{SkillID: "33333333-3333-3333-3333-333333333333", VersionID: "44444444-4444-4444-4444-444444444444", Name: "Ref", Available: true}, llmclient.GenerateReference{}, nil
	}
	v = creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{
		"command_id": creationID(t), "expected_revision": v.Revision, "kind": "select_references",
		"message":             withRefs,
		"reference_skill_ids": []string{"33333333-3333-3333-3333-333333333333"},
	}, 200)
	if len(v.Snapshot.References) != 1 {
		t.Fatalf("the references themselves were not accepted: %+v", v.Snapshot)
	}
	if len(v.Snapshot.Messages) == 0 || v.Snapshot.Messages[0].Role != "user" || v.Snapshot.Messages[0].Content != withRefs {
		t.Fatalf("the sentence that came with the references is not in the history: %+v", v.Snapshot.Messages)
	}
	// Not asserted here: the 4000-rune ceiling. `CreationAction.message` already
	// carries `maxLength: 4000`, so over HTTP the contract refuses it first and a
	// test through this door would go green with `attachNote`'s own check
	// deleted. The check stays because the domain does not get to assume its only
	// caller is the generated server (iron rule 12 makes the contract the shape,
	// not the enforcement).
}

// TestCreationBatchEveryPictureKeepsItsPlaceInTheConversation: 2026-09-08.
// `diagram_fingerprint`/`media_type`/`bytes` are a latest-value — right for the
// model and for generation_inputs, wrong for a conversation, because a second
// upload erased the first one from the history entirely. `attachments` is that
// history, and each entry says which turn it belongs to so the web log can put
// the picture where it happened rather than guess.
//
// Metadata only. ADR-066 決策 4 keeps the digest and refuses the bytes, and its
// 2026-09-05 closing note answered 「維持不保存」; nothing here stores an image.
func TestCreationBatchEveryPictureKeepsItsPlaceInTheConversation(t *testing.T) {
	a, _, _ := creationFixture(t)
	c := a.login(t, "creation-batch-pictures")
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "", "budget_usd": .5}, 200)

	// With words: the picture belongs inside the person's own message.
	const said = "這是我的流程。"
	v = creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{
		"command_id": creationID(t), "expected_revision": v.Revision, "kind": "diagram",
		"message": said,
		"diagram": map[string]any{"media_type": "image/png", "data": "cG5n"},
	}, 200)
	if len(v.Snapshot.Attachments) != 1 {
		t.Fatalf("the first picture was not recorded: %+v", v.Snapshot.Attachments)
	}
	first := v.Snapshot.Attachments[0]
	if first.MediaType != "image/png" || first.Bytes != 3 || first.SHA256 != v.Snapshot.DiagramFingerprint {
		t.Fatalf("attachment does not describe what was sent: %+v", first)
	}
	if first.MessageIndex < 0 || first.MessageIndex >= len(v.Snapshot.Messages) {
		t.Fatalf("attachment points outside the history: %+v of %d", first, len(v.Snapshot.Messages))
	}
	if m := v.Snapshot.Messages[first.MessageIndex]; m.Role != "user" || m.Content != said {
		t.Fatalf("the picture is not on the turn it was sent with: %+v", m)
	}

	// Without words: it takes the index the model's own reply will take, so it
	// still sits between the two turns it happened between.
	before := len(v.Snapshot.Messages)
	v = creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{
		"command_id": creationID(t), "expected_revision": v.Revision, "kind": "diagram",
		"diagram": map[string]any{"media_type": "image/webp", "data": "d2VicA=="},
	}, 200)
	if len(v.Snapshot.Attachments) != 2 {
		t.Fatalf("the second picture erased the first instead of following it: %+v", v.Snapshot.Attachments)
	}
	if v.Snapshot.Attachments[0] != first {
		t.Fatalf("the first picture changed: %+v", v.Snapshot.Attachments[0])
	}
	second := v.Snapshot.Attachments[1]
	if second.MessageIndex != before {
		t.Fatalf("a wordless picture did not take the next index: %+v (history was %d)", second, before)
	}
	if second.MediaType != "image/webp" || second.SHA256 == first.SHA256 {
		t.Fatalf("the second attachment describes the first picture: %+v", second)
	}
	// And the latest-value fields still describe the newest picture: that is what
	// the model reads and what materialize records.
	if v.Snapshot.DiagramFingerprint != second.SHA256 || v.Snapshot.DiagramMediaType != "image/webp" {
		t.Fatalf("the newest-picture fields did not follow the newest picture: %+v", v.Snapshot)
	}
}

// TestCreationBatchStopEndsTheStepNotTheSession: 2026-09-09, 04 丙-203. Until
// this, the only brake on a step in flight was `cancel`, which ends the whole
// session. `stop_step` releases the attempt instead.
//
// The two paths differ in exactly one thing that matters — whether the money
// was already spent — and the session says which one happened. Before the call
// the receipt is finished as cancelled, which is also what makes the Worker
// refuse to start it. After the call has gone out the receipt is left alone, so
// finish() still settles the real cost; it simply no longer adopts the reply,
// because ActiveReceipt no longer points at it.
func TestCreationBatchStopEndsTheStepNotTheSession(t *testing.T) {
	a, _, _ := creationFixture(t)
	c := a.login(t, "creation-batch-stop")

	receiptOf := func(sessionID string) (string, string) {
		t.Helper()
		var id, status string
		// kind='attempt' matters: every command writes a receipt of its own, so
		// the newest row is the stop_step command, not the model attempt.
		err := testPool.QueryRow(context.Background(),
			"SELECT id::text, status FROM creation_receipts WHERE session_id=$1 AND kind='attempt' ORDER BY created_at DESC LIMIT 1",
			sessionID).Scan(&id, &status)
		if err != nil {
			t.Fatal(err)
		}
		return id, status
	}

	// Before the call: nothing has been paid for, and the session says so.
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立摘要 Skill。", "budget_usd": .5}, 200)
	if v.State != "queued" {
		t.Fatalf("a new session with a message should be queued: %+v", v.State)
	}
	if _, status := receiptOf(v.ID); status != "queued" {
		t.Fatalf("attempt is not queued: %s", status)
	}
	v = creationAct(t, c, v, "stop_step")
	if v.State != "waiting_input" {
		t.Fatalf("stop_step did not hand the turn back: %+v", v.State)
	}
	last := v.Snapshot.Messages[len(v.Snapshot.Messages)-1]
	if last.Role != "assistant" || !strings.Contains(last.Content, "沒有花到錢") {
		t.Fatalf("a stop before the call must say it cost nothing: %+v", last)
	}
	if _, status := receiptOf(v.ID); status != "cancelled" {
		t.Fatalf("the attempt was left for the Worker to pick up: %s", status)
	}

	// After the call has gone out: the receipt is NOT closed here, because the
	// Worker still has to record what it cost. Forced to `running` directly,
	// which is the state the Worker's own ClaimCreationReceipt puts it in.
	v = creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "另一個摘要 Skill。", "budget_usd": .5}, 200)
	receiptID, _ := receiptOf(v.ID)
	if _, err := testPool.Exec(context.Background(),
		"UPDATE creation_receipts SET status='running' WHERE id=$1", receiptID); err != nil {
		t.Fatal(err)
	}
	v = creationAct(t, c, v, "stop_step")
	if v.State != "waiting_input" {
		t.Fatalf("stop_step did not hand the turn back: %+v", v.State)
	}
	last = v.Snapshot.Messages[len(v.Snapshot.Messages)-1]
	if !strings.Contains(last.Content, "費用照計") {
		t.Fatalf("a stop after the call went out must not claim it was free: %+v", last)
	}
	if _, status := receiptOf(v.ID); status != "running" {
		t.Fatalf("closing a running receipt here would lose the spend: %s", status)
	}

	// And a stop with nothing in flight is not a command.
	code := creationStatus(t, c, map[string]any{"command_id": creationID(t), "expected_revision": v.Revision, "kind": "stop_step"})
	if code == 200 {
		t.Fatal("stop_step was accepted with no attempt in flight")
	}
}
