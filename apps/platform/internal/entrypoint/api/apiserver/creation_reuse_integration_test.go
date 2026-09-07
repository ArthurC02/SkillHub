package apiserver_test

import (
	"context"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
)

// Re-Use before creation (05 R-49／R-50, 2026-09-06): Go asks the catalogue
// before the first model call and before anything is stored, and the person
// may take an existing Skill instead of composing one.

// seedExistingSkill runs one whole session to its saved candidate: the
// "existing Skill" the catalogue check will offer.
func seedExistingSkill(t *testing.T, a *api, s *creation.Service, c *client) creation.Candidate {
	t.Helper()
	markCatalog(t, testPool, c.workspaceID)
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "建立可重用的範本", "budget_usd": .5}, 200)
	v = creationStep(t, s, v)
	v = creationAct(t, c, v, "confirm_brief")
	v = creationStep(t, s, v)
	v = creationAct(t, c, v, "finalize")
	if v.State != "saved" || v.Snapshot.Candidate == nil {
		t.Fatalf("seed session did not save: %+v", v)
	}
	return *v.Snapshot.Candidate
}

func TestCreationFirstMessageCatalogueCheckOffersAdoptKeepOrDecline(t *testing.T) {
	a, s, calls := creationFixture(t)
	c := a.login(t, "creation-reuse-first")
	// The existing Skill is another workspace's catalogue entry: what a person
	// finds, not what they already own.
	existing := seedExistingSkill(t, a, s, a.login(t, "creation-reuse-first-owner"))
	stepsBefore := calls.Load()
	var checked []string
	a.app.CreationSvc.CatalogCheck = func(_ context.Context, _ identity.Workspace, query string) ([]creation.Reference, float64, error) {
		checked = append(checked, query)
		return []creation.Reference{{SkillID: existing.SkillID, VersionID: existing.VersionID, Name: "creation-summary", Available: true, Confirmed: true}}, 0.00001, nil
	}
	start := func() creation.View {
		return creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "幫我整理輸入資料並輸出摘要", "budget_usd": .5}, 200)
	}

	v := start()
	if v.State != "waiting_confirmation" || v.Snapshot.PendingAction != "confirm_references" || !v.Snapshot.CatalogChecked ||
		len(v.Snapshot.References) != 1 || v.Snapshot.References[0].SkillID != existing.SkillID || v.Snapshot.References[0].Confirmed ||
		v.Snapshot.SpentUSD == nil || *v.Snapshot.SpentUSD != 0.00001 || calls.Load() != stepsBefore {
		t.Fatalf("the catalogue hit must wait for the person before any model call: %+v calls=%d", v.Snapshot, calls.Load()-stepsBefore)
	}
	if len(checked) != 1 || checked[0] != "幫我整理輸入資料並輸出摘要" {
		t.Fatalf("the first message is the query: %v", checked)
	}
	var jobs int
	if err := testPool.QueryRow(context.Background(), "SELECT count(*) FROM river_job WHERE kind='creation_step' AND args->>'session_id'=$1", v.ID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 0 {
		t.Fatalf("a held session must not have a step queued: %d", jobs)
	}

	// Decline: the model is told, the session goes to the model with no reference.
	d := creationAct(t, c, v, "decline_references")
	last := d.Snapshot.Messages[len(d.Snapshot.Messages)-1]
	if d.State != "queued" || len(d.Snapshot.References) != 0 || last.Role != "tool" || !strings.Contains(last.Content, "不採用") {
		t.Fatalf("decline: %+v", d.Snapshot)
	}

	// Adopt: the fork is the candidate, nothing is composed, the session ends.
	v = start()
	ad := creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{"command_id": creationID(t), "expected_revision": v.Revision, "kind": "adopt_reference", "reference_skill_ids": []string{existing.SkillID}}, 200)
	if ad.State != "saved" || !ad.Snapshot.Adopted || ad.Snapshot.Candidate == nil || ad.Snapshot.Candidate.SkillID == existing.SkillID || ad.Snapshot.PendingAction != "" {
		t.Fatalf("adopt: %+v", ad.Snapshot)
	}
	var forkedFrom string
	if err := testPool.QueryRow(context.Background(), "SELECT forked_from_skill_id::text FROM skills WHERE id=$1", ad.Snapshot.Candidate.SkillID).Scan(&forkedFrom); err != nil || forkedFrom != existing.SkillID {
		t.Fatalf("the candidate must be a fork of the adopted Skill: %q err=%v", forkedFrom, err)
	}
	if calls.Load() != stepsBefore {
		t.Fatalf("adoption must cost no model call: %d", calls.Load()-stepsBefore)
	}

	// Only what Go offered may be adopted.
	v = start()
	creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{"command_id": creationID(t), "expected_revision": v.Revision, "kind": "adopt_reference", "reference_skill_ids": []string{"11111111-1111-4111-8111-111111111111"}}, 422)

	// Keep: the hits become confirmed references and the model is called.
	k := creationAct(t, c, v, "confirm_references")
	if k.State != "queued" || len(k.Snapshot.References) != 1 || !k.Snapshot.References[0].Confirmed {
		t.Fatalf("keep as references: %+v", k.Snapshot)
	}
}

func TestCreationMaterializeHoldsForADuplicateUntilAdoptedOrConfirmed(t *testing.T) {
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-reuse-dup")
	existing := seedExistingSkill(t, a, s, a.login(t, "creation-reuse-dup-owner"))
	// The draft's description is what the guard embeds; the first message is not
	// close to anything (the first-message check is a separate, looser search).
	a.app.CreationSvc.CatalogCheck = func(context.Context, identity.Workspace, string) ([]creation.Reference, float64, error) {
		return nil, 0, nil
	}
	a.app.CreationSvc.DuplicateCheck = func(_ context.Context, _ identity.Workspace, query string) ([]creation.Reference, float64, error) {
		if strings.Contains(query, "Summarize user input") {
			return []creation.Reference{{SkillID: existing.SkillID, VersionID: existing.VersionID, Name: "creation-summary-existing", Available: true}}, 0.00002, nil
		}
		return nil, 0, nil
	}
	drafted := func(c *client) creation.View {
		v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "再做一個摘要 Skill", "budget_usd": .5}, 200)
		if v.State != "queued" || v.Snapshot.PendingAction != "" {
			t.Fatalf("the first message must not be held: %+v", v.Snapshot)
		}
		v = creationStep(t, s, v)
		v = creationAct(t, c, v, "confirm_brief")
		v = creationStep(t, s, v)
		if v.Snapshot.Draft == nil {
			t.Fatalf("no draft: %+v", v.Snapshot)
		}
		return v
	}

	v := drafted(c)
	spentBefore := *v.Snapshot.SpentUSD
	held := creationAct(t, c, v, "materialize")
	if held.State != "waiting_confirmation" || held.Snapshot.PendingAction != "confirm_duplicate" || held.Snapshot.PendingMaterialize != "materialize" ||
		len(held.Snapshot.Duplicates) != 1 || held.Snapshot.Duplicates[0].SkillID != existing.SkillID || held.Snapshot.Candidate != nil ||
		*held.Snapshot.SpentUSD != spentBefore+0.00002 {
		t.Fatalf("a near-duplicate must hold materialize: %+v", held.Snapshot)
	}
	// Confirm anyway: the held command runs, the answer is kept for the record.
	done := creationAct(t, c, held, "confirm_duplicate")
	if done.State != "candidate_ready" || done.Snapshot.Candidate == nil || !done.Snapshot.DuplicateAcknowledged || done.Snapshot.PendingAction != "" ||
		done.Snapshot.PendingMaterialize != "" || len(done.Snapshot.Duplicates) != 1 || *done.Snapshot.SpentUSD != spentBefore+0.00002 {
		t.Fatalf("confirm_duplicate must materialize the held command: %+v", done.Snapshot)
	}
	// A second confirmation has nothing to replay.
	creationPost(t, c, "/creation-sessions/"+done.ID+"/actions", map[string]any{"command_id": creationID(t), "expected_revision": done.Revision, "kind": "confirm_duplicate", "content_hash": done.Snapshot.Draft.ContentHash}, 422)

	// Adopt the duplicate instead of storing the draft.
	v = drafted(c)
	held = creationAct(t, c, v, "materialize")
	ad := creationPost(t, c, "/creation-sessions/"+held.ID+"/actions", map[string]any{"command_id": creationID(t), "expected_revision": held.Revision, "kind": "adopt_reference", "reference_skill_ids": []string{existing.SkillID}}, 200)
	if ad.State != "saved" || !ad.Snapshot.Adopted || ad.Snapshot.Candidate == nil || ad.Snapshot.Candidate.SkillID == existing.SkillID {
		t.Fatalf("adopt from the duplicate list: %+v", ad.Snapshot)
	}

	// "Build anyway" with the duplicate's own name: Go asks the model to rename
	// instead of letting the save be refused as 同名 (run x R09).
	c4 := a.login(t, "creation-reuse-dup-rename")
	a.app.CreationSvc.DuplicateCheck = func(_ context.Context, _ identity.Workspace, query string) ([]creation.Reference, float64, error) {
		if strings.Contains(query, "Summarize user input") {
			return []creation.Reference{{SkillID: existing.SkillID, VersionID: existing.VersionID, Name: "creation-summary", Available: true}}, 0.00002, nil
		}
		return nil, 0, nil
	}
	v = drafted(c4)
	held = creationAct(t, c4, v, "materialize")
	renaming := creationAct(t, c4, held, "confirm_duplicate")
	if renaming.State != "queued" || renaming.Snapshot.Candidate != nil || !strings.Contains(renaming.Snapshot.Messages[len(renaming.Snapshot.Messages)-1].Content, "請只改名稱") {
		t.Fatalf("a colliding name must go back to the model: %+v", renaming.Snapshot)
	}
	renamed := creationStep(t, s, renaming)
	if renamed.Snapshot.Draft == nil || renamed.Snapshot.Draft.Skill.Name != "creation-summary-renamed" || !renamed.Snapshot.DuplicateAcknowledged {
		t.Fatalf("renamed draft: %+v", renamed.Snapshot)
	}
	if saved := creationAct(t, c4, renamed, "materialize"); saved.State != "candidate_ready" || saved.Snapshot.Candidate == nil {
		t.Fatalf("the renamed draft saves without a second duplicate hold: %+v", saved.Snapshot)
	}

	// A held finalize resumes as finalize (a workspace of its own: the first
	// session above already saved this draft's name into c's). The duplicate
	// offered here does not share the draft's name, so no rename round.
	a.app.CreationSvc.DuplicateCheck = func(_ context.Context, _ identity.Workspace, query string) ([]creation.Reference, float64, error) {
		if strings.Contains(query, "Summarize user input") {
			return []creation.Reference{{SkillID: existing.SkillID, VersionID: existing.VersionID, Name: "creation-summary-existing", Available: true}}, 0.00002, nil
		}
		return nil, 0, nil
	}
	c3 := a.login(t, "creation-reuse-dup-finalize")
	v = drafted(c3)
	held = creationAct(t, c3, v, "finalize")
	if held.Snapshot.PendingMaterialize != "finalize" {
		t.Fatalf("finalize must be the held command: %+v", held.Snapshot)
	}
	if fin := creationAct(t, c3, held, "confirm_duplicate"); fin.State != "saved" || fin.Snapshot.Candidate == nil {
		t.Fatalf("confirmed finalize must save: %+v", fin.Snapshot)
	}
}

// 05 SEC-013 (LLM02): what the session stores of the person's own words is
// masked on the way in, the way a trace is (TRACE-005).
func TestCreationMasksCredentialsInTheStoredConversation(t *testing.T) {
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-mask")
	key := "sk-proj-" + strings.Repeat("A", 28)
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "用這把金鑰 " + key + " 讀資料", "budget_usd": .5}, 200)
	if got := v.Snapshot.Messages[0].Content; strings.Contains(got, key) || !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("the first message stored the credential: %q", got)
	}
	v = creationStep(t, s, v)
	v = creationMessage(t, c, v, "另一把 "+key)
	if got := v.Snapshot.Messages[len(v.Snapshot.Messages)-1].Content; strings.Contains(got, key) {
		t.Fatalf("a later message stored the credential: %q", got)
	}
}

// 05 SEC-013 (LLM05): a draft that writes outside its own package is refused
// at materialize, from the creation path and not only from an upload.
func TestCreationRefusesADraftThatEscapesItsPackage(t *testing.T) {
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-escape")
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "做一個摘要 Skill，順便測路徑穿越", "budget_usd": .5}, 200)
	v = creationStep(t, s, v)
	v = creationAct(t, c, v, "confirm_brief")
	v = creationStep(t, s, v)
	if v.Snapshot.Draft == nil || len(v.Snapshot.Draft.Skill.Files) != 1 {
		t.Fatalf("the stub did not plant the escaping file: %+v", v.Snapshot.Draft)
	}
	if !v.Snapshot.Draft.Blocked {
		// Static validation is the first wall; the save is the second.
		creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{"command_id": creationID(t), "expected_revision": v.Revision, "kind": "materialize", "content_hash": v.Snapshot.Draft.ContentHash}, 422)
	}
	var versions int
	if err := testPool.QueryRow(context.Background(), "SELECT count(*) FROM skill_versions v JOIN skills sk ON sk.id = v.skill_id WHERE sk.workspace_id = $1", c.workspaceID).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if versions != 0 {
		t.Fatalf("an escaping draft became a version: %d", versions)
	}
}
