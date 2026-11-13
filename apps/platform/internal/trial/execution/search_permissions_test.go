package run

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSearchRequiresAnExactCompatibilityDeclarationLine(t *testing.T) {
	for _, tc := range []struct {
		name, compatibility string
		want                bool
	}{
		{"empty", "", false},
		{"unrelated", "Linux", false},
		{"exact declaration", "skillhub-web-search", true},
		{"separate line", "Linux\r\n  skillhub-web-search \r\nPython", true},
		{"prose mention", "Does not require skillhub-web-search", false},
		{"prefix", "skillhub-web-search-extra", false},
		{"case differs", "Skillhub-web-search", false},
		{"quoted mention", "`skillhub-web-search`", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			version := VersionFacts{Compatibility: tc.compatibility}
			policy := versionPolicy(Deployment{}, version)
			if policy.WebSearch != tc.want {
				t.Fatalf("search requirement = %v, want %v", policy.WebSearch, tc.want)
			}
		})
	}
}

func TestSearchAuthorizationRequiresTheFrozenPolicyAndVersion(t *testing.T) {
	for _, tc := range []struct {
		name                             string
		consented, declared, model, held bool
		want                             bool
	}{
		{"all required facts", true, true, true, false, true},
		{"old snapshot", false, true, true, false, false},
		{"not declared", true, false, true, false, false},
		{"no gateway outlet", true, true, false, false, false},
		{"skill withheld", true, true, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			version := VersionFacts{}
			if tc.declared {
				version.Compatibility = "skillhub-web-search"
			}
			policy := policySnapshot{WebSearch: tc.consented, SkillWithheld: tc.held}
			if tc.model {
				policy.Egress.Allow = []egressAllow{{Purpose: "model_gateway", URL: "http://gateway.test"}}
			}
			if got := policy.searchAllowedFor(version); got != tc.want {
				t.Fatalf("search authorization = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSearchRequirementsSurviveTheImmutablePolicyRoundTrip(t *testing.T) {
	policy := versionPolicy(Deployment{GatewayURL: "http://gateway.test"}, VersionFacts{Compatibility: "skillhub-web-search"})
	encoded, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"web_search":true`) {
		t.Fatalf("the frozen policy lost the search requirement: %s", encoded)
	}
	req, decoded, err := requirementsFor(encoded)
	if err != nil || !req.WebSearch || !decoded.WebSearch {
		t.Fatalf("search requirement was lost at dispatch: req=%+v policy=%+v err=%v", req, decoded, err)
	}
	req, decoded, err = requirementsFor([]byte(`{"egress":{"mode":"default_deny","allow":[]}}`))
	if err != nil || req.WebSearch || decoded.WebSearch {
		t.Fatalf("a legacy Run acquired search: req=%+v policy=%+v err=%v", req, decoded, err)
	}
	policy.SkillWithheld = true
	if !requirementsFromPolicy(policy).WebSearch {
		t.Fatal("a Run withholding its Skill changed the required runtime")
	}
	if _, _, err := requirementsFor([]byte(`{"web_search":"true"}`)); err == nil {
		t.Fatal("an unreadable search policy was accepted as authorization")
	}
}

func TestSearchMatchesOnlyARuntimeThatDeclaresSupport(t *testing.T) {
	for _, required := range []bool{false, true} {
		for _, offered := range []string{"", "supported"} {
			name := map[bool]string{false: "ordinary", true: "search"}[required] + "/" + map[string]string{"": "old runtime", "supported": "search runtime"}[offered]
			t.Run(name, func(t *testing.T) {
				c := compatible()
				c.Runtimes[0].WebSearch = offered
				req := defaultRequirements()
				req.WebSearch = required
				profile, err := Match(c, req)
				if required && offered != "supported" {
					if err == nil || !strings.Contains(err.Error(), "web search") {
						t.Fatalf("unsupported search was not refused: %+v, %v", profile, err)
					}
					return
				}
				if err != nil || profile.RuntimeVersion != "0.4.2" {
					t.Fatalf("a supported request was refused or selected wrongly: %+v, %v", profile, err)
				}
			})
		}
	}
}

func TestSearchSelectsTheSupportingRuntimeEntry(t *testing.T) {
	c := compatible()
	old := c.Runtimes[0]
	old.Versions = []string{"1"}
	current := old
	current.Versions, current.WebSearch = []string{"2"}, "supported"
	c.Runtimes = []RuntimeSupport{old, current}
	req := defaultRequirements()
	req.WebSearch = true
	profile, err := Match(c, req)
	if err != nil || profile.RuntimeVersion != "2" {
		t.Fatalf("search selected an unsupported runtime entry: %+v, %v", profile, err)
	}
}

func TestSearchCapabilityUnavailabilityWaitsWhileUnsupportedRefuses(t *testing.T) {
	for _, tc := range []struct {
		state string
		want  error
	}{
		{"", ErrNoCompatibleProvider},
		{"unsupported", ErrNoCompatibleProvider},
		{"unavailable", ErrNoSandboxAvailableYet},
		{"unrecognized", ErrNoSandboxAvailableYet},
		{"supported", nil},
	} {
		t.Run(tc.state, func(t *testing.T) {
			cap := compatible()
			cap.Runtimes[0].WebSearch = tc.state
			req := defaultRequirements()
			req.WebSearch = true
			_, err := registryWithCapabilities(cap).Select(context.Background(), req)
			if !errors.Is(err, tc.want) {
				t.Fatalf("search state %q selected with %v, want %v", tc.state, err, tc.want)
			}
			req.WebSearch = false
			if _, err := registryWithCapabilities(cap).Select(context.Background(), req); err != nil {
				t.Fatalf("ordinary run was refused because of search state %q: %v", tc.state, err)
			}
		})
	}
}

func TestSearchSkipsEarlierEntriesWithWrongIntegrationOrUnreadableEvidence(t *testing.T) {
	for _, state := range []string{"supported", "unavailable"} {
		cap := compatible()
		first := cap.Runtimes[0]
		first.Versions, first.WebSearch = []string{"wrong"}, state
		first.AgentIntegration = []string{"provider_hosted_agent"}
		ready := cap.Runtimes[0]
		ready.WebSearch = "supported"
		cap.Runtimes = []RuntimeSupport{first, ready}
		req := defaultRequirements()
		req.WebSearch = true
		profile, err := Match(cap, req)
		if err != nil || profile.RuntimeVersion != "0.4.2" {
			t.Fatalf("earlier integration %q hid a ready entry: %+v, %v", state, profile, err)
		}
		first.AgentIntegration = ready.AgentIntegration
		first.WebSearch = "unavailable"
		cap.Runtimes[0] = first
		if profile, err := Match(cap, req); err != nil || profile.RuntimeVersion != "0.4.2" {
			t.Fatalf("unreadable entry hid a ready entry: %+v, %v", profile, err)
		}
	}
}
