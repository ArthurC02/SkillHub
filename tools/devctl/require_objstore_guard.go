package main

// requireObjstoreGuard checks that the one test of SBX-008's short-lived
// authorization still exists and still fails closed.
//
// 02:PORT-009. Everything else in this repository that touches a pre-signed URL
// either does not verify one (objstore's in-process backend, which says so in
// its own header) or costs money and never runs in CI (the gateway end-to-end
// test). So a single package carries the whole claim that a grant expires, that
// its signature cannot be forged, and that a read ticket cannot write — and the
// requirement says in as many words that this class of test does not usually die
// by going red, it dies by being quietly skipped.
//
// Two ways it could be skipped, and this check watches both:
//
//   - the endpoint variable stops being set (a workflow edit, a service that
//     failed to start) and every assertion removes itself while go test prints
//     ok. SKILLHUB_REQUIRE_OBJSTORE=1 is what turns that into a failure.
//   - the test file itself goes away, which no amount of switch-checking would
//     notice, because a tree with no gated package is a tree with nothing to
//     complain about. That is why this check names the directory.
//
// Comments are stripped before matching, for the reason written at
// withoutComments in require_db_guard.go: the first version of the sibling
// check searched raw file text, and its own mutation stayed green because the
// guard's explanatory comment mentioned the variable. A check that a comment can
// satisfy is the thing it exists to prevent.

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
