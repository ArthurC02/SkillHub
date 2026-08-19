package dockerdrv_test

// SBX-008 input delivery, against real containers — the CI failure this covers
// was a workload that had already exited by the time the first exec ran, which
// the daemon reports in three different shapes depending on how far the exec got
// (see Driver.exec). A unit test over a fake could not have produced any of them.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

// A workload that ends before its inputs could be placed has not been failed by
// the delivery: it ran and finished on its own terms, and Wait is about to
// report what it actually did. Reporting a provision failure instead would
// replace the real outcome with a misleading one.
func TestAWorkloadThatEndsBeforeDeliveryIsNotAProvisionFailure(t *testing.T) {
	d, _ := newDriver(t)
	id := handle(t)
	t.Cleanup(func() { _ = d.Remove(context.Background(), id) })

	// Exits immediately, ignores the handshake, carries no grants: exactly the
	// shape of the isolation probes.
	if err := d.Start(context.Background(), id, testRequest("true")); err != nil {
		t.Fatalf("start of a fast-exiting workload reported a delivery failure: %v", err)
	}
}

// The other half: an object the run *was* granted and that could not be placed
// is fatal, because the workload would otherwise run without something it was
// given (SBX-008 is fail-closed).
func TestAGrantedObjectThatCannotBeFetchedFailsTheDispatch(t *testing.T) {
	d, _ := newDriver(t)
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden) // an expired or wrong grant
	}))
	defer dead.Close()

	// Long-lived, so the container is unambiguously alive when the fetch fails.
	req := testRequest("sleep 30")
	req.ObjectGrants = []sandbox.ObjectGrant{{
		Purpose: "skill_package", ObjectKey: "packages/test.zip", Access: "read",
		URL: dead.URL + "/packages/test.zip", ExpiresAt: time.Now().Add(time.Hour),
	}}

	id := handle(t)
	t.Cleanup(func() { _ = d.Remove(context.Background(), id) })
	err := d.Start(context.Background(), id, req)
	if err == nil {
		t.Fatal("a skill package that could not be fetched was treated as delivered")
	}
	// The URL is the authorization and must not travel in an error (iron rule 11).
	if strings.Contains(err.Error(), dead.URL) {
		t.Errorf("the grant URL leaked into the error: %v", err)
	}
	if !strings.Contains(err.Error(), "skill_package") {
		t.Errorf("error does not name what could not be placed: %v", err)
	}
}
