package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestResidentP02ProbeFallsBackToItsDefaultCadenceWithNoTargets(t *testing.T) {
	t.Setenv("SKILLHUB_SANDBOX_P02_TARGETS", "")
	t.Setenv("SKILLHUB_SANDBOX_P02_INTERVAL_SECONDS", "")
	t.Setenv("SKILLHUB_SANDBOX_P02_TIMEOUT_SECONDS", "")

	probe := residentP02Probe()

	if probe.Configured() {
		t.Errorf("targets = %v, want none configured", probe.Targets)
	}
	if probe.Interval != 5*time.Minute {
		t.Errorf("interval = %v, want 5m0s", probe.Interval)
	}
	if probe.Timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", probe.Timeout)
	}
}

func TestResidentP02ProbeReadsItsTargetsAndCadenceFromTheEnvironment(t *testing.T) {
	t.Setenv("SKILLHUB_SANDBOX_P02_TARGETS", "169.254.169.254:80, 10.0.0.5:5432")
	t.Setenv("SKILLHUB_SANDBOX_P02_INTERVAL_SECONDS", "60")
	t.Setenv("SKILLHUB_SANDBOX_P02_TIMEOUT_SECONDS", "5")

	probe := residentP02Probe()

	if want := []string{"10.0.0.5:5432", "169.254.169.254:80"}; !slices.Equal(probe.Targets, want) {
		t.Errorf("targets = %v, want %v", probe.Targets, want)
	}
	if probe.Interval != time.Minute {
		t.Errorf("interval = %v, want 1m0s", probe.Interval)
	}
	if probe.Timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", probe.Timeout)
	}
}

func TestDeclaredEgressDependsOnTheNodeKindAndItsRenderedRoutes(t *testing.T) {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tc := range []struct {
		name      string
		kind      string
		cleanMode cleanNode
		routed    bool
		wantModes []string
		wantLoose bool
	}{
		{name: "docker node with no network", kind: driverDocker, wantModes: []string{"none"}},
		{name: "docker node with a rendered route", kind: driverDocker, routed: true,
			wantModes: []string{"default_deny", "none"}},
		{name: "mxc node never declares a route it cannot enforce", kind: driverMXC, routed: true,
			wantModes: []string{"none"}},
		{name: "clean node declares both modes as unenforced", kind: driverLocal, cleanMode: true,
			wantModes: []string{"default_deny", "none"}, wantLoose: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SKILLHUB_SANDBOX_NETWORK", "")
			wantRoutes := 0
			if tc.routed {
				t.Setenv("SKILLHUB_SANDBOX_NETWORK", "skillhub_egress")
				t.Setenv("SKILLHUB_SANDBOX_EGRESS_ALLOW", renderedAllowList(t))
				wantRoutes = 1
			}

			modes, allow, unenforced := declaredEgress(tc.kind, tc.cleanMode, quiet)

			if !slices.Equal(modes, tc.wantModes) {
				t.Errorf("modes = %v, want %v", modes, tc.wantModes)
			}
			if len(allow) != wantRoutes {
				t.Errorf("allow = %v, want %d rendered route(s)", allow, wantRoutes)
			}
			if unenforced != tc.wantLoose {
				t.Errorf("egress unenforced = %v, want %v", unenforced, tc.wantLoose)
			}
		})
	}
}

func renderedAllowList(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "egress-allow.json")
	rendered := `{"source":"test","destinations":[{"purpose":"model_gateway","fqdn":"gateway.internal","port":443,"protocol":"tcp"}]}`
	if err := os.WriteFile(path, []byte(rendered), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
