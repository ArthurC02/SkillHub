package sandbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func routedConfig() Config {
	return Config{
		Runtimes:     []RuntimeCapability{{Runtime: "claude_agent_sdk", Versions: []string{"1"}}},
		MaxResources: DefaultLimits,
		EgressModes:  []string{"default_deny", "none"},
		EgressAllow: []EgressDestination{{
			Purpose: "model_gateway", FQDN: "litellm.internal",
			PinnedIP: "10.20.30.40", Port: 4000, Protocol: "tcp",
		}},
	}
}

func routedRequest(allow ...EgressAllowEntry) RunRequest {
	return RunRequest{
		Runtime:        RuntimeProfile{Runtime: "claude_agent_sdk", RuntimeVersion: "1"},
		ResourceLimits: DefaultLimits,
		Egress:         EgressPolicy{Mode: "default_deny", Allow: allow},
	}
}

func TestAcceptRefusesADestinationThisNodeRendersNoRuleFor(t *testing.T) {
	cfg := routedConfig()
	for _, tc := range []struct {
		name string
		url  string
		want bool
	}{
		{"the rendered destination by name", "http://litellm.internal:4000", true},
		{"the rendered destination by its pinned address", "http://10.20.30.40:4000", true},
		{"the rendered host on a port with no rule", "http://litellm.internal:5432", false},
		{"a different host on the rendered port", "http://litellm.example.com:4000", false},
		{"the node's own loopback", "http://127.0.0.1:4000", false},
		{"no port, and http implies one this node does not route", "http://litellm.internal", false},
		{"a scheme that implies no port at all", "gopher://litellm.internal", false},
		{"not a URL", "litellm.internal:4000", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			re := cfg.accept(routedRequest(EgressAllowEntry{Purpose: "model_gateway", URL: tc.url}))
			if tc.want && re != nil {
				t.Fatalf("%s was refused: %v", tc.url, re)
			}
			if !tc.want {
				if re == nil {
					t.Fatalf("%s was accepted, and this node has no accept rule for it: "+
						"the run would be dispatched and time out instead of being refused", tc.url)
				}
				if re.Class != ClassCapabilityMismatch {
					t.Fatalf("%s was refused as %q, want %q", tc.url, re.Class, ClassCapabilityMismatch)
				}
			}
		})
	}
}

func TestAPurposeThisNodeRoutesDoesNotRouteAnotherHost(t *testing.T) {
	cfg := routedConfig()
	re := cfg.accept(routedRequest(EgressAllowEntry{Purpose: "model_gateway", URL: "http://attacker.example:4000"}))
	if re == nil {
		t.Fatal("a request naming an unrelated host was accepted because it called itself model_gateway")
	}

	if !strings.Contains(re.Message, "model_gateway:4000") {
		t.Errorf("refusal does not say what this node does route: %q", re.Message)
	}
	if strings.Contains(re.Message, "10.20.30.40") {
		t.Errorf("refusal leaks the node's rendered address: %q", re.Message)
	}
}

func TestANodeThatRendersNothingRefusesEveryDestination(t *testing.T) {
	cfg := routedConfig()
	cfg.EgressAllow = nil
	re := cfg.accept(routedRequest(EgressAllowEntry{Purpose: "model_gateway", URL: "http://litellm.internal:4000"}))
	if re == nil || re.Class != ClassCapabilityMismatch {
		t.Fatalf("a node with nothing rendered accepted a destination: %v", re)
	}
	if !strings.Contains(re.Message, "no destination has a pinned address") {
		t.Errorf("the refusal does not say why this node routes nowhere: %q", re.Message)
	}

	if re := cfg.accept(routedRequest()); re != nil {
		t.Fatalf("a node with no rendered destination refused a run that needs none: %v", re)
	}
}

func TestANodeDeclaresNoEgressRouteUntilSomethingIsRendered(t *testing.T) {
	rendered := []EgressDestination{{Purpose: "model_gateway", FQDN: "gw", Port: 4000}}
	for _, tc := range []struct {
		name     string
		network  string
		rendered []EgressDestination
		want     string
	}{
		{"a network and a destination", "skillhub_egress", rendered, "default_deny"},
		{"a network and nothing rendered", "skillhub_egress", nil, "none"},
		{"no network at all", "none", rendered, "none"},
		{"unset network", "", rendered, "none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := EgressModesFor(tc.network, tc.rendered)
			if got[0] != tc.want {
				t.Errorf("EgressModesFor(%q, %d rendered) = %v, want %q first", tc.network, len(tc.rendered), got, tc.want)
			}

			if !slices.Contains(got, "none") {
				t.Errorf("EgressModesFor(%q, ...) = %v, which cannot carry a run needing no egress", tc.network, got)
			}
		})
	}
}

func TestLoadEgressAllowRefusesWhatItCannotEnforce(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	for _, tc := range []struct {
		name, body string
	}{
		{"not json", "destinations: []"},
		{"a destination with no purpose", `{"destinations":[{"fqdn":"gw","port":4000}]}`},
		{"a destination with no port", `{"destinations":[{"purpose":"model_gateway","fqdn":"gw"}]}`},
		{"a port one past the largest", `{"destinations":[{"purpose":"model_gateway","fqdn":"gw","port":65536}]}`},
		{"a negative port", `{"destinations":[{"purpose":"model_gateway","fqdn":"gw","port":-1}]}`},
		{"a destination naming no host", `{"destinations":[{"purpose":"model_gateway","port":4000}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := LoadEgressAllow(write(tc.name+".json", tc.body)); err == nil {
				t.Fatalf("loaded a list that renders no rule: %s", tc.body)
			}
		})
	}
	if _, err := LoadEgressAllow(filepath.Join(dir, "absent.json")); err == nil {
		t.Fatal("a missing file loaded as an empty list; a node with a network and no rendered file " +
			"would then advertise a route and refuse everything sent to it")
	}
	for _, tc := range []struct {
		name, body string
		want       EgressDestination
	}{
		{
			"every field",
			`{"destinations":[{"purpose":"model_gateway","fqdn":"gw","pinned_ip":"10.0.0.1","port":4000,"protocol":"tcp"}]}`,
			EgressDestination{Purpose: "model_gateway", FQDN: "gw", PinnedIP: "10.0.0.1", Port: 4000, Protocol: "tcp"},
		},
		{
			"a pinned address and no name, on the largest port",
			`{"destinations":[{"purpose":"pypi","pinned_ip":"10.0.0.2","port":65535}]}`,
			EgressDestination{Purpose: "pypi", PinnedIP: "10.0.0.2", Port: 65535},
		},
		{
			"a name and no pinned address, on the smallest port",
			`{"destinations":[{"purpose":"pypi","fqdn":"pypi.example","port":1}]}`,
			EgressDestination{Purpose: "pypi", FQDN: "pypi.example", Port: 1},
		},
	} {
		t.Run("loads "+tc.name, func(t *testing.T) {
			p := write(tc.name+".json", tc.body)
			got, err := LoadEgressAllow(p)
			if err != nil || !slices.Equal(got, []EgressDestination{tc.want}) {
				t.Fatalf("LoadEgressAllow(%s) = %+v, %v; want [%+v]", tc.body, got, err, tc.want)
			}
		})
	}
}

func TestTheCommittedRenderedListIsReadableByTheNode(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "infra", "egress", "rendered", "egress-allow.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no rendered allow list at %s: %v", path, err)
	}
	if _, err := LoadEgressAllow(path); err != nil {
		t.Fatalf("the committed rendered list does not load: %v", err)
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if got := string(doc["source"]); got != `"infra/egress/allowlist.yaml"` {
		t.Errorf("rendered from %s, want \"infra/egress/allowlist.yaml\"", got)
	}
	if _, ok := doc["destinations"]; !ok {
		t.Fatalf("the rendered file has no `destinations` key (it has %v); the loader reads that "+
			"name, so a rename here reaches the node as a silent \"this node routes nowhere\"", keysOf(doc))
	}
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func TestANodeThatEnforcesNothingAcceptsEveryDestinationAndDeclaresThat(t *testing.T) {
	cfg := routedConfig()
	cfg.EgressAllow = nil
	cfg.EgressUnenforced = true

	if re := cfg.accept(routedRequest(EgressAllowEntry{Purpose: "model_gateway", URL: "http://litellm.internal:4000"})); re != nil {
		t.Fatalf("a node that filters nothing refused a destination it can reach: %v", re)
	}

	req := routedRequest(EgressAllowEntry{Purpose: "model_gateway", URL: "http://litellm.internal:4000"})
	req.Egress.Mode = "none"
	if re := cfg.accept(req); re == nil {
		t.Error("egress mode none with an allow list was accepted because enforcement was off")
	}

	m := NewManager(&p02Driver{}, cfg, slog.New(slog.DiscardHandler))
	if got := m.Capability(context.Background()).Network; got == nil || !got.EgressUnenforced {
		t.Errorf("capability network = %+v, want egress_unenforced true", got)
	}
}
