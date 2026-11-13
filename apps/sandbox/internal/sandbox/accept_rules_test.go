package sandbox

import "testing"

func integratedConfig() Config {
	return Config{
		Runtimes: []RuntimeCapability{{
			Runtime: "claude_agent_sdk", Versions: []string{"1"}, AgentIntegration: []string{"in_sandbox_sdk"},
		}},
		MaxResources: DefaultLimits,
		EgressModes:  []string{"none"},
	}
}

func boundedRequest() RunRequest {
	limits := DefaultLimits
	budget := *DefaultLimits.TokenBudget
	limits.TokenBudget = &budget
	return RunRequest{
		Runtime:        RuntimeProfile{Runtime: "claude_agent_sdk", RuntimeVersion: "1"},
		ResourceLimits: limits,
		Egress:         EgressPolicy{Mode: "none"},
	}
}

type acceptCase struct {
	name        string
	change      func(*RunRequest)
	wantMessage string
}

func runAcceptCases(t *testing.T, cfg Config, cases []acceptCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := boundedRequest()
			tc.change(&req)
			assertAcceptVerdict(t, cfg.accept(req), tc.wantMessage)
		})
	}
}

func assertAcceptVerdict(t *testing.T, got *RunError, wantMessage string) {
	t.Helper()
	if wantMessage == "" {
		if got != nil {
			t.Fatalf("accept refused a run it can enforce: %v", got)
		}
		return
	}
	if got == nil {
		t.Fatalf("accept admitted the run, want refusal %q", wantMessage)
	}
	if got.Class != ClassCapabilityMismatch || got.Retryable {
		t.Fatalf("refusal class = %q retryable = %v, want %q and not retryable", got.Class, got.Retryable, ClassCapabilityMismatch)
	}
	if got.Message != wantMessage {
		t.Fatalf("refusal message = %q, want %q", got.Message, wantMessage)
	}
}

func TestAcceptTellsAMissingVersionApartFromAMissingRuntime(t *testing.T) {
	runAcceptCases(t, integratedConfig(), []acceptCase{
		{"carried version", func(*RunRequest) {}, ""},
		{"version this node does not carry", func(r *RunRequest) { r.Runtime.RuntimeVersion = "2" },
			"runtime claude_agent_sdk version 2 is not available here"},
		{"runtime this node does not carry", func(r *RunRequest) { r.Runtime.Runtime = "other_harness" },
			"runtime other_harness is not available here"},
	})
}

func TestAcceptRefusesAnAgentIntegrationTheRuntimeDoesNotOffer(t *testing.T) {
	runAcceptCases(t, integratedConfig(), []acceptCase{
		{"no integration requested", func(*RunRequest) {}, ""},
		{"offered integration", func(r *RunRequest) { r.Runtime.AgentIntegration = "in_sandbox_sdk" }, ""},
		{"integration the runtime does not offer", func(r *RunRequest) { r.Runtime.AgentIntegration = "host_sdk" },
			"agent integration host_sdk is not supported here"},
	})
}

func TestAcceptRefusesARequestThatLeavesAResourceCeilingUnset(t *testing.T) {
	const unbounded = "resource_limits must set every ceiling: this provider will not run unbounded"
	runAcceptCases(t, integratedConfig(), []acceptCase{
		{"every ceiling at its smallest positive value", func(r *RunRequest) {
			r.ResourceLimits = ResourceLimits{
				VCPU: 1, MemoryBytes: 1, DiskBytes: 1, MaxPIDs: 1, MaxOpenFiles: 1,
				WallClockSoftSeconds: 1, WallClockHardSeconds: 2, ArtifactTotalBytes: 1, ArtifactFileBytes: 1,
			}
		}, ""},
		{"vcpu zero", func(r *RunRequest) { r.ResourceLimits.VCPU = 0 }, unbounded},
		{"memory zero", func(r *RunRequest) { r.ResourceLimits.MemoryBytes = 0 }, unbounded},
		{"disk zero", func(r *RunRequest) { r.ResourceLimits.DiskBytes = 0 }, unbounded},
		{"processes zero", func(r *RunRequest) { r.ResourceLimits.MaxPIDs = 0 }, unbounded},
		{"open files zero", func(r *RunRequest) { r.ResourceLimits.MaxOpenFiles = 0 }, unbounded},
		{"soft wall clock zero", func(r *RunRequest) { r.ResourceLimits.WallClockSoftSeconds = 0 }, unbounded},
		{"hard wall clock zero", func(r *RunRequest) { r.ResourceLimits.WallClockHardSeconds = 0 }, unbounded},
		{"artifact total zero", func(r *RunRequest) { r.ResourceLimits.ArtifactTotalBytes = 0 }, unbounded},
		{"artifact file zero", func(r *RunRequest) { r.ResourceLimits.ArtifactFileBytes = 0 }, unbounded},
	})
}

func TestAcceptRefusesATokenBudgetThatLeavesACeilingUnset(t *testing.T) {
	const unset = "token_budget must set both ceilings"
	runAcceptCases(t, integratedConfig(), []acceptCase{
		{"both ceilings at one", func(r *RunRequest) {
			r.ResourceLimits.TokenBudget = &TokenBudget{MaxInputTokens: 1, MaxOutputTokens: 1}
		}, ""},
		{"no input ceiling", func(r *RunRequest) { r.ResourceLimits.TokenBudget.MaxInputTokens = 0 }, unset},
		{"no output ceiling", func(r *RunRequest) { r.ResourceLimits.TokenBudget.MaxOutputTokens = 0 }, unset},
	})
}

func TestAcceptRefusesAnEgressModeItDoesNotKnow(t *testing.T) {
	cfg := integratedConfig()
	cfg.EgressModes = []string{"default_deny", "none"}
	runAcceptCases(t, cfg, []acceptCase{
		{"none", func(*RunRequest) {}, ""},
		{"default deny with nothing allowed", func(r *RunRequest) { r.Egress.Mode = "default_deny" }, ""},
		{"unknown mode", func(r *RunRequest) { r.Egress.Mode = "allow_all" }, `egress mode "allow_all" is not supported`},
	})
}

func TestAcceptSaysANodeWithoutAnEgressRouteCannotAllowAnything(t *testing.T) {
	runAcceptCases(t, integratedConfig(), []acceptCase{
		{"one destination on a node with no route", func(r *RunRequest) {
			r.Egress = EgressPolicy{Mode: "default_deny", Allow: []EgressAllowEntry{{Purpose: "model_gateway", URL: "http://gw:4000"}}}
		}, "this provider has no egress route, so it cannot allow 1 destination(s)"},
	})
}

func TestAcceptTellsAMalformedDestinationApartFromAnUnroutedOne(t *testing.T) {
	cfg := routedConfig()
	for _, tc := range []struct {
		name, url, wantMessage string
	}{
		{"scheme that implies no port", "gopher://litellm.internal",
			`egress destination "gopher://litellm.internal" for model_gateway is not a URL naming a host and port, ` +
				"so no accept rule could match it"},
		{"well-formed destination with no rule", "http://litellm.internal:5432",
			"this node renders no egress rule for model_gateway at litellm.internal:5432; it routes to model_gateway:4000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := cfg.accept(routedRequest(EgressAllowEntry{Purpose: "model_gateway", URL: tc.url}))
			assertAcceptVerdict(t, got, tc.wantMessage)
		})
	}
}

func TestSearchIsAcceptedOnlyByTheRequestedRuntimeVersion(t *testing.T) {
	for _, tc := range []struct {
		name, version, tool, want string
	}{
		{"ordinary old runtime", "1", "", ""},
		{"search on old runtime", "1", "skillhub-web-search", "runtime claude_agent_sdk version 1 does not support web search"},
		{"search on supporting runtime", "2", "skillhub-web-search", ""},
		{"unregistered search tool", "2", "other-search", "search tool other-search is not supported here"},
		{"unavailable version", "3", "skillhub-web-search", "runtime claude_agent_sdk version 3 is not available here"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := integratedConfig()
			current := cfg.Runtimes[0]
			current.Versions, current.WebSearch = []string{"2"}, "supported"
			cfg.Runtimes = append(cfg.Runtimes, current)
			req := boundedRequest()
			req.Runtime.RuntimeVersion = tc.version
			if tc.tool != "" {
				req.ModelGateway = &ModelGatewayGrant{SearchTool: tc.tool}
			}
			assertAcceptVerdict(t, cfg.accept(req), tc.want)
		})
	}
}

func TestSearchRuntimeEvidenceCanBeUnavailableWithoutBlockingOrdinaryRuns(t *testing.T) {
	for _, tc := range []struct {
		state     string
		class     string
		retryable bool
	}{
		{"", ClassCapabilityMismatch, false},
		{"unsupported", ClassCapabilityMismatch, false},
		{"unavailable", ClassProvision, true},
		{"unexpected", ClassProvision, true},
		{"supported", "", false},
	} {
		t.Run(tc.state, func(t *testing.T) {
			cfg := integratedConfig()
			cfg.Runtimes[0].WebSearch = tc.state
			req := boundedRequest()
			req.ModelGateway = &ModelGatewayGrant{SearchTool: "skillhub-web-search"}
			got := cfg.accept(req)
			if tc.class == "" {
				if got != nil {
					t.Fatalf("supported search was refused: %+v", got)
				}
			} else if got == nil || got.Class != tc.class || got.Retryable != tc.retryable {
				t.Fatalf("search state %q verdict = %+v, want %s retryable=%v", tc.state, got, tc.class, tc.retryable)
			}
			req.ModelGateway = nil
			if got := cfg.accept(req); got != nil {
				t.Fatalf("ordinary request depended on search evidence: %+v", got)
			}
		})
	}
}
