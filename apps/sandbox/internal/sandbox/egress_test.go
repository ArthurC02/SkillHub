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

// The node this suite describes: one rendered destination, one address, one port.
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

// ADR-022 A1-e, and the case that matters is the near miss.
//
// `04` 甲-3 recorded that `Egress.Allow[].URL` was read by nothing at all: it was
// shown to the user, stored on the run, and dropped. The check that replaced
// that has to be on the address and the port, because a check on the purpose
// would accept every one of these rows while the node has a rule for exactly one
// of them.
func TestAcceptRefusesADestinationThisNodeRendersNoRuleFor(t *testing.T) {
	cfg := routedConfig()
	for _, tc := range []struct {
		name string
		url  string
		want bool // want accepted
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
						"the run would be dispatched and time out instead of being refused (ADR-022 A1-e)", tc.url)
				}
				if re.Class != ClassCapabilityMismatch {
					t.Fatalf("%s was refused as %q, want %q", tc.url, re.Class, ClassCapabilityMismatch)
				}
			}
		})
	}
}

// The purpose is a label the caller chooses, so it cannot be the thing that
// grants a route. This is the hole the old code had: dockerdrv attached the
// egress network to anything whose purpose read `model_gateway`, whatever host
// it named.
func TestAPurposeThisNodeRoutesDoesNotRouteAnotherHost(t *testing.T) {
	cfg := routedConfig()
	re := cfg.accept(routedRequest(EgressAllowEntry{Purpose: "model_gateway", URL: "http://attacker.example:4000"}))
	if re == nil {
		t.Fatal("a request naming an unrelated host was accepted because it called itself model_gateway")
	}
	// The refusal has to be usable by whoever reads the dispatch failure, and it
	// must not put the node's topology in there: purpose and port, never the
	// address, which lives in the node's own rendered file.
	if !strings.Contains(re.Message, "model_gateway:4000") {
		t.Errorf("refusal does not say what this node does route: %q", re.Message)
	}
	if strings.Contains(re.Message, "10.20.30.40") {
		t.Errorf("refusal leaks the node's rendered address: %q", re.Message)
	}
}

// A destination whose pin is still `unset` is rendered nowhere, and a node in
// that state must refuse rather than accept-and-hang.
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
	// It still carries a run that is allowed to reach nothing.
	if re := cfg.accept(routedRequest()); re != nil {
		t.Fatalf("a node with no rendered destination refused a run that needs none: %v", re)
	}
}

// A node must not advertise a route it will refuse every time. Declaring
// default_deny with nothing rendered is not a stricter node, it is a black hole
// the scheduler keeps feeding (RUN-005).
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
			// `none` is always offered: it is stronger than default_deny, and a
			// node that can route can still carry a run allowed to reach nothing.
			if !contains(got, "none") {
				t.Errorf("EgressModesFor(%q, ...) = %v, which cannot carry a run needing no egress", tc.network, got)
			}
		})
	}
}

// A missing file is not an empty list. One is a node that renders nothing and
// says so; the other is a node whose operator believed it had a route.
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
		{"a port no rule can carry", `{"destinations":[{"purpose":"model_gateway","fqdn":"gw","port":70000}]}`},
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
	p := write("ok.json", `{"destinations":[{"purpose":"model_gateway","fqdn":"gw","pinned_ip":"10.0.0.1","port":4000,"protocol":"tcp"}]}`)
	got, err := LoadEgressAllow(p)
	if err != nil || len(got) != 1 || got[0].Port != 4000 {
		t.Fatalf("LoadEgressAllow(%s) = %v, %v", p, got, err)
	}
}

// The two sides of the same file, in two languages, and nothing else compares
// them. tools/egress/render.py writes infra/egress/rendered/egress-allow.json
// and this is the only code that reads it; a field renamed on either side would
// otherwise show up as a node that routes nowhere, which is a state the file is
// also allowed to be in legitimately.
//
// That last sentence is why this test used to prove nothing. It compared
// len(loaded) against len(raw destinations) — and the rendered list is empty by
// design today (every allowlist entry still has `pinned_ip: unset`), so the
// comparison was 0 != 0, false forever, on an assertion whose whole purpose was
// to notice a rename. Everything below is chosen to have teeth *while the list
// is empty*, because that is the state the file is in and will stay in until
// `05` R-23 is signed.
func TestTheCommittedRenderedListIsReadableByTheNode(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "infra", "egress", "rendered", "egress-allow.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("no rendered allow list at %s: %v", path, err)
	}
	dests, err := LoadEgressAllow(path)
	if err != nil {
		t.Fatalf("the committed rendered list does not load: %v", err)
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if got := string(doc["source"]); got != `"infra/egress/allowlist.yaml"` {
		t.Errorf("rendered from %s, want \"infra/egress/allowlist.yaml\"", got)
	}

	// 1. The key the destinations live under has to be the key the loader reads.
	// This is the rename the old assertion claimed to catch and could not: a
	// renderer that started writing `allow` instead of `destinations` would make
	// LoadEgressAllow return an empty list, and an empty list is a legitimate
	// state, so nothing downstream would ever say so. Unmarshalling into a map
	// is what makes "the key is absent" distinguishable from "the key is there
	// and holds nothing".
	rawDests, ok := doc["destinations"]
	if !ok {
		t.Fatalf("the rendered file has no `destinations` key (it has %v); the loader reads that "+
			"name, so a rename here reaches the node as a silent \"this node routes nowhere\"", keysOf(doc))
	}
	var listed []json.RawMessage
	if err := json.Unmarshal(rawDests, &listed); err != nil {
		t.Fatalf("`destinations` is not a list: %v", err)
	}

	// 2. The field names on the reader's side, pinned. json.Marshal round-trips
	// the struct tags LoadEgressAllow decodes with, so renaming one — the other
	// half of the same rename risk — goes red here immediately, with or without
	// a destination in the file.
	encoded, err := json.Marshal(EgressDestination{
		Purpose: "model_gateway", FQDN: "gw", PinnedIP: "10.0.0.1", Port: 4000, Protocol: "tcp",
	})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	want := []string{"fqdn", "pinned_ip", "port", "protocol", "purpose"}
	if got := keysOf(fields); !slices.Equal(got, want) {
		t.Errorf("EgressDestination encodes as %v, want %v: these are the names tools/egress/render.py "+
			"writes, and a renamed tag drops the field to its zero value rather than failing to parse", got, want)
	}

	// 3. Whatever the file does carry has to survive the loader intact.
	if len(dests) != len(listed) {
		t.Errorf("the loader kept %d of %d destinations: a field was renamed on one side",
			len(dests), len(listed))
	}

	// 4. And the tripwire for the reason (3) is vacuous. The empty list is
	// correct today and this test must not go red for it — but it must also not
	// keep reporting a pass for a comparison that compares nothing. So the
	// emptiness itself is the assertion: the day render.py emits a destination,
	// this fails and whoever made that happen has to come back and turn (3) into
	// the real check it was always meant to be. Same shape as Reaping() in
	// localdrv: assert what is true today so a change cannot pass silently.
	if len(listed) != 0 {
		t.Fatalf("the rendered list now has %d destination(s), so the round-trip check above is "+
			"finally measuring something — delete this branch and assert on the fields of dests[0] "+
			"(purpose/fqdn/pinned_ip/port/protocol) instead. It was empty because every allowlist "+
			"entry had `pinned_ip: unset` (05 R-23); that is evidently no longer true", len(listed))
	}
}

// keysOf is the sorted key set of a decoded JSON object, so a failure message
// names what was actually there rather than what was missing.
func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// A node that filters nothing accepts every destination, and says so in the
// capability rather than in its own log.
//
// This is the inverse of the case above, and the two together are the point.
// A1-e refuses a destination the node has no route to, because the alternative
// is a run that starts, hangs on its first model call and ends at the wall
// clock looking like a slow skill. A host process has a route to everything,
// so there is no destination it would hang on - refusing one would be the false
// answer here, not the safe one.
//
// What such a node cannot promise is the other half: that the workload reaches
// ONLY the destinations named. Before 2026-08-30 there was no field for that, so
// clean mode declared `none`, and every run carrying a model gateway grant was
// unschedulable - the demo could not produce one real trace event (04 丙-98).
func TestANodeThatEnforcesNothingAcceptsEveryDestinationAndDeclaresThat(t *testing.T) {
	cfg := routedConfig()
	cfg.EgressAllow = nil // a host process renders no nftables rule at all
	cfg.EgressUnenforced = true

	// The destination this node renders nothing for, and would refuse if it
	// claimed enforcement (the test above).
	if re := cfg.accept(routedRequest(EgressAllowEntry{Purpose: "model_gateway", URL: "http://litellm.internal:4000"})); re != nil {
		t.Fatalf("a node that filters nothing refused a destination it can reach: %v", re)
	}

	// The exemption is not a blanket one: everything accept() checks before the
	// destination loop still applies, including the mode itself.
	req := routedRequest(EgressAllowEntry{Purpose: "model_gateway", URL: "http://litellm.internal:4000"})
	req.Egress.Mode = "none"
	if re := cfg.accept(req); re == nil {
		t.Error("egress mode none with an allow list was accepted because enforcement was off")
	}

	// And the capability has to carry it. A node that behaves this way while
	// declaring `default_deny` and nothing else is precisely the divergence
	// ADR-022 A1-e and ADR-059 decision 3 both exist to make impossible; the
	// platform's gate reads this field and nothing else.
	m := NewManager(&p02Driver{}, cfg, slog.New(slog.DiscardHandler))
	if got := m.Capability(context.Background()).Network; got == nil || !got.EgressUnenforced {
		t.Errorf("capability network = %+v, want egress_unenforced true", got)
	}
}
