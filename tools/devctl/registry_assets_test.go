package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureRegistryAsset(root, asset string) ([]byte, error) {
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(registryDirectory), "index.md")); err == nil {
		absolute, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		return exportedRegistryAsset(absolute, asset)
	}
	return os.ReadFile(filepath.Join(root, filepath.FromSlash(registryDirectory), asset+".json"))
}

func resetExportedRegistries(t *testing.T) {
	t.Helper()
	exportedRegistries.Lock()
	exportedRegistries.byRoot = map[string]map[string]json.RawMessage{}
	exportedRegistries.Unlock()
}

func stubDomainMemoryTool(t *testing.T, tool func(string, ...string) (string, error)) {
	t.Helper()
	original := domainMemoryTool
	t.Cleanup(func() { domainMemoryTool = original })
	domainMemoryTool = tool
	resetExportedRegistries(t)
}

func TestTheRepositoryRegistryIsReadThroughTheDomainMemoryExport(t *testing.T) {
	t.Parallel()
	resetExportedRegistries(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	contexts, err := exportedRegistryAsset(root, "contexts")
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	if !strings.Contains(string(contexts), `"id": "run"`) {
		t.Fatalf("the exported contexts do not hold the run Context:\n%s", contexts)
	}
}

func TestTheExportRunsOnceForEveryAssetOfOneRoot(t *testing.T) {
	calls := 0
	stubDomainMemoryTool(t, func(_ string, args ...string) (string, error) {
		calls++
		return `{"contexts": {"contexts": []}, "interactions": {"interactions": []}}`, nil
	})
	for _, asset := range []string{"contexts", "interactions", "contexts"} {
		if _, err := exportedRegistryAsset("root", asset); err != nil {
			t.Fatalf("%s: %v", asset, err)
		}
	}
	if calls != 1 {
		t.Fatalf("the export ran %d times, want 1", calls)
	}
}

func TestEachExportFailureIsNamed(t *testing.T) {
	cases := []struct {
		name, output string
		err          error
		asset, want  string
	}{
		{"tool fails", "uv: not found", errors.New("exit status 1"), "contexts", "the Domain Memory export failed: exit status 1: uv: not found"},
		{"no JSON", "nothing here", nil, "contexts", "the Domain Memory export printed no JSON: nothing here"},
		{"broken JSON", "{not json", nil, "contexts", "the Domain Memory export is not JSON"},
		{"asset absent", `{"rules": {}}`, nil, "contexts", "the Domain Memory export holds no contexts asset"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubDomainMemoryTool(t, func(string, ...string) (string, error) { return tc.output, tc.err })
			_, err := exportedRegistryAsset("root", tc.asset)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

func TestAWarningPrintedBeforeTheExportIsSkipped(t *testing.T) {
	stubDomainMemoryTool(t, func(string, ...string) (string, error) {
		return "NOTE: something on stderr\n{\"contexts\": {\"contexts\": [{\"id\": \"run\"}]}}", nil
	})
	contexts, err := exportedRegistryAsset("root", "contexts")
	if err != nil || !strings.Contains(string(contexts), `"run"`) {
		t.Fatalf("got %s, %v", contexts, err)
	}
}
