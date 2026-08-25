package sandbox

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// EgressDestination is one place a sandbox on this node actually has a route to.
//
// It is rendered, never written by hand: tools/egress/render.py turns
// infra/egress/allowlist.yaml into the node's nftables ruleset, its resolver
// config, and this list, from the same entries in the same pass. That is the
// point of the file existing at all. `04` 甲-3 recorded what happened without
// it -- `Egress.Allow[].URL` was shown to the user in the pre-run permission
// summary, written into the run record, and then read by nothing. The only
// enforcement was the structural accident that one Docker network happened to
// have one service on it. A destination a user agrees to, with nothing holding
// the run to it, is narration.
//
// A destination whose `pinned_ip` is still `unset` is deliberately NOT in this
// list. The ruleset has no accept rule for it, so a run allowed to name it
// would be dispatched and then fail to connect, and "accepted, then timed out"
// is the behaviour ADR-022 A1-e exists to replace with a refusal.
type EgressDestination struct {
	Purpose  string `json:"purpose"`
	FQDN     string `json:"fqdn"`
	PinnedIP string `json:"pinned_ip"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

// renderedEgress is the file tools/egress/render.py writes.
type renderedEgress struct {
	Source       string              `json:"source"`
	Destinations []EgressDestination `json:"destinations"`
}

// LoadEgressAllow reads a rendered allow list from disk.
//
// A missing path is an error rather than an empty list, and the difference
// matters: an empty list is a node that renders no destination and correctly
// declares no egress route, while a missing file is a node whose operator
// believed it had one. Silently treating the second as the first would leave
// the capability response claiming `default_deny` while every dispatch to it
// was refused -- a black hole the scheduler keeps feeding.
func LoadEgressAllow(path string) ([]EgressDestination, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read rendered egress allow list: %w", err)
	}
	var doc renderedEgress
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse rendered egress allow list %s: %w", path, err)
	}
	for i, d := range doc.Destinations {
		switch {
		case d.Purpose == "":
			return nil, fmt.Errorf("%s: destination %d has no purpose", path, i)
		case d.Port <= 0 || d.Port > 65535:
			return nil, fmt.Errorf("%s: destination %q has port %d, which renders no rule", path, d.Purpose, d.Port)
		case d.FQDN == "" && d.PinnedIP == "":
			return nil, fmt.Errorf("%s: destination %q names no host", path, d.Purpose)
		}
	}
	return doc.Destinations, nil
}

// routes reports whether this node has a rendered route for one requested
// destination (ADR-022 A1-e).
//
// Purpose alone is not enough and never was: the request carries a URL, the
// user was shown that URL, and the node has an accept rule for one address and
// one port. Matching on purpose would accept a request naming any host at all
// as long as it called itself `model_gateway`.
//
// Either spelling of the host is accepted, because both name the same rendered
// destination: the platform sends the FQDN today (execution.GatewayURL), and a
// deployment that addresses the gateway by its pinned address is describing the
// same accept rule, not a different one.
func (d EgressDestination) routes(want EgressAllowEntry) bool {
	if d.Purpose != want.Purpose {
		return false
	}
	host, port, ok := hostPort(want.URL)
	if !ok || port != d.Port {
		return false
	}
	return strings.EqualFold(host, d.FQDN) || host == d.PinnedIP
}

// hostPort splits a destination URL into the pair an accept rule is written in.
//
// A URL with no explicit port still names one, and the scheme says which: a
// rule is `ip daddr X tcp dport N`, so "no port" is not a thing the ruleset can
// express. An unparseable URL, or one whose scheme implies nothing, returns
// false and is refused by the caller -- guessing 80 for a scheme nobody
// recognised would invent a rule the node does not have.
func hostPort(raw string) (string, int, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", 0, false
	}
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		host = u.Host
		switch strings.ToLower(u.Scheme) {
		case "http":
			portStr = "80"
		case "https":
			portStr = "443"
		default:
			return "", 0, false
		}
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, false
	}
	return host, port, true
}

// EgressModesFor reports what a node can honestly advertise.
//
// Two inputs, and both have to hold: a network to attach a sandbox to, and at
// least one destination rendered onto it. A node with a network and an empty
// rendered list has no route to offer -- every dispatch naming a destination
// would be refused by accept() -- so it declares `none` and the scheduler stops
// sending it work it will always turn away (RUN-005).
//
// That is the same ordering the contract already states for none versus
// default_deny, applied to one more input. Declaring `default_deny` and then
// refusing every destination is not a stricter node; it is a black hole.
func EgressModesFor(network string, rendered []EgressDestination) []string {
	if network == "" || network == "none" || len(rendered) == 0 {
		return []string{"none"}
	}
	return []string{"default_deny", "none"}
}
