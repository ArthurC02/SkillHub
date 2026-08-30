package run

// RUN-005 capability matching and RUN-006 outcome classification, as pure
// functions. Both are decision tables, and a decision table is worth testing
// exhaustively because every wrong cell is either a run that should have been
// refused or a failure blamed on the wrong party.

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

// TestMatchIsAnAllowListSoAnUnknownIsolationLevelIsRefused pins the shape, not
// the values. The previous deny list refused "", `process` and `container` and
// passed everything else, so `gvsior` — one transposition away from the
// production baseline — was dispatched to as if it were gvisor. No code path
// produced such a value, but the same shape already caused one incident here,
// and the fix that time was to extend the deny list rather than invert it.
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
	// The baseline itself still runs, or the allow list has eaten production.
	c := compatible()
	c.Isolation.Level = "gvisor"
	if _, err := Match(c, defaultRequirements()); err != nil {
		t.Errorf("the production isolation baseline was refused: %v", err)
	}
}

// TestMatchAcceptsCleanOnlyUnderItsOwnOptIn covers the level that has no
// boundary at all. Its gate must be a variable of its own: reusing DEV_LOGIN
// would give it to every machine that already exports it.
func TestMatchAcceptsCleanOnlyUnderItsOwnOptIn(t *testing.T) {
	c := compatible()
	c.Isolation.Level = "clean"

	t.Setenv("DEV_LOGIN", "")
	t.Setenv("SKILLHUB_CLEAN_MODE", "")
	if _, err := Match(c, defaultRequirements()); err == nil {
		t.Fatal("a provider with no isolation was accepted by a deployment that never opted in")
	}

	// A development machine is not a clean-test machine. This is the assertion
	// that fails if somebody later "simplifies" the two gates into one.
	t.Setenv("DEV_LOGIN", "1")
	if _, err := Match(c, defaultRequirements()); err == nil {
		t.Error("DEV_LOGIN alone accepted a provider that does not isolate at all")
	}

	t.Setenv("DEV_LOGIN", "")
	t.Setenv("SKILLHUB_CLEAN_MODE", "1")
	if _, err := Match(c, defaultRequirements()); err != nil {
		t.Errorf("the clean test mode could not dispatch to its own driver: %v", err)
	}

	// And the clean opt-in reaches `clean` and nothing further.
	for _, level := range []string{"process", ""} {
		bare := compatible()
		bare.Isolation.Level = level
		if _, err := Match(bare, defaultRequirements()); err == nil {
			t.Errorf("isolation %q was accepted by a clean-test deployment", level)
		}
	}
}

// A ceiling the node declares a number for but does not enforce is unbounded,
// whatever `max_resources` says. 02:PORT-010 asks the declaration to reflect what
// was actually detected — and the reason it asks (ADR-059 decision 3's incident:
// a node that ran every untrusted skill on the shared kernel, and said so only in
// a startup log) is only answered if something refuses on it. Until this, the two
// honesty fields were decoded nowhere on the platform side and the signal was
// still log-only.
func TestMatchRefusesAProviderThatDoesNotEnforceWhatItDeclares(t *testing.T) {
	t.Setenv("DEV_LOGIN", "")
	t.Setenv("SKILLHUB_CLEAN_MODE", "")

	c := compatible()
	c.MaxResourcesUnenforced = []string{"vcpu", "disk_bytes"}
	_, err := Match(c, defaultRequirements())
	if err == nil {
		t.Fatal("a provider naming ceilings it does not enforce was dispatched to")
	}
	// The reason names which ones: an operator reading it has to be able to tell
	// "the CPU limit is decorative" from "this node is down".
	for _, want := range []string{"test_provider", "vcpu", "disk_bytes"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("reason = %q, want it to mention %q", err, want)
		}
	}

	// The clean test mode is the one deployment where this is not news: it has
	// already opted into having no boundary at all, so refusing it for an
	// unenforced CPU ceiling would take out the only mode that can run there.
	t.Setenv("SKILLHUB_CLEAN_MODE", "1")
	clean := compatible()
	clean.Isolation.Level = "clean"
	clean.MaxResourcesUnenforced = []string{"vcpu"}
	if _, err := Match(clean, defaultRequirements()); err != nil {
		t.Errorf("the clean test mode could not dispatch to its own driver: %v", err)
	}
}

// The same gate for the other half of a node's declaration, and it needs its own
// test because it needs its own branch: a node can hold every resource ceiling
// and still filter no traffic. Until 2026-08-30 there was no field for it at
// all, so a clean node declared `none` and RUN-005 refused every run carrying a
// model gateway grant - the demo's Trace screen had nothing to show and the
// reason was recorded as "needs money" (04 丙-96, 丙-98, 05 R-32).
func TestMatchRefusesAProviderThatDeclaresEgressItDoesNotEnforce(t *testing.T) {
	t.Setenv("DEV_LOGIN", "")
	t.Setenv("SKILLHUB_CLEAN_MODE", "")

	c := compatible()
	c.Network.EgressUnenforced = true
	_, err := Match(c, defaultRequirements())
	if err == nil {
		t.Fatal("a provider that filters nothing was dispatched to in a deployment that requires a boundary")
	}
	// It has to name the node and the reason. This refusal is easy to mistake for
	// the allow-list mismatch right below it in Match, and an operator who reads
	// it that way goes off to edit a rendered file that would not have helped.
	for _, want := range []string{"test_provider", "egress"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("reason = %q, want it to mention %q", err, want)
		}
	}

	// And the one deployment that has already accepted having no boundary can
	// dispatch to its own driver - with a destination, which is the whole point:
	// no model gateway grant means no model call, and no model call means no
	// trace worth showing.
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

// reaps_detached_descendants is disclosed, not refused. The two platforms of one
// driver legitimately differ on it (a Windows job object holds every descendant;
// a POSIX process group does not hold one that called setsid), so a gate would
// refuse a node for its operating system. 02:PORT-003 asks instead that "the
// sandbox does not hold this" reaches somewhere the user can read, which is the
// pre-run permission summary.
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

	// Three states, three summaries: said no (disclose), said yes (nothing to
	// say), did not say (also nothing to say — an absent field is not a claim,
	// and rendering it as "descendants survive" would be inventing one).
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

// ── 02:PORT-010 / 04 丙-85: the content-source gate ─────────────────────────
//
// The clean test mode's driver has no isolation boundary at all, so the only
// thing left between it and somebody else's code is where the material came
// from. Until this gate the acceptance criterion 「不得承載不受信任的內容」 had
// no enforcement point anywhere: three documents named each other and the one
// they all pointed at (Match) is a function of a provider capability, which
// cannot see a skill.

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

// The normal path must not acquire a new way to fail. Nil reader, uncurated
// content, a read that explodes — none of it may matter when the deployment has
// a real sandbox, and this is the assertion that fails if the env check is ever
// "simplified" out of requireCuratedContent.
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
	// Not even without a reader wired: an unconfigured field is a refusal only
	// where the refusal protects something.
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
			// The one refusal that would otherwise read as a bug, so it gets its
			// own sentence: the tier is right and the bytes are not the reviewed
			// ones (skills.curated_version_id, 0042).
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

// The gate must ask about the run in front of it. Passing the wrong workspace
// would check somebody else's material; passing the wrong version would check
// the wrong bytes of the right skill.
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
