package main

import (
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/worker"
)

func TestTheCreationListenerBoundsHeaderReadingBodyReadingAndIdleConnections(t *testing.T) {
	t.Setenv("CREATION_WORKER_INTERNAL_ADDR", "127.0.0.1:0")
	t.Setenv("CREATION_WORKER_INTERNAL_TOKEN", "internal-token")
	server, err := startCreationListener(&worker.Set{Creation: &creation.Service{}}, validCreationLimits())
	if err != nil || server == nil {
		t.Fatalf("listener did not start: %v", err)
	}
	t.Cleanup(func() { shutdownCreationListener(server) })
	if server.ReadHeaderTimeout != 5*time.Second || server.ReadTimeout != 10*time.Second || server.IdleTimeout != 30*time.Second {
		t.Errorf("timeouts = header %v, body %v, idle %v; want 5s, 10s, 30s",
			server.ReadHeaderTimeout, server.ReadTimeout, server.IdleTimeout)
	}
}
