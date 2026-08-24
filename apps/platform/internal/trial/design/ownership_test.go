package testlab

import (
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestCreateTestCaseRefusesWithoutRegistryRead(t *testing.T) {
	if _, err := (&Service{}).CreateTestCase(t.Context(), identity.Workspace{}, gen.Workspace{}.ID, "name", "prompt"); err == nil {
		t.Error("CreateTestCase succeeded without Registry's skill reader")
	}
}

func TestPublishedFaceRefusesWithoutPersistence(t *testing.T) {
	svc := &Service{}
	id := gen.Workspace{}.ID
	checks := []struct {
		name string
		call func() error
	}{
		{"ReadDraft", func() error { _, err := svc.ReadDraft(t.Context(), id, id); return err }},
		{"ReadSnapshot", func() error { _, err := svc.ReadSnapshot(t.Context(), id, id); return err }},
		{"ReadDataset", func() error { _, err := svc.ReadDataset(t.Context(), id, id); return err }},
		{"CasesForSkill", func() error { _, err := svc.CasesForSkill(t.Context(), id, id); return err }},
		{"CaseDatasets", func() error { _, err := svc.CaseDatasets(t.Context(), id, id); return err }},
		{"LockDraft", func() error { _, err := svc.LockDraft(t.Context(), nil, id, id); return err }},
		{"CreateSnapshot", func() error { _, err := svc.CreateSnapshot(t.Context(), nil, id, id); return err }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.call(); err == nil {
				t.Fatal("call succeeded without Test Lab persistence")
			}
		})
	}
}
