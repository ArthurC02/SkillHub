package apiserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/providertest"
)

func TestALaterRunYieldsTheOnlyFreeSlotToTheRunAheadOfIt(t *testing.T) {
	s := newTurnScene(t, "later-yields")
	s.fake.SetFreeSlots(1)

	err := s.svc.Drive(s.ctx, mustUUID(t, s.carol.workspaceID), mustUUID(t, s.carolsQueuedLater))
	if !errors.Is(err, run.ErrTryAgainLater) {
		t.Fatalf("driving carol's later run returned %v, want it to wait its turn", err)
	}
	if s.fake.Dispatches() != 1 {
		t.Errorf("dispatches = %d, want only alice's: bob's earlier run is owed the free slot", s.fake.Dispatches())
	}
	if _, view := s.carol.getRun(t, s.carolsQueuedLater); view.Status != string(gen.RunStatusQueued) || len(view.Attempts) != 0 {
		t.Errorf("carol's run is %q with %d attempts, want queued with none", view.Status, len(view.Attempts))
	}
}

func sandboxesFailingWhilePrepared(t *testing.T, fake *providertest.Fake) *httptest.Server {
	t.Helper()
	target, err := url.Parse(fake.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ModifyResponse = func(resp *http.Response) error {
		if resp.Request.Method != http.MethodPost || resp.Request.URL.Path != "/runs" || resp.StatusCode != http.StatusCreated {
			return nil
		}
		var body map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return err
		}
		_ = resp.Body.Close()
		body["state"] = string(run.ProviderStateFailed)
		body["state_reason"] = "the image could not be pulled"
		rewritten, err := json.Marshal(body)
		if err != nil {
			return err
		}
		resp.Body = io.NopCloser(bytes.NewReader(rewritten))
		resp.ContentLength = int64(len(rewritten))
		resp.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
		return nil
	}
	front := httptest.NewServer(proxy)
	t.Cleanup(front.Close)
	return front
}

func TestASandboxThatFailsWhileBeingPreparedIsRetriedAndThenFailsTheRun(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-failed-provisioning")
	clearRunBacklog(t, pool)
	fake := providertest.New("fake_sandbox", "test-token")
	t.Cleanup(fake.Close)
	front := sandboxesFailingWhilePrepared(t, fake)
	registry := run.NewRegistry(run.NewProvider(fake.Name, front.URL, fake.Token))
	a.runs.Providers = registry
	svc := *a.runs
	svc.Providers = registry
	svc.Store = a.packages
	svc.PollInterval = 20 * time.Millisecond
	svc.SlotWaitInterval = 20 * time.Millisecond
	svc.MaxAttempts = 2
	startWorkerWith(t, &svc, a.evaluations)

	created := f.start(t)
	final := waitForStatus(t, f.client, created.RunID, string(gen.RunStatusFailed))
	if final.FailureClass.Value != "provider_error" || !strings.Contains(final.StatusReason, "準備階段") {
		t.Errorf("failure %q (%s), want provider_error naming the preparation", final.FailureClass.Value, final.StatusReason)
	}
	if len(final.Attempts) != 2 || fake.Dispatches() != 2 {
		t.Errorf("attempts = %d, dispatches = %d; want both allowed attempts spent", len(final.Attempts), fake.Dispatches())
	}
}

func TestASandboxWhoseDispatchCannotBeRecordedIsDestroyed(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-unrecorded-dispatch")
	ctx := context.Background()
	created := f.start(t)
	if _, err := pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION skillhub_test_refuse_dispatch_record() RETURNS trigger AS $fn$
		BEGIN
			IF NEW.workspace_id = `+pgLiteral(f.workspaceID)+`::uuid THEN
				RAISE EXCEPTION 'dispatch record refused by test';
			END IF;
			RETURN NEW;
		END;
		$fn$ LANGUAGE plpgsql`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER skillhub_test_refuse_dispatch_record BEFORE UPDATE ON run_attempts
		FOR EACH ROW WHEN (OLD.provider_run_id IS NULL AND NEW.provider_run_id IS NOT NULL)
		EXECUTE FUNCTION skillhub_test_refuse_dispatch_record()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DROP TRIGGER IF EXISTS skillhub_test_refuse_dispatch_record ON run_attempts")
		_, _ = pool.Exec(ctx, "DROP FUNCTION IF EXISTS skillhub_test_refuse_dispatch_record()")
	})
	fake := providertest.New("fake_sandbox", "test-token")
	t.Cleanup(fake.Close)
	svc := *a.runs
	svc.Providers = run.NewRegistry(fake.Provider())
	svc.Store = a.packages

	err := svc.Drive(ctx, mustUUID(t, f.workspaceID), mustUUID(t, created.RunID))
	if err == nil || errors.Is(err, run.ErrTryAgainLater) {
		t.Fatalf("driving a run whose dispatch cannot be recorded returned %v, want the recording error", err)
	}
	if fake.Dispatches() != 1 || fake.Live() != 0 {
		t.Errorf("dispatches = %d, live sandboxes = %d; want the one sandbox destroyed", fake.Dispatches(), fake.Live())
	}
}

func TestAProviderRefusalThatWaitingCannotFixEndsTheRunAfterOneAttempt(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-final-refusal")
	fake, svc := withProvider(t, a, pool, providertest.Plan{})
	svc.MaxAttempts = 3
	fake.DispatchStatuses = []int{http.StatusBadRequest, http.StatusBadRequest, http.StatusBadRequest}

	created := f.start(t)
	final := waitForStatus(t, f.client, created.RunID, string(gen.RunStatusFailed))
	if final.FailureClass.Value != "provider_error" {
		t.Errorf("failure_class = %q, want provider_error", final.FailureClass.Value)
	}
	if len(final.Attempts) != 1 {
		t.Errorf("attempts = %d, want 1: a refusal that is not about load is not retried", len(final.Attempts))
	}
}

func TestARunWhoseModelGatewayIsNotAnsweringWaitsInTheQueueInsteadOfFailing(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-gateway-down")
	ctx := context.Background()
	created := f.start(t)

	var down atomic.Bool
	down.Store(true)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"key":"sk-test"}`))
	}))
	t.Cleanup(gateway.Close)
	fake := providertest.New("fake_sandbox", "test-token")
	t.Cleanup(fake.Close)
	svc := *a.runs
	svc.Providers = run.NewRegistry(fake.Provider())
	svc.Store = a.packages
	svc.Gateway = run.NewGateway(run.GatewayConfig{SandboxBaseURL: gateway.URL, AdminKey: "test-admin-key"})
	ws, runID := mustUUID(t, f.workspaceID), mustUUID(t, created.RunID)

	if err := svc.Drive(ctx, ws, runID); !errors.Is(err, run.ErrTryAgainLater) {
		t.Fatalf("driving with the gateway down returned %v, want the run to wait", err)
	}
	if _, view := f.getRun(t, created.RunID); view.Status != string(gen.RunStatusQueued) || len(view.Attempts) != 1 {
		t.Fatalf("run is %q with %d attempts, want queued with the one attempt that could not get a key",
			view.Status, len(view.Attempts))
	}
	if fake.Dispatches() != 0 {
		t.Errorf("dispatches = %d, want none: the attempt had no key to give the sandbox", fake.Dispatches())
	}

	down.Store(false)
	_ = svc.Drive(ctx, ws, runID)
	if fake.Dispatches() != 1 {
		t.Errorf("dispatches = %d after the gateway answered again, want 1", fake.Dispatches())
	}
}
