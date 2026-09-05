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
	v = creationStep(t, s, v)
	v = creationAct(t, c, v, "confirm_brief")
	v = creationStep(t, s, v)
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
