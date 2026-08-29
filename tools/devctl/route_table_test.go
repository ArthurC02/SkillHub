package main

import (
	"fmt"
	"strings"
	"testing"
)

// The one that matters: 72 mounted routes against 72 documented operations, in
// the tree as it stands. A fixture-only checker is a checker nobody has pointed
// at the subject.
func TestTheRealRouteTableAndTheContractAgree(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := routeTableProblems(root); len(problems) > 0 {
		t.Fatalf("route table and %s disagree:\n%s", routeContractFile, strings.Join(problems, "\n"))
	}
	// Guard the scan's own reach in both directions, not only through the
	// floor inside the checker: if either side silently found nothing, the
	// comparison above would agree about the empty set.
	mounted, problems := mountedRoutes(root)
	if len(problems) > 0 {
		t.Fatalf("scanning the real route table reported problems: %v", problems)
	}
	if len(mounted) < routeTableFloor {
		t.Fatalf("scan found %d mounted routes, floor is %d", len(mounted), routeTableFloor)
	}
	// The documented exception, resolved rather than skipped.
	found := false
	for _, pattern := range mounted {
		if pattern == "POST /internal/trace/{token}" {
			found = true
		}
	}
	if !found {
		t.Fatalf("the one constant-built pattern (trace.IngestPath) did not resolve to "+
			"POST /internal/trace/{token}; got %v", mounted)
	}
}

// A whole fixture repo: two route tables, one contract, one string constant.
func writeRouteFixture(t *testing.T, extraRoute, extraOperation string) string {
	t.Helper()
	root := t.TempDir()
	var mounts strings.Builder
	// The floor is 60, so the fixture has to clear it on both sides or the
	// sentinel speaks instead of the comparison.
	var ops strings.Builder
	ops.WriteString("openapi: 3.1.0\npaths:\n")
	for i := 0; i < routeTableFloor+1; i++ {
		mounts.WriteString(fmt.Sprintf("\tmux.HandleFunc(\"GET /r%d\", h.x)\n", i))
		ops.WriteString(fmt.Sprintf("  /r%d:\n    get:\n      summary: r%d\n", i, i))
	}
	mounts.WriteString("\tmux.HandleFunc(\"POST \"+trace.IngestPath+\"{token}\", h.ingest)\n")
	ops.WriteString("  /internal/trace/{token}:\n    post:\n      summary: ingest\n")
	if extraRoute != "" {
		mounts.WriteString("\tmux.HandleFunc(\"" + extraRoute + "\", h.x)\n")
	}
	if extraOperation != "" {
		method, path, _ := strings.Cut(extraOperation, " ")
		ops.WriteString("  " + path + ":\n    " + strings.ToLower(method) + ":\n      summary: extra\n")
	}
	ops.WriteString("components:\n  schemas: {}\n")

	writeAt(t, root, routeTableSources[0], "package apiserver\n\nfunc NewRouter() {\n"+mounts.String()+"}\n")
	writeAt(t, root, routeTableSources[1], "package workspace\n\nfunc Mount() {\n}\n")
	writeAt(t, root, routePatternConstants["trace.IngestPath"].file,
		"package evidence\n\nconst IngestPath = \"/internal/trace/\"\n")
	writeAt(t, root, routeContractFile, ops.String())
	return root
}

func TestRouteTableAcceptsATreeWhereBothSidesAgree(t *testing.T) {
	t.Parallel()
	if problems := routeTableProblems(writeRouteFixture(t, "", "")); len(problems) != 0 {
		t.Fatalf("a matching pair was rejected: %v", problems)
	}
}

func TestRouteTableSpeaksInBothDirections(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name           string
		route, op      string
		want           string
		wantOccurrence int
	}{{
		name: "a route nobody documented",
		// The M4/M5 shape: a route added to NewRouter, contract untouched.
		route: "GET /skills/{id}/undocumented",
		want:  `"GET /skills/{id}/undocumented" is mounted but has no operation`,
	}, {
		name: "an operation nobody mounts",
		op:   "DELETE /skills/{id}/ghost",
		want: `documents "DELETE /skills/{id}/ghost", which nothing mounts`,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			problems := routeTableProblems(writeRouteFixture(t, tc.route, tc.op))
			if len(problems) != 1 || !strings.Contains(problems[0], tc.want) {
				t.Fatalf("want exactly one problem containing %q, got %v", tc.want, problems)
			}
		})
	}
}

// Both floors, and the unknown-identifier refusal. Each of these is green under
// a naive implementation that skips what it cannot read.
func TestRouteTableSaysSoWhenItHasLostItsSubject(t *testing.T) {
	t.Parallel()
	t.Run("the mount scan matches nothing", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		writeAt(t, root, routeTableSources[0], "package apiserver\n\n// nothing mounts here any more\n")
		writeAt(t, root, routeTableSources[1], "package workspace\n")
		writeAt(t, root, routeContractFile, "paths:\n  /x:\n    get: {}\n")
		problems := routeTableProblems(root)
		if len(problems) != 1 || !strings.Contains(problems[0], "the scan is broken rather than the table shrunk") {
			t.Fatalf("an empty scan was accepted: %v", problems)
		}
	})
	t.Run("the contract scan matches nothing", func(t *testing.T) {
		t.Parallel()
		root := writeRouteFixture(t, "", "")
		writeAt(t, root, routeContractFile, "openapi: 3.1.0\ncomponents:\n  schemas: {}\n")
		problems := routeTableProblems(root)
		if len(problems) != 1 || !strings.Contains(problems[0], "has no operations under `paths:`") {
			t.Fatalf("a contract with no paths block was accepted: %v", problems)
		}
	})
	t.Run("a pattern built from an identifier nobody declared", func(t *testing.T) {
		t.Parallel()
		root := writeRouteFixture(t, "", "")
		writeAt(t, root, routeTableSources[1],
			"package workspace\n\nfunc Mount() {\n\tmux.HandleFunc(\"GET \"+someOtherPackage.Path, h.x)\n}\n")
		problems := routeTableProblems(root)
		if len(problems) != 1 || !strings.Contains(problems[0], "silently missing a route") {
			t.Fatalf("an unresolvable pattern was skipped instead of refused: %v", problems)
		}
	})
	t.Run("the resolved constant was deleted", func(t *testing.T) {
		t.Parallel()
		root := writeRouteFixture(t, "", "")
		writeAt(t, root, routePatternConstants["trace.IngestPath"].file,
			"package evidence\n\n// IngestPath used to live here.\n")
		problems := routeTableProblems(root)
		if len(problems) != 1 || !strings.Contains(problems[0], "no longer declares the constant IngestPath") {
			t.Fatalf("a deleted constant was resolved anyway: %v", problems)
		}
	})
}
