package dockerdrv_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

func TestLargeGrantedObjectIsDeliveredByteForByte(t *testing.T) {
	d, _ := newDriver(t)
	payload := bytes.Repeat([]byte("x"), (2<<20)+17)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	req := testRequest(
		"while [ ! -f /work/.skillhub/ready ]; do sleep 0.05; done; " +
			"test \"$(wc -c < /work/.skillhub/skill.zip)\" -eq 2097169",
	)
	req.ObjectGrants = []sandbox.ObjectGrant{{
		Purpose: "skill_package", ObjectKey: "packages/large.zip", Access: "read",
		URL: srv.URL + "/packages/large.zip", ExpiresAt: time.Now().Add(time.Hour),
	}}

	id := handle(t)
	t.Cleanup(func() { _ = d.Remove(context.Background(), id) })
	if err := d.Start(context.Background(), id, req); err != nil {
		t.Fatalf("large input delivery failed: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := d.Wait(waitCtx, id)
	if err != nil || out.ExitCode != 0 {
		t.Fatalf("workload did not observe exact input: outcome=%+v err=%v", out, err)
	}
}

func TestAWorkloadThatEndsBeforeDeliveryIsNotAProvisionFailure(t *testing.T) {
	d, _ := newDriver(t)
	id := handle(t)
	t.Cleanup(func() { _ = d.Remove(context.Background(), id) })

	if err := d.Start(context.Background(), id, testRequest("true")); err != nil {
		t.Fatalf("start of a fast-exiting workload reported a delivery failure: %v", err)
	}
}

func TestAGrantedObjectThatCannotBeFetchedFailsTheDispatch(t *testing.T) {
	d, _ := newDriver(t)
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer dead.Close()

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

	if strings.Contains(err.Error(), dead.URL) {
		t.Errorf("the grant URL leaked into the error: %v", err)
	}
	if !strings.Contains(err.Error(), "skill_package") {
		t.Errorf("error does not name what could not be placed: %v", err)
	}
}
