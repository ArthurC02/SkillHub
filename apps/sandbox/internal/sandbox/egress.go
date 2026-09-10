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

type EgressDestination struct {
	Purpose  string `json:"purpose"`
	FQDN     string `json:"fqdn"`
	PinnedIP string `json:"pinned_ip"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

type renderedEgress struct {
	Source       string              `json:"source"`
	Destinations []EgressDestination `json:"destinations"`
}

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

func EgressModesFor(network string, rendered []EgressDestination) []string {
	if network == "" || network == "none" || len(rendered) == 0 {
		return []string{"none"}
	}
	return []string{"default_deny", "none"}
}
