package run

// RUN-005 capability matching and RUN-006 outcome classification, as pure
// functions. Both are decision tables, and a decision table is worth testing
// exhaustively because every wrong cell is either a run that should have been
// refused or a failure blamed on the wrong party.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
)

// compatible is a provider that can run the platform's default request. Every
// test below breaks exactly one thing about it.
func compatible() ProviderCapability {
	healthy := true
	c := ProviderCapability{
		Provider: "test_provider",
		Runtimes: []RuntimeSupport{{
			Runtime:          defaultRuntime,
			Versions:         []string{"0.1.0", "0.4.2"},
			AgentIntegration: []string{defaultAgentIntegration},
		}},
		MaxResources: DefaultResourceLimits(),
	}
	c.Isolation.Level = "gvisor"
	c.Isolation.Rootless = true
	c.Network.EgressModes = []string{"default_deny"}
	c.Availability.Healthy = &healthy
	return c
}

func defaultRequirements() Requirements {
	return requirementsFromPolicy(policySnapshot{
		ResourceLimits: DefaultResourceLimits(),
		Egress:         EgressPolicy{Mode: "default_deny"},
	})
}

func TestMatchAcceptsACompatibleProviderAndResolvesTheRuntimeVersion(t *testing.T) {
	profile, err := Match(compatible(), defaultRequirements())
	if err != nil {
		t.Fatalf("a compatible provider was refused: %v", err)
	}
	// The newest declared version, not the first: a run should get the best the
	// provider is prepared to support, and PDM-004 has not pinned one.
	if profile.RuntimeVersion != "0.4.2" {
		t.Errorf("resolved runtime version = %q, want 0.4.2", profile.RuntimeVersion)
	}
	if profile.Runtime != defaultRuntime || profile.AgentIntegration != defaultAgentIntegration {
		t.Errorf("resolved profile = %+v, want the requested runtime and integration mode", profile)
	}
}

// Each row is a provider that must be refused, and the fragment of the reason a
// user would need in order to understand why (ADR-004: refuse with a reason).
func TestMatchRefusesIncompatibleProviders(t *testing.T) {
	unhealthy := false
	for _, tc := range []struct {
		name    string
		break_  func(*ProviderCapability)
		wantSay string
	}{
		{"unhealthy", func(c *ProviderCapability) { c.Availability.Healthy = &unhealthy }, "unhealthy"},
		{"bare process isolation", func(c *ProviderCapability) { c.Isolation.Level = "process" }, "isolate"},
		{"undeclared isolation", func(c *ProviderCapability) { c.Isolation.Level = "" }, "isolate"},
		{"runs as root", func(c *ProviderCapability) { c.Isolation.Rootless = false }, "unprivileged"},
		{"no default-deny egress", func(c *ProviderCapability) {
			c.Network.EgressModes = []string{"none"}
		}, "egress"},
		{"different runtime", func(c *ProviderCapability) {
			c.Runtimes[0].Runtime = "some_other_sdk"
		}, "does not support"},
		{"no versions declared", func(c *ProviderCapability) {
			c.Runtimes[0].Versions = nil
		}, "does not support"},
		{"wrong integration mode", func(c *ProviderCapability) {
			c.Runtimes[0].AgentIntegration = []string{"provider_hosted_agent"}
		}, "mode"},
		{"too little memory", func(c *ProviderCapability) {
			c.MaxResources.MemoryBytes = 1 << 30
		}, "memory"},
		{"too little disk", func(c *ProviderCapability) { c.MaxResources.DiskBytes = 1 << 30 }, "disk"},
		{"too few processes", func(c *ProviderCapability) { c.MaxResources.MaxPIDs = 8 }, "processes"},
		{"wall clock too short", func(c *ProviderCapability) {
			c.MaxResources.WallClockHardSeconds = 60
		}, "wall clock"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := compatible()
			tc.break_(&c)
			_, err := Match(c, defaultRequirements())
			if err == nil {
				t.Fatalf("%s was accepted", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSay) {
				t.Errorf("reason = %q, want it to mention %q", err, tc.wantSay)
			}
			if !strings.Contains(err.Error(), "test_provider") {
				t.Errorf("reason = %q, want it to name the provider", err)
			}
		})
	}
}

// A provider that declares no ceiling for a resource has not declared it, which is
// not the same as declaring zero. Refusing on an omitted field would make every
// partially-filled capability answer unusable.
func TestMatchTreatsAnUndeclaredCeilingAsUnconstrained(t *testing.T) {
	c := compatible()
	c.MaxResources = ResourceLimits{}
	if _, err := Match(c, defaultRequirements()); err != nil {
		t.Fatalf("a provider that declared no ceilings was refused: %v", err)
	}
}

// RUN-006's central distinction: a workload that ran and failed is the skill's
// problem and is never retried, while a provider that could not carry the attempt
// is ours. Everything downstream — the retry policy, the funnel metrics, what the
// user is told — reads these two columns.
func TestClassifyResultSeparatesWorkloadFailureFromProviderFailure(t *testing.T) {
	result := func(status, class string) *RunResult {
		r := &RunResult{Status: status}
		if class != "" {
			r.Error = &RunError{Class: class, Message: "provider said so"}
		}
		return r
	}
	for _, tc := range []struct {
		name             string
		in               ProviderRun
		wantStatus       gen.RunStatus
		wantFailureClass string
	}{
		{
			"workload succeeded",
			ProviderRun{State: ProviderStateCompleted, Result: result("succeeded", "")},
			gen.RunStatusSucceeded, "",
		},
		{
			"workload ran and reported failure",
			ProviderRun{State: ProviderStateCompleted, Result: result("failed", "execution")},
			gen.RunStatusFailed, failureWorkload,
		},
		{
			"provider could not carry the attempt",
			ProviderRun{State: ProviderStateFailed, Result: result("failed", "provision")},
			gen.RunStatusFailed, failureProvider,
		},
		{
			"provider enforced its wall clock",
			ProviderRun{State: ProviderStateFailed, Result: result("timed_out", "timeout")},
			gen.RunStatusTimedOut, failureTimeout,
		},
		{
			"cancelled",
			ProviderRun{State: ProviderStateCancelled, Result: result("cancelled", "cancelled")},
			gen.RunStatusCancelled, failureCancelled,
		},
		{
			// The contract says a terminal state always carries a result. A provider
			// that breaks that promise must not be read as a success.
			"terminal with no result at all",
			ProviderRun{State: ProviderStateCompleted},
			gen.RunStatusFailed, failureProvider,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, failureClass, _, message := classifyResult(tc.in)
			if status != tc.wantStatus {
				t.Errorf("status = %q, want %q", status, tc.wantStatus)
			}
			if failureClass != tc.wantFailureClass {
				t.Errorf("failure class = %q, want %q", failureClass, tc.wantFailureClass)
			}
			if status != gen.RunStatusSucceeded && message == "" {
				t.Error("a failed run was classified with no message to show the user")
			}
			if !IsTerminal(status) {
				t.Errorf("classify produced the non-terminal status %q", status)
			}
		})
	}
}

// The wall clock counts from creation, not from dispatch: PDM-005 §5.2's limit
// covers queue wait too, so a run can time out before a sandbox ever exists.
func TestHardDeadlineComesFromTheRunsOwnFrozenPolicy(t *testing.T) {
	created := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	policy, err := json.Marshal(policySnapshot{
		ResourceLimits: ResourceLimits{WallClockHardSeconds: 120},
	})
	if err != nil {
		t.Fatal(err)
	}
	run := gen.Run{
		CreatedAt:      pgtype.Timestamptz{Time: created, Valid: true},
		PolicySnapshot: policy,
	}
	if got, want := hardDeadline(run), created.Add(2*time.Minute); !got.Equal(want) {
		t.Errorf("deadline = %s, want %s", got, want)
	}

	// A run whose policy says nothing falls back to the platform default rather
	// than to "no deadline", which would leave it running forever.
	run.PolicySnapshot = []byte(`{}`)
	want := created.Add(time.Duration(DefaultResourceLimits().WallClockHardSeconds) * time.Second)
	if got := hardDeadline(run); !got.Equal(want) {
		t.Errorf("deadline without a policy = %s, want the default %s", got, want)
	}
}
