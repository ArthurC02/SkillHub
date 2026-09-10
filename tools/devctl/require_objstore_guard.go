package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	objstoreTestDir       = "apps/platform/internal/foundation/storage/objstore"
	objstoreEndpointName  = "SKILLHUB_TEST_OBJSTORE_ENDPOINT"
	requireObjstoreName   = "SKILLHUB_REQUIRE_OBJSTORE"
	requireObjstoreSwitch = `os.Getenv("SKILLHUB_REQUIRE_OBJSTORE")`
)

func requireObjstoreGuardProblems(root string) []string {
	dir := filepath.Join(root, filepath.FromSlash(objstoreTestDir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{fmt.Sprintf("require-objstore-guard: %s: %v", objstoreTestDir, err)}
	}

	var gated, guarded bool
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		source, err := os.ReadFile(path)
		if err != nil {
			return []string{fmt.Sprintf("require-objstore-guard: %s: %v", path, err)}
		}
		body := withoutComments(path, source)
		if strings.Contains(body, objstoreEndpointName) {
			gated = true
		}
		if strings.Contains(body, requireObjstoreSwitch) {
			guarded = true
		}
	}

	if !gated {
		return []string{fmt.Sprintf(
			"require-objstore-guard: no test in %s reads %s, so nothing there runs against a real S3 service; "+
				"SBX-008's three properties (expiry, signature, method binding) are proven nowhere else in this repository (02:PORT-009)",
			objstoreTestDir, objstoreEndpointName)}
	}
	if !guarded {
		return []string{fmt.Sprintf(
			"require-objstore-guard: %s gates itself on %s but ignores %s, so an object store that never came up "+
				"skips every short-lived authorization test and still reports success (02:PORT-009)",
			objstoreTestDir, objstoreEndpointName, requireObjstoreName)}
	}
	return nil
}
