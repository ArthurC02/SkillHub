package registry

import (
	"context"
	"testing"
)

func TestWorkspaceSkillsReturnsOnlyTheAskedPairsThatAreLive(t *testing.T) {
	pool := requireRegistryDB(t)
	ctx := context.Background()
	wsA, skillA := seedSkill(t, pool, "batch-read-a")
	wsB, skillB := seedSkill(t, pool, "batch-read-b")
	wsC, skillC := seedSkill(t, pool, "batch-read-deleted")
	if _, err := pool.Exec(ctx, "UPDATE skills SET deleted_at = now() WHERE id = $1", skillC); err != nil {
		t.Fatal(err)
	}
	svc := &Service{Pool: pool}

	got, err := svc.WorkspaceSkills(ctx, []SkillRef{
		{WorkspaceID: wsA.ID, SkillID: skillA},
		{WorkspaceID: wsA.ID, SkillID: skillB},
		{WorkspaceID: wsB.ID, SkillID: skillA},
		{WorkspaceID: wsC.ID, SkillID: skillC},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d skills, want only skill A in workspace A (crossed pairs and a deleted skill excluded)", len(got))
	}
	if skill, ok := got[SkillRef{WorkspaceID: wsA.ID, SkillID: skillA}]; !ok || skill.ID != skillA || skill.Name != "batch-read-a" {
		t.Fatalf("skill A in workspace A = %+v, %v", skill, ok)
	}
}
