package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
