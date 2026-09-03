package run

import (
	"context"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

func TestRequireTestLabDoesNotInspectOwnerInternals(t *testing.T) {
	if err := (&Service{TestLab: &testlab.Service{}}).requireTestLab(); err != nil {
		t.Fatalf("requireTestLab rejected an injected owner service: %v", err)
	}
}

func TestDeleteArtifactRefusesWithoutOwnerCounter(t *testing.T) {
	ws := identity.Workspace{}
	if err := (&Service{}).DeleteArtifact(context.Background(), ws, ws.ID, ws.ID); err == nil {
		t.Error("DeleteArtifact succeeded without packaging's artifact reference counter")
	}
}

func TestMaskingActivityRefusesWithoutTraceService(t *testing.T) {
	if _, err := (&Service{}).maskingActivity(t.Context(), time.Now(), time.Now()); err == nil {
		t.Error("masking activity succeeded without Trace owner service")
	}
}

func TestPermissionSummaryRefusesWithoutRegistryRead(t *testing.T) {
	if _, err := (&Service{}).PermissionSummaryFor(t.Context(), gen.Workspace{}.ID, gen.Workspace{}.ID, gen.Workspace{}.ID, gen.Workspace{}.ID); err == nil {
		t.Error("PermissionSummaryFor succeeded without Registry's version reader")
	}
}
