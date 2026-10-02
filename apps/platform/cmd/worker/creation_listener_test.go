package main

import (
	"net"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/worker"
)

func validCreationLimits() creation.Limits {
	return creation.Limits{
		MaxCostUSD: 1, MaxCallCostUSD: .1, MaxSteps: 24, MaxToolCalls: 8,
		CallTimeout: 90 * time.Second, SessionTimeout: 72 * time.Hour,
		Retention: 30 * 24 * time.Hour, MaxOutputTokens: 16000,
	}
}

func TestTheCreationListenerStartsOnlyWithAnAddressATokenAndValidLimits(t *testing.T) {
	set := &worker.Set{Creation: &creation.Service{}}
	for _, tc := range []struct {
		name        string
		addr, token string
		limits      creation.Limits
		starts      bool
	}{
		{"everything present", "127.0.0.1:0", "internal-token", validCreationLimits(), true},
		{"no address", "", "internal-token", validCreationLimits(), false},
		{"no token", "127.0.0.1:0", "", validCreationLimits(), false},
		{"limits that do not validate", "127.0.0.1:0", "internal-token", creation.Limits{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CREATION_WORKER_INTERNAL_ADDR", tc.addr)
			t.Setenv("CREATION_WORKER_INTERNAL_TOKEN", tc.token)
			server, err := startCreationListener(set, tc.limits)
			if err != nil {
				t.Fatal(err)
			}
			if server != nil {
				t.Cleanup(func() { shutdownCreationListener(server) })
			}
			if (server != nil) != tc.starts {
				t.Fatalf("listener started = %v, want %v", server != nil, tc.starts)
			}
			if tc.starts && server.Addr != tc.addr {
				t.Errorf("listener address = %q, want %q", server.Addr, tc.addr)
			}
		})
	}
}

func TestACreationListenerThatCannotBindStopsTheWorker(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = taken.Close() })
	t.Setenv("CREATION_WORKER_INTERNAL_ADDR", taken.Addr().String())
	t.Setenv("CREATION_WORKER_INTERNAL_TOKEN", "internal-token")

	server, err := startCreationListener(&worker.Set{Creation: &creation.Service{}}, validCreationLimits())

	if err == nil || server != nil {
		if server != nil {
			shutdownCreationListener(server)
		}
		t.Fatalf("server = %v, err = %v: a worker whose hand-off address is taken started anyway", server, err)
	}
}
