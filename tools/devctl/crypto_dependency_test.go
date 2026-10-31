package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestShippedGoDependencyGraphsExcludeUnmaintainedOpenPGP(t *testing.T) {
	t.Parallel()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, module := range []string{"apps/platform", "apps/sandbox"} {
		t.Run(module, func(t *testing.T) {
			cmd := exec.Command("go", "list", "-deps", "-test", "./...")
			cmd.Dir = filepath.Join(root, filepath.FromSlash(module))
			cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
			data, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("dependency graph unavailable: %v\n%s", err, data)
			}
			packages := strings.Fields(string(data))
			if len(packages) == 0 {
				t.Fatal("dependency graph is empty")
			}
			for _, pkg := range packages {
				pkg = strings.TrimPrefix(pkg, "vendor/")
				if pkg == "golang.org/x/crypto/openpgp" || strings.HasPrefix(pkg, "golang.org/x/crypto/openpgp/") {
					t.Fatalf("unmaintained OpenPGP package is imported: %s", pkg)
				}
			}
		})
	}
}
