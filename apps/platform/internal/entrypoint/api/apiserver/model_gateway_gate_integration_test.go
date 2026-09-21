package apiserver_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/providertest"
)

func TestARunWithNoWayToReachAModelIsRefusedBeforeItReachesAnySandbox(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	fake, svc := haltHarness(t, a, pool)
	f := newFixture(t, a, pool, "no-model-gateway")
	ctx := context.Background()
	ws := mustUUID(t, f.workspaceID)

	svc.Gateway = nil
	refused := f.start(t)
	if err := driveThroughPolls(ctx, svc.Drive, ws, mustUUID(t, refused.RunID)); err != nil {
		t.Fatalf("driving a run with no model gateway returned an error: %v", err)
	}

	code, view := f.getRun(t, refused.RunID)
	if code != http.StatusOK {
		t.Fatalf("GET run: %d", code)
	}
	if view.Status != string(gen.RunStatusSucceeded) && view.Status != string(gen.RunStatusFailed) {
		t.Fatalf("status = %q (%s), want a settled run", view.Status, view.StatusReason)
	}
	if view.Status == string(gen.RunStatusSucceeded) {
		t.Fatalf("a run that never had a way to reach a model reported success (%s); "+
			"a failure gets asked about, a green light does not", view.StatusReason)
	}
	if !strings.Contains(view.StatusReason, "模型閘道") {
		t.Errorf("status reason = %q, want it to name the missing model gateway", view.StatusReason)
	}
	if fake.Dispatches() != 0 {
		t.Fatalf("dispatches = %d; a run with no way to reach a model still took a sandbox", fake.Dispatches())
	}

	svc.Gateway = providertest.NewGateway()
	accepted := f.start(t)
	if err := driveThroughPolls(ctx, svc.Drive, ws, mustUUID(t, accepted.RunID)); err != nil {
		t.Fatalf("driving a run with a model gateway: %v", err)
	}
	if _, view := f.getRun(t, accepted.RunID); view.Status != string(gen.RunStatusSucceeded) {
		t.Fatalf("a run with a model gateway ended as %q (%s), want succeeded", view.Status, view.StatusReason)
	}
	if fake.Dispatches() != 1 {
		t.Errorf("dispatches = %d, want 1: the gate kept out a run it should have let through", fake.Dispatches())
	}
}

func TestADeploymentWithNoModelOutletRefusesToStartARunAndSaysSoBeforeConfirming(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	a.runs.Deployment.GatewayURL = ""
	f := newFixture(t, a, pool, "no-model-outlet-start")

	hash := f.confirmPermissions(t)
	code, view := f.startWithHash(t, hash)

	if code != http.StatusUnprocessableEntity {
		t.Fatalf("POST run: got %d (%s), want 422: this deployment cannot reach a model", code, view.Error)
	}
	if !strings.Contains(view.Error, "沒有接上模型閘道") {
		t.Errorf("error = %q, want it to name in Chinese the outlet this deployment is missing", view.Error)
	}
	if strings.Contains(view.Error, "model gateway") {
		t.Errorf("error = %q, still carries the English sentence meant for the log", view.Error)
	}
}
