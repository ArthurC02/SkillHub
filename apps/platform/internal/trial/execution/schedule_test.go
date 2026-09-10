package run

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

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

	if profile.RuntimeVersion != "0.4.2" {
		t.Errorf("resolved runtime version = %q, want 0.4.2", profile.RuntimeVersion)
	}
	if profile.Runtime != defaultRuntime || profile.AgentIntegration != defaultAgentIntegration {
		t.Errorf("resolved profile = %+v, want the requested runtime and integration mode", profile)
	}
}

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

func TestMatchRefusesHostKernelIsolationUnlessTheDeploymentIsADevelopmentOne(t *testing.T) {
	c := compatible()
	c.Isolation.Level = "container"

	t.Setenv("DEV_LOGIN", "")
	_, err := Match(c, defaultRequirements())
	if err == nil {
		t.Fatal("a host-kernel provider was accepted by a deployment that never opted in")
	}
	if !strings.Contains(err.Error(), "test_provider") {
		t.Errorf("reason = %q, want it to name the provider", err)
	}

	if !strings.Contains(err.Error(), "deployment") {
		t.Errorf("reason = %q, want it to say the deployment is what refuses this provider", err)
	}

	t.Setenv("DEV_LOGIN", "1")
	if _, err := Match(c, defaultRequirements()); err != nil {
		t.Errorf("a development deployment could not run its own sandbox: %v", err)
	}

	for _, level := range []string{"process", ""} {
		bare := compatible()
		bare.Isolation.Level = level
		if _, err := Match(bare, defaultRequirements()); err == nil {
			t.Errorf("isolation %q was accepted by a development deployment", level)
		}
	}
}

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

	needsEgress := req
	needsEgress.EgressAllowed = 1
	_, err := Match(noEgress, needsEgress)
	if err == nil {
		t.Fatal("a run with an allow list was sent to a provider with no route out")
	}
	if !strings.Contains(err.Error(), "allow list") {
		t.Errorf("reason = %q, want it to name the allow list as the thing that did not fit", err)
	}

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

	run.PolicySnapshot = []byte(`{}`)
	want := created.Add(time.Duration(DefaultResourceLimits().WallClockHardSeconds) * time.Second)
	if got := hardDeadline(run); !got.Equal(want) {
		t.Errorf("deadline without a policy = %s, want the default %s", got, want)
	}
}

func TestMatchIsAnAllowListSoAnUnknownIsolationLevelIsRefused(t *testing.T) {
	t.Setenv("DEV_LOGIN", "")
	t.Setenv("SKILLHUB_CLEAN_MODE", "")
	for _, level := range []string{"gvsior", "banana", "hosted-vm", "GVISOR", "gvisor "} {
		c := compatible()
		c.Isolation.Level = level
		if _, err := Match(c, defaultRequirements()); err == nil {
			t.Errorf("isolation %q was accepted; only levels written down here may run anything", level)
		}
	}

	c := compatible()
	c.Isolation.Level = "gvisor"
	if _, err := Match(c, defaultRequirements()); err != nil {
		t.Errorf("the production isolation baseline was refused: %v", err)
	}
}

func TestMatchAcceptsCleanOnlyUnderItsOwnOptIn(t *testing.T) {
	c := compatible()
	c.Isolation.Level = "clean"

	t.Setenv("DEV_LOGIN", "")
	t.Setenv("SKILLHUB_CLEAN_MODE", "")
	if _, err := Match(c, defaultRequirements()); err == nil {
		t.Fatal("a provider with no isolation was accepted by a deployment that never opted in")
	}

	t.Setenv("DEV_LOGIN", "1")
	if _, err := Match(c, defaultRequirements()); err == nil {
		t.Error("DEV_LOGIN alone accepted a provider that does not isolate at all")
	}

	t.Setenv("DEV_LOGIN", "")
	t.Setenv("SKILLHUB_CLEAN_MODE", "1")
	if _, err := Match(c, defaultRequirements()); err != nil {
		t.Errorf("the clean test mode could not dispatch to its own driver: %v", err)
	}

	for _, level := range []string{"process", ""} {
		bare := compatible()
		bare.Isolation.Level = level
		if _, err := Match(bare, defaultRequirements()); err == nil {
			t.Errorf("isolation %q was accepted by a clean-test deployment", level)
		}
	}
}

func TestMatchRefusesAProviderThatDoesNotEnforceWhatItDeclares(t *testing.T) {
	t.Setenv("DEV_LOGIN", "")
	t.Setenv("SKILLHUB_CLEAN_MODE", "")

	c := compatible()
	c.MaxResourcesUnenforced = []string{"vcpu", "disk_bytes"}
	_, err := Match(c, defaultRequirements())
	if err == nil {
		t.Fatal("a provider naming ceilings it does not enforce was dispatched to")
	}

	for _, want := range []string{"test_provider", "vcpu", "disk_bytes"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("reason = %q, want it to mention %q", err, want)
		}
	}

	t.Setenv("SKILLHUB_CLEAN_MODE", "1")
	clean := compatible()
	clean.Isolation.Level = "clean"
	clean.MaxResourcesUnenforced = []string{"vcpu"}
	if _, err := Match(clean, defaultRequirements()); err != nil {
		t.Errorf("the clean test mode could not dispatch to its own driver: %v", err)
	}
}

func TestMatchRefusesAProviderThatDeclaresEgressItDoesNotEnforce(t *testing.T) {
	t.Setenv("DEV_LOGIN", "")
	t.Setenv("SKILLHUB_CLEAN_MODE", "")

	c := compatible()
	c.Network.EgressUnenforced = true
	_, err := Match(c, defaultRequirements())
	if err == nil {
		t.Fatal("a provider that filters nothing was dispatched to in a deployment that requires a boundary")
	}

	for _, want := range []string{"test_provider", "egress"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("reason = %q, want it to mention %q", err, want)
		}
	}

	t.Setenv("SKILLHUB_CLEAN_MODE", "1")
	clean := compatible()
	clean.Isolation.Level = "clean"
	clean.Network.EgressUnenforced = true
	req := defaultRequirements()
	req.EgressMode, req.EgressAllowed = "default_deny", 1
	if _, err := Match(clean, req); err != nil {
		t.Errorf("the clean test mode could not take a run that names a destination: %v", err)
	}
}

func TestAProviderThatCannotReapDetachedDescendantsRunsButSaysSo(t *testing.T) {
	t.Setenv("DEV_LOGIN", "")
	t.Setenv("SKILLHUB_CLEAN_MODE", "1")

	no, yes := false, true
	c := compatible()
	c.Isolation.Level = "clean"
	c.Isolation.ReapsDetachedDescendants = &no
	if _, err := Match(c, defaultRequirements()); err != nil {
		t.Fatalf("a clean provider was refused for a disclosure-shaped fact: %v", err)
	}

	for _, tc := range []struct {
		what  string
		reaps *bool
		want  bool
	}{
		{"declared false", &no, true},
		{"declared true", &yes, false},
		{"not declared at all", nil, false},
	} {
		c.Isolation.ReapsDetachedDescendants = tc.reaps
		if got := detachedDescendantsSurvive(c); got != tc.want {
			t.Errorf("%s: summary would say descendants survive = %v, want %v", tc.what, got, tc.want)
		}
	}
}

func contentSourceRun() gen.Run {
	return gen.Run{
		WorkspaceID:    mustTestUUID("11111111-1111-1111-1111-111111111111"),
		SkillVersionID: mustTestUUID("22222222-2222-2222-2222-222222222222"),
	}
}

func mustTestUUID(s string) pgtype.UUID {
	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		panic(err)
	}
	return id
}

func curatedSource() ContentSource {
	return ContentSource{CurationTier: curatedTier, CuratedVersionIsThisOne: true}
}

func TestTheContentSourceGateDoesNothingOutsideTheCleanTestMode(t *testing.T) {
	t.Setenv("DEV_LOGIN", "1")
	t.Setenv("SKILLHUB_CLEAN_MODE", "")

	called := false
	svc := &Service{ReadContentSource: func(context.Context, pgtype.UUID, pgtype.UUID) (ContentSource, bool, error) {
		called = true
		return ContentSource{CurationTier: "indexed"}, true, nil
	}}
	if err := svc.requireCuratedContent(t.Context(), contentSourceRun()); err != nil {
		t.Fatalf("a production deployment was refused by the clean-mode content gate: %v", err)
	}
	if called {
		t.Error("the content-source read ran outside the clean test mode; it must cost the normal path nothing")
	}

	if err := (&Service{}).requireCuratedContent(t.Context(), contentSourceRun()); err != nil {
		t.Errorf("an unwired reader refused a run on a deployment that has a sandbox: %v", err)
	}
}

func TestTheCleanTestModeOnlyRunsCuratedMaterial(t *testing.T) {
	for _, tc := range []struct {
		what     string
		read     func(context.Context, pgtype.UUID, pgtype.UUID) (ContentSource, bool, error)
		wantPass bool
		wantSaid []string
	}{
		{
			what:     "a skill in the public catalogue",
			read:     stubContentSource(ContentSource{WorkspaceIsCatalog: true, CurationTier: "indexed"}, true, nil),
			wantPass: true,
		},
		{
			what:     "a curated verdict on the exact version being run",
			read:     stubContentSource(curatedSource(), true, nil),
			wantPass: true,
		},
		{
			what: "a curated verdict on some other version",
			read: stubContentSource(ContentSource{CurationTier: curatedTier}, true, nil),

			wantSaid: []string{"different version"},
		},
		{
			what:     "an ordinary imported skill",
			read:     stubContentSource(ContentSource{CurationTier: "indexed"}, true, nil),
			wantSaid: []string{"indexed", "catalogue", curatedTier, "sandbox"},
		},
		{
			what:     "a version whose skill is gone",
			read:     stubContentSource(ContentSource{}, false, nil),
			wantSaid: []string{"could not be found"},
		},
		{
			what:     "a read that failed",
			read:     stubContentSource(ContentSource{}, false, errors.New("connection refused")),
			wantSaid: []string{"could not be read", "connection refused"},
		},
		{
			what:     "no reader wired at all",
			read:     nil,
			wantSaid: []string{"not configured"},
		},
	} {
		t.Run(tc.what, func(t *testing.T) {
			t.Setenv("SKILLHUB_CLEAN_MODE", "1")
			err := (&Service{ReadContentSource: tc.read}).requireCuratedContent(t.Context(), contentSourceRun())
			if tc.wantPass {
				if err != nil {
					t.Fatalf("curated material was refused: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("uncurated material was handed to a driver with no isolation boundary")
			}
			if !errors.Is(err, ErrContentNotCurated) {
				t.Errorf("error = %v, want it to wrap ErrContentNotCurated so the caller can classify it", err)
			}
			for _, want := range tc.wantSaid {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("reason = %q, want it to mention %q", err, want)
				}
			}
		})
	}
}

func TestTheContentSourceGateAsksAboutThisRunsOwnVersion(t *testing.T) {
	t.Setenv("SKILLHUB_CLEAN_MODE", "1")
	run := contentSourceRun()

	var gotWorkspace, gotVersion pgtype.UUID
	svc := &Service{ReadContentSource: func(_ context.Context, workspaceID, versionID pgtype.UUID) (ContentSource, bool, error) {
		gotWorkspace, gotVersion = workspaceID, versionID
		return curatedSource(), true, nil
	}}
	if err := svc.requireCuratedContent(t.Context(), run); err != nil {
		t.Fatalf("curated material was refused: %v", err)
	}
	if gotWorkspace != run.WorkspaceID {
		t.Errorf("asked about workspace %v, want the run's own %v", gotWorkspace, run.WorkspaceID)
	}
	if gotVersion != run.SkillVersionID {
		t.Errorf("asked about version %v, want the run's own %v", gotVersion, run.SkillVersionID)
	}
}

func stubContentSource(source ContentSource, found bool, err error) func(context.Context, pgtype.UUID, pgtype.UUID) (ContentSource, bool, error) {
	return func(context.Context, pgtype.UUID, pgtype.UUID) (ContentSource, bool, error) {
		return source, found, err
	}
}
