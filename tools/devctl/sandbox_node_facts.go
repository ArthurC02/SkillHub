package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type sandboxNodeSource struct {
	relative string
	pattern  *regexp.Regexp
}

type sandboxNodeFact struct {
	what    string
	from    sandboxNodeSource
	derive  func(declared string) string
	repeats []sandboxNodeSource
}

var sandboxNodeFacts = []sandboxNodeFact{
	{
		what:   "the docker bridge gateway",
		from:   sandboxNodeSource{filepath.Join("infra", "deploy", "sandbox", "daemon.json"), nil},
		derive: func(bip string) string { return strings.SplitN(bip, "/", 2)[0] },
		repeats: []sandboxNodeSource{
			{filepath.Join("infra", "deploy", "sandbox", "bin", "skillhub-bootstrap"),
				regexp.MustCompile(`(?m)^bridge_gateway=(\S+)`)},
		},
	},
	{
		what:   "the docker bridge subnet",
		from:   sandboxNodeSource{filepath.Join("infra", "deploy", "sandbox", "daemon.json"), nil},
		derive: subnetOfBIP,
		repeats: []sandboxNodeSource{
			{filepath.Join("infra", "deploy", "sandbox", "systemd", "skillhub-egress-flows.service"),
				regexp.MustCompile(`--orig-src\s+(\S+)`)},
		},
	},
	{
		what: "the sandboxd listening port",
		from: sandboxNodeSource{filepath.Join("infra", "deploy", "sandbox", "bin", "skillhub-bootstrap"),
			regexp.MustCompile(`SKILLHUB_SANDBOX_ADDR=\$SKILLHUB_PRIVATE_IP:(\d+)`)},
		repeats: []sandboxNodeSource{
			{filepath.Join("tools", "egress", "render.py"),
				regexp.MustCompile(`tcp dport (\d+) counter log prefix "skillhub-drop-sandboxd `)},
			{filepath.Join("tools", "egress", "render.py"),
				regexp.MustCompile(`ip saddr " \+ control_plane \+ " tcp dport (\d+) counter accept`)},
		},
	},
	{
		what: "the container runtime name",
		from: sandboxNodeSource{filepath.Join("infra", "deploy", "sandbox", "daemon.json"), nil},
		repeats: []sandboxNodeSource{
			{filepath.Join("infra", "deploy", "sandbox", "bin", "skillhub-bootstrap"),
				regexp.MustCompile(`(?m)^SKILLHUB_SANDBOX_RUNTIME=(\S+)`)},
			{filepath.Join("infra", "deploy", "sandbox", "bin", "skillhub-preflight"),
				regexp.MustCompile(`grep -qw ([A-Za-z0-9_.-]+)`)},
		},
	},
}

func subnetOfBIP(bip string) string {
	address, mask, found := strings.Cut(bip, "/")
	if !found {
		return bip
	}
	octets := strings.Split(address, ".")
	if len(octets) != 4 {
		return bip
	}
	octets[3] = "0"
	return strings.Join(octets, ".") + "/" + mask
}

func sandboxDaemonFact(root, what string) (string, error) {
	body, err := os.ReadFile(filepath.Join(root, "infra", "deploy", "sandbox", "daemon.json"))
	if err != nil {
		return "", err
	}
	var daemon struct {
		Runtimes map[string]json.RawMessage `json:"runtimes"`
		BIP      string                     `json:"bip"`
	}
	if err := json.Unmarshal(body, &daemon); err != nil {
		return "", fmt.Errorf("daemon.json: %w", err)
	}
	if what == "the container runtime name" {
		if len(daemon.Runtimes) != 1 {
			return "", fmt.Errorf("daemon.json declares %d runtimes; this check reads the one the node runs", len(daemon.Runtimes))
		}
		for name := range daemon.Runtimes {
			return name, nil
		}
	}
	if daemon.BIP == "" {
		return "", fmt.Errorf("daemon.json declares no bip, so the bridge address has no source")
	}
	return daemon.BIP, nil
}

func sandboxNodeValue(root string, source sandboxNodeSource, what string) (string, error) {
	if source.pattern == nil {
		return sandboxDaemonFact(root, what)
	}
	body, err := os.ReadFile(filepath.Join(root, source.relative))
	if err != nil {
		return "", err
	}
	match := source.pattern.FindSubmatch(body)
	if match == nil {
		return "", fmt.Errorf("%s: nothing matches %s, so this fact has no site to compare",
			filepath.ToSlash(source.relative), source.pattern)
	}
	return string(match[1]), nil
}

func sandboxNodeFactProblems(root string) []string {
	var problems []string
	for _, fact := range sandboxNodeFacts {
		declared, err := sandboxNodeValue(root, fact.from, fact.what)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		if fact.derive != nil {
			declared = fact.derive(declared)
		}
		for _, repeat := range fact.repeats {
			got, err := sandboxNodeValue(root, repeat, fact.what)
			if err != nil {
				problems = append(problems, err.Error())
				continue
			}
			if got != declared {
				problems = append(problems, fmt.Sprintf(
					"%s is %q in %s and %q in %s; changing one and not the other builds a node that cannot reach itself",
					fact.what, declared, filepath.ToSlash(fact.from.relative),
					got, filepath.ToSlash(repeat.relative)))
			}
		}
	}
	return problems
}
