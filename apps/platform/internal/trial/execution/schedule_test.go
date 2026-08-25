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

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
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
		{"no egress mode the request can use", func(c *ProviderCapability) {
			c.Network.EgressModes = []string{"something_else"}
		}, "egress"},
		// A provider that declares no egress modes at all has not answered the
		// question. The table tested only a non-empty wrong answer, so the
		// no-answer case fell through a `len(offered) == 0: return true` that
		// contradicted the function's own comment — egress was the one
		// capability failing open while an undeclared resource ceiling is a hard
		// refusal (M2 audit, 2026-08-24; ADR-022 做不到的一律 fail-closed).
		{"declares no egress modes at all", func(c *ProviderCapability) {
			c.Network.EgressModes = nil
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

// ADR-015 makes gVisor the production baseline, and `container` is what a
// provider declares when it is running plain runc — every untrusted skill on the
// host kernel. The sandbox declares that honestly (SKILLHUB_SANDBOX_RUNTIME unset
// or misspelled), so the only place it can be caught is here, and `container` used
// to pass simply by being neither "" nor `process`.
//
// It is accepted where the deployment has declared itself an offline development
// one (DEV_LOGIN, ADR-020) and refused everywhere else, so a production node whose
// unit file lost the runtime variable is refused instead of quietly served.
func TestMatchRefusesHostKernelIsolationUnlessTheDeploymentIsADevelopmentOne(t *testing.T) {
	c := compatible()
	c.Isolation.Level = "container"

	// Explicit rather than inherited: a developer whose shell already exports
	// DEV_LOGIN would otherwise never see this half fail.
	t.Setenv("DEV_LOGIN", "")
	_, err := Match(c, defaultRequirements())
	if err == nil {
		t.Fatal("a host-kernel provider was accepted by a deployment that never opted in")
	}
	if !strings.Contains(err.Error(), "test_provider") {
		t.Errorf("reason = %q, want it to name the provider", err)
	}
	// The refusal has to read as "this deployment is misconfigured", not as "your
	// run asked for too much" — nothing about the request changed.
	if !strings.Contains(err.Error(), "deployment") {
		t.Errorf("reason = %q, want it to say the deployment is what refuses this provider", err)
	}

	t.Setenv("DEV_LOGIN", "1")
	if _, err := Match(c, defaultRequirements()); err != nil {
		t.Errorf("a development deployment could not run its own sandbox: %v", err)
	}

	// The opt-in reaches `container` and nothing further: iron rule 1 is not for
	// sale on a developer machine either.
	for _, level := range []string{"process", ""} {
		bare := compatible()
		bare.Isolation.Level = level
		if _, err := Match(bare, defaultRequirements()); err == nil {
			t.Errorf("isolation %q was accepted by a development deployment", level)
		}
	}
}

// The two egress modes are ordered, not alternatives. A provider with no route
// out at all is strictly stronger than one that denies by default and permits a
// list — so it can carry a run allowed to reach nothing, and only that run. This
// is the dev DockerProvider's declaration (`--network none`), so getting the
// direction wrong would refuse every run on a developer machine, and getting it
// backwards would run a network-needing skill somewhere it cannot reach anything.
func TestMatchAcceptsAStrongerEgressModeButNeverAWeakerOne(t *testing.T) {
	noEgress := compatible()
	noEgress.Network.EgressModes = []string{"none"}

	req := defaultRequirements()
	if req.EgressAllowed != 0 {
		t.Fatalf("the default policy allows %d destinations, want none", req.EgressAllowed)
	}
	if _, err := Match(noEgress, req); err != nil {
		t.Errorf("a provider with no egress at all was refused a run allowed to reach nothing: %v", err)
	}

	// The moment the run needs to reach something, that provider cannot serve it.
	needsEgress := req
	needsEgress.EgressAllowed = 1
	_, err := Match(noEgress, needsEgress)
	if err == nil {
		t.Fatal("a run with an allow list was sent to a provider with no route out")
	}
	if !strings.Contains(err.Error(), "allow list") {
		t.Errorf("reason = %q, want it to name the allow list as the thing that did not fit", err)
	}

	// And the substitution never runs the other way: default_deny is not a
	// stand-in for a request that asked for none.
	proxied := compatible()
	proxied.Network.EgressModes = []string{"default_deny"}
	strict := req
	strict.EgressMode = "none"
	if _, err := Match(proxied, strict); err == nil {
		t.Error("a default_deny provider was accepted for a run that asked for no egress at all")
	}
}

func TestMatchRefusesAnUndeclaredCeiling(t *testing.T) {
	c := compatible()
	c.MaxResources = ResourceLimits{}
	if _, err := Match(c, defaultRequirements()); err == nil {
		t.Fatal("a provider that declared no ceilings accepted a bounded run")
	}
}

func TestMatchChecksEveryResourceCeiling(t *testing.T) {
	checks := map[string]func(*ResourceLimits){
		"vcpu":                func(l *ResourceLimits) { l.VCPU = 1 },
		"memory":              func(l *ResourceLimits) { l.MemoryBytes = 1 },
		"disk":                func(l *ResourceLimits) { l.DiskBytes = 1 },
		"processes":           func(l *ResourceLimits) { l.MaxPIDs = 1 },
		"open files":          func(l *ResourceLimits) { l.MaxOpenFiles = 1 },
		"soft wall clock":     func(l *ResourceLimits) { l.WallClockSoftSeconds = 1 },
		"hard wall clock":     func(l *ResourceLimits) { l.WallClockHardSeconds = 1 },
		"artifact total":      func(l *ResourceLimits) { l.ArtifactTotalBytes = 1 },
		"artifact file":       func(l *ResourceLimits) { l.ArtifactFileBytes = 1 },
		"input token budget":  func(l *ResourceLimits) { l.TokenBudget.MaxInputTokens = 1 },
		"output token budget": func(l *ResourceLimits) { l.TokenBudget.MaxOutputTokens = 1 },
	}
	for name, lower := range checks {
		t.Run(name, func(t *testing.T) {
			c := compatible()
			lower(&c.MaxResources)
			if _, err := Match(c, defaultRequirements()); err == nil {
				t.Fatal("provider accepted a run above its declared ceiling")
			}
		})
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
