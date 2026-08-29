package run

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Registry.Capability held the registry mutex across the HTTP call to the node.
// The provider client's own timeout is 30 seconds, so one black-holed node
// serialised every scheduling decision and every pre-run permission summary in
// the process behind it: SelectExcluding on each dispatch, providerSummary on
// each preflight screen, EvaluateOrphanThresholds every five minutes and
// detectP02Breach every thirty seconds all queue on the same lock. The API and
// the worker each hold their own Registry, so the visible symptom was "one node
// went dark and the preflight endpoint stopped answering for everybody".
//
// The assertion is about the lock and nothing else: a second caller asking about
// a DIFFERENT provider must get its answer while the first is still hanging.
func TestCapabilityDoesNotHoldTheRegistryLockAcrossTheNetwork(t *testing.T) {
	blocked := make(chan struct{})
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocked // a TCP black hole with a socket, which is the case that hurt
	}))
	defer hang.Close()
	defer close(blocked)

	quick := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"provider":"quick","runtimes":[],"max_resources":{},"isolation":{"level":"gvisor"}}`))
	}))
	defer quick.Close()

	slow := NewProvider("slow", hang.URL, "")
	fast := NewProvider("quick", quick.URL, "")
	registry := NewRegistry(slow, fast)

	// The first caller is inside p.Capability and staying there.
	entered := make(chan struct{})
	go func() {
		close(entered)
		_, _ = registry.Capability(context.Background(), slow)
	}()
	<-entered

	answered := make(chan error, 1)
	go func() {
		_, err := registry.Capability(context.Background(), fast)
		answered <- err
	}()
	select {
	case err := <-answered:
		if err != nil {
			t.Fatalf("the reachable provider answered with an error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a capability read for a reachable provider waited on an unreachable one; " +
			"the registry lock is being held across the HTTP call")
	}
}

// The TTL cache still works, failures included: a provider that is down must not
// be re-probed once per scheduling decision, which is the whole reason the cache
// exists and the thing a naive "just drop the lock" change loses.
func TestCapabilityStillCachesTheAnswerAndTheFailure(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := NewProvider("flaky", srv.URL, "")
	registry := NewRegistry(p)
	registry.TTL = time.Minute

	for range 5 {
		if _, err := registry.Capability(context.Background(), p); err == nil {
			t.Fatal("a 500 from the node was reported as a capability")
		}
	}
	if hits != 1 {
		t.Errorf("the node was probed %d times for 5 sequential reads; the failure cache is not holding", hits)
	}

	// And the TTL is real: Refresh drops it, so the next read asks again.
	registry.Refresh()
	if _, err := registry.Capability(context.Background(), p); err == nil {
		t.Fatal("a 500 from the node was reported as a capability")
	}
	if hits != 2 {
		t.Errorf("probes after Refresh = %d, want 2; the cache outlived the thing that clears it", hits)
	}
}
