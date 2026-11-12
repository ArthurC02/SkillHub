package sandbox

import (
	"context"
	"errors"
	"log/slog"
	"testing"
)

type searchRuntimeDriver struct {
	*p02Driver
	profile RuntimeProfile
	err     error
	reads   int
}

func (d *searchRuntimeDriver) WebSearchRuntime(context.Context) (RuntimeProfile, error) {
	d.reads++
	return d.profile, d.err
}

func TestSearchCapabilityComesFromTheWorkloadRatherThanConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name    string
		profile RuntimeProfile
		err     error
		want    string
	}{
		{"old workload", RuntimeProfile{}, nil, ""},
		{"actual version", RuntimeProfile{Runtime: "claude_agent_sdk", RuntimeVersion: "2"}, nil, "supported"},
		{"unconfigured version", RuntimeProfile{Runtime: "claude_agent_sdk", RuntimeVersion: "3"}, nil, ""},
		{"another runtime", RuntimeProfile{Runtime: "other", RuntimeVersion: "2"}, nil, ""},
		{"unreadable", RuntimeProfile{}, errors.New("metadata unavailable"), "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			drv := &searchRuntimeDriver{p02Driver: newP02Driver(), profile: tc.profile, err: tc.err}
			cfg := p02Config()
			cfg.Runtimes[0].Versions = []string{"1", "2"}
			cfg.Runtimes[0].WebSearch = "supported"
			m := NewManager(drv, cfg, slog.New(slog.DiscardHandler))
			cap := m.Capability(context.Background())
			if len(cap.Runtimes) != 2 || cap.Runtimes[0].Versions[0] != "2" || cap.Runtimes[1].Versions[0] != "1" || cap.Runtimes[0].WebSearch != tc.want || !cap.Availability.Healthy || drv.reads != 1 {
				t.Fatalf("actual capability = %+v, reads=%d", cap, drv.reads)
			}
			wantOld := ""
			if tc.err != nil {
				wantOld = "unavailable"
			}
			if cap.Runtimes[1].WebSearch != wantOld || cfg.Runtimes[0].WebSearch != "supported" || len(cfg.Runtimes[0].Versions) != 2 {
				t.Fatal("capability leaked across versions or mutated its configuration")
			}
		})
	}
	legacy := p02Manager(newP02Driver())
	legacy.cfg.Runtimes[0].WebSearch = "supported"
	if got := legacy.Capability(context.Background()).Runtimes[0].WebSearch; got != "" {
		t.Fatalf("driver without workload evidence advertised search: %q", got)
	}
}

func TestSearchCreateChecksFreshEvidenceBeforeStartingAWorkload(t *testing.T) {
	for _, tc := range []struct {
		name      string
		profile   RuntimeProfile
		err       error
		search    bool
		wantClass string
		retryable bool
	}{
		{"support disappeared", RuntimeProfile{}, nil, true, ClassCapabilityMismatch, false},
		{"evidence unreadable", RuntimeProfile{}, errors.New("unavailable"), true, ClassProvision, true},
		{"ordinary workload survives unreadable search", RuntimeProfile{}, errors.New("unavailable"), false, "", false},
		{"current workload supports search", RuntimeProfile{Runtime: "claude_agent_sdk", RuntimeVersion: "1"}, nil, true, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			drv := &searchRuntimeDriver{p02Driver: newP02Driver(), profile: RuntimeProfile{Runtime: "claude_agent_sdk", RuntimeVersion: "1"}}
			m := p02Manager(drv)
			m.Capability(context.Background())
			drv.profile, drv.err, drv.reads = tc.profile, tc.err, 0
			req := p02Request()
			if tc.search {
				req.ModelGateway = &ModelGatewayGrant{SearchTool: "skillhub-web-search"}
			}
			run, created, err := m.Create(context.Background(), req)
			if tc.wantClass != "" {
				var refusal *RunError
				if !errors.As(err, &refusal) || refusal.Class != tc.wantClass || refusal.Retryable != tc.retryable || created || len(m.List().Runs) != 0 {
					t.Fatalf("Create = %+v, %v, %v", run, created, err)
				}
				return
			}
			if err != nil || !created || run.State != StateRunning || (!tc.search && drv.reads != 0) {
				t.Fatalf("Create = %+v, %v, %v, reads=%d", run, created, err, drv.reads)
			}
			if err := m.Destroy(context.Background(), run.ProviderRunID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSearchAcceptanceSkipsAnEarlierIncompatibleIntegration(t *testing.T) {
	cfg := integratedConfig()
	other := cfg.Runtimes[0]
	other.AgentIntegration = []string{"provider_hosted_agent"}
	current := cfg.Runtimes[0]
	current.WebSearch = "supported"
	cfg.Runtimes = []RuntimeCapability{other, current}
	req := boundedRequest()
	req.Runtime.AgentIntegration = "in_sandbox_sdk"
	req.ModelGateway = &ModelGatewayGrant{SearchTool: "skillhub-web-search"}
	if refusal := cfg.accept(req); refusal != nil {
		t.Fatalf("later compatible runtime was refused: %v", refusal)
	}
	current.WebSearch = "unavailable"
	cfg.Runtimes = []RuntimeCapability{current, cfg.Runtimes[1]}
	if refusal := cfg.accept(req); refusal != nil {
		t.Fatalf("unavailable entry hid a later ready entry: %v", refusal)
	}
}

func TestSearchCreateReplayDoesNotDependOnFreshCapabilityEvidence(t *testing.T) {
	drv := &searchRuntimeDriver{p02Driver: newP02Driver(), profile: RuntimeProfile{Runtime: "claude_agent_sdk", RuntimeVersion: "1"}}
	m := p02Manager(drv)
	req := p02Request()
	req.ModelGateway = &ModelGatewayGrant{SearchTool: "skillhub-web-search"}
	first, created, err := m.Create(context.Background(), req)
	if err != nil || !created {
		t.Fatalf("initial Create = %+v, %v, %v", first, created, err)
	}
	defer func() { _ = m.Destroy(context.Background(), first.ProviderRunID) }()
	drv.err = errors.New("image temporarily unreadable")
	again, created, err := m.Create(context.Background(), req)
	if err != nil || created || again.ProviderRunID != first.ProviderRunID {
		t.Fatalf("replay = %+v, %v, %v; want the original attempt", again, created, err)
	}
	req.TestCase.UserPrompt = "changed request"
	if _, _, err := m.Create(context.Background(), req); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed request replay = %v, want conflict", err)
	}
}
