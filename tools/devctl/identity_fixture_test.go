package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type fixtureContext struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Subdomain string `json:"subdomain"`
	Path      string `json:"implementation_path"`
	Status    string `json:"status"`
}

func identitySources(identities string) (layout string, contexts []byte) {
	entries, _ := identityEntries(identities, contextMapDoc, nil)
	var yaml strings.Builder
	yaml.WriteString(identityListKey + "\n")
	reviewed := []fixtureContext{}
	for _, entry := range entries {
		switch architectureKind(entry.kind) {
		case architectureCore, architectureSupporting:
			reviewed = append(reviewed, fixtureContext{
				ID: entry.id, Name: entry.context, Path: entry.path, Status: "reviewed",
				Subdomain: strings.ToLower(entry.kind),
			})
		default:
			fmt.Fprintf(&yaml, "  - id: %s\n    kind: %s\n    path: %s\n", entry.id, entry.kind, entry.path)
			if entry.context != "" {
				fmt.Fprintf(&yaml, "    context: %s\n", entry.context)
			}
		}
	}
	body, err := json.Marshal(map[string]any{
		"format": "domain-contexts/v1", "status": "reviewed", "contexts": reviewed,
	})
	if err != nil {
		panic(err)
	}
	return yaml.String(), body
}

func writeIdentitySources(t *testing.T, root, identities string) {
	t.Helper()
	layout, contexts := identitySources(identities)
	for relative, body := range map[string][]byte{
		contextMapDoc:        []byte(layout),
		registryContextsFile: contexts,
	} {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

var fixtureAppendixID = regexp.MustCompile("`([a-z][a-z0-9_]*)`")

// Converts a legacy "| A → B | … | 保留 |" whitelist fixture into the reviewed
// dependency policies the checker now reads.
func writeDependencyPolicies(t *testing.T, root, appendix string, declared map[string]packageIdentity) {
	t.Helper()
	type policy struct {
		ID     string `json:"id"`
		From   string `json:"from_context"`
		To     string `json:"to_context"`
		Mode   string `json:"mode"`
		Policy string `json:"policy"`
		Status string `json:"status"`
	}
	policies := []policy{}
	seen := map[string]bool{}
	add := func(from, to string) {
		if from == to || seen[from+"→"+to] {
			return
		}
		seen[from+"→"+to] = true
		policies = append(policies, policy{
			ID: from + "-may-use-" + to, From: from, To: to,
			Mode: "synchronous-query", Policy: "allowed", Status: "reviewed",
		})
	}
	inAppendix := false
	for _, line := range strings.Split(appendix, "\n") {
		if strings.HasPrefix(line, "## ") {
			inAppendix = strings.HasPrefix(line, "## 跨 context import 白名單")
			continue
		}
		trimmed := strings.TrimSpace(line)
		if !inAppendix || !strings.HasPrefix(trimmed, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(trimmed, "|"), "|")
		if len(cells) != 3 || !strings.HasPrefix(strings.TrimSpace(cells[2]), "保留") {
			continue
		}
		left, right, arrow := strings.Cut(cells[0], "→")
		if !arrow {
			continue
		}
		var sources, targets []string
		for _, m := range fixtureAppendixID.FindAllStringSubmatch(left, -1) {
			if knownBoundaryID(declared, m[1]) {
				sources = append(sources, m[1])
			}
		}
		for _, m := range fixtureAppendixID.FindAllStringSubmatch(right, -1) {
			if knownBoundaryID(declared, m[1]) {
				targets = append(targets, m[1])
			}
		}
		if len(targets) == 0 {
			continue
		}
		if len(sources) == 0 {
			continue
		}
		for _, from := range sources {
			for _, to := range targets {
				add(from, to)
			}
		}
	}
	body, err := json.Marshal(map[string]any{
		"format": "domain-dependencies/v1", "status": "reviewed", "dependencies": policies,
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, filepath.FromSlash(dependencyPoliciesFile))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func identityEntriesAsIdentities(identities string) (map[string]packageIdentity, []string) {
	root, err := os.MkdirTemp("", "identity")
	if err != nil {
		panic(err)
	}
	layout, contexts := identitySources(identities)
	for relative, body := range map[string][]byte{
		contextMapDoc: []byte(layout), registryContextsFile: contexts,
	} {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(path, body, 0o600); err != nil {
			panic(err)
		}
	}
	declared, problems := architectureIdentities(root)
	os.RemoveAll(root)
	return declared, problems
}
