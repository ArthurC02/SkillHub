package main

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"testing"
)

type countedWiring struct {
	calls int
	edges map[contextEdge]string
	err   error
}

func (c *countedWiring) compute() (map[contextEdge]string, error) {
	c.calls++
	return c.edges, c.err
}

func wiringModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeAt(t, dir, "go.mod", "module example.com/m\n\ngo 1.22\n")
	writeAt(t, dir, "internal/a/a.go", "package a\n")
	return dir
}

var someIdentities = map[string]packageIdentity{"a": {Kind: architectureCore, ID: "a", Path: "a"}}

const someChecker = "checker-v1"

var someEdges = map[contextEdge]string{
	{"a", "b"}: "internal/entrypoint/wire.go:12",
	{"b", "c"}: "cmd/api/main.go:40",
}

func TestUnchangedSourcesReuseTheWiringEdges(t *testing.T) {
	t.Parallel()
	module, cache := wiringModule(t), t.TempDir()
	counted := &countedWiring{edges: someEdges}
	for range 2 {
		edges, err := cachedWiringEdges(cache, someChecker, module, someIdentities, counted.compute)
		if err != nil || !maps.Equal(edges, someEdges) {
			t.Fatalf("edges = %v, %v; want %v", edges, err, someEdges)
		}
	}
	if counted.calls != 1 {
		t.Fatalf("computed %d times, want once", counted.calls)
	}
}

func TestWhatInvalidatesTheWiringEdges(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		change     func(t *testing.T, module string)
		identities map[string]packageIdentity
		checker    string
		recomputed bool
	}{
		{"an edited source", func(t *testing.T, m string) { writeAt(t, m, "internal/a/a.go", "package a\n\nvar X = 1\n") }, someIdentities, someChecker, true},
		{"an added source", func(t *testing.T, m string) { writeAt(t, m, "internal/b/b.go", "package b\n") }, someIdentities, someChecker, true},
		{"an edited go.sum", func(t *testing.T, m string) { writeAt(t, m, "go.sum", "example.com/x v1.0.0 h1:x=\n") }, someIdentities, someChecker, true},
		{"a renamed source", func(t *testing.T, m string) {
			if err := os.Rename(filepath.Join(m, "internal/a/a.go"), filepath.Join(m, "internal/a/b.go")); err != nil {
				t.Fatal(err)
			}
		}, someIdentities, someChecker, true},
		{"a changed context map", func(*testing.T, string) {}, map[string]packageIdentity{"a": {Kind: architectureCore, ID: "renamed", Path: "a"}}, someChecker, true},
		{"a rebuilt checker", func(*testing.T, string) {}, someIdentities, "checker-v2", true},
		{"an edited test file", func(t *testing.T, m string) { writeAt(t, m, "internal/a/a_test.go", "package a\n") }, someIdentities, someChecker, false},
		{"an edited document", func(t *testing.T, m string) { writeAt(t, m, "README.md", "notes\n") }, someIdentities, someChecker, false},
		{"a file under testdata", func(t *testing.T, m string) { writeAt(t, m, "internal/a/testdata/x.go", "package x\n") }, someIdentities, someChecker, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			module, cache := wiringModule(t), t.TempDir()
			counted := &countedWiring{edges: someEdges}
			if _, err := cachedWiringEdges(cache, someChecker, module, someIdentities, counted.compute); err != nil {
				t.Fatal(err)
			}
			c.change(t, module)
			if _, err := cachedWiringEdges(cache, c.checker, module, c.identities, counted.compute); err != nil {
				t.Fatal(err)
			}
			if got := counted.calls == 2; got != c.recomputed {
				t.Fatalf("recomputed = %v, want %v", got, c.recomputed)
			}
		})
	}
}

func TestAnUnreadableCachedResultIsComputedAgainNotTakenAsNoEdges(t *testing.T) {
	t.Parallel()
	module, cache := wiringModule(t), t.TempDir()
	counted := &countedWiring{edges: someEdges}
	if _, err := cachedWiringEdges(cache, someChecker, module, someIdentities, counted.compute); err != nil {
		t.Fatal(err)
	}
	stored, err := filepath.Glob(filepath.Join(cache, "*"))
	if err != nil || len(stored) != 1 {
		t.Fatalf("stored results = %v, %v; want one", stored, err)
	}
	for _, corrupt := range []string{"{not json", "null"} {
		if err := os.WriteFile(stored[0], []byte(corrupt), 0o600); err != nil {
			t.Fatal(err)
		}
		edges, err := cachedWiringEdges(cache, someChecker, module, someIdentities, counted.compute)
		if err != nil || !maps.Equal(edges, someEdges) {
			t.Fatalf("%q: edges = %v, %v; want the computed ones", corrupt, edges, err)
		}
	}
	if counted.calls != 3 {
		t.Fatalf("computed %d times, want 3", counted.calls)
	}
}

func TestNoWiringEdgesIsAResultWorthKeeping(t *testing.T) {
	t.Parallel()
	module, cache := wiringModule(t), t.TempDir()
	counted := &countedWiring{edges: map[contextEdge]string{}}
	for range 2 {
		edges, err := cachedWiringEdges(cache, someChecker, module, someIdentities, counted.compute)
		if err != nil || len(edges) != 0 || edges == nil {
			t.Fatalf("edges = %#v, %v; want an empty map", edges, err)
		}
	}
	if counted.calls != 1 {
		t.Fatalf("computed %d times, want once", counted.calls)
	}
}

func TestAFailedWiringCheckIsNotKept(t *testing.T) {
	t.Parallel()
	module, cache := wiringModule(t), t.TempDir()
	failing := &countedWiring{err: errors.New("go list: exit status 1")}
	if _, err := cachedWiringEdges(cache, someChecker, module, someIdentities, failing.compute); err == nil || err.Error() != "go list: exit status 1" {
		t.Fatalf("err = %v, want the check's own error", err)
	}
	counted := &countedWiring{edges: someEdges}
	edges, err := cachedWiringEdges(cache, someChecker, module, someIdentities, counted.compute)
	if err != nil || counted.calls != 1 || !maps.Equal(edges, someEdges) {
		t.Fatalf("after a failure: edges = %v, err = %v, computed %d times; want a fresh computation", edges, err, counted.calls)
	}
}

func TestWithoutACacheDirectoryTheEdgesAreAlwaysComputed(t *testing.T) {
	t.Parallel()
	module := wiringModule(t)
	counted := &countedWiring{edges: someEdges}
	for range 2 {
		if _, err := cachedWiringEdges("", someChecker, module, someIdentities, counted.compute); err != nil {
			t.Fatal(err)
		}
	}
	if counted.calls != 2 {
		t.Fatalf("computed %d times, want twice", counted.calls)
	}
}

func TestWithoutAKnownCheckerTheEdgesAreAlwaysComputed(t *testing.T) {
	t.Parallel()
	module, cache := wiringModule(t), t.TempDir()
	counted := &countedWiring{edges: someEdges}
	for range 2 {
		if _, err := cachedWiringEdges(cache, "", module, someIdentities, counted.compute); err != nil {
			t.Fatal(err)
		}
	}
	if counted.calls != 2 {
		t.Fatalf("computed %d times, want twice", counted.calls)
	}
}

func TestTheRunningCheckerIsIdentifiedByItsExecutable(t *testing.T) {
	t.Parallel()
	if got := runningChecker(); len(got) != 64 {
		t.Fatalf("runningChecker() = %q, want a SHA-256 of the test binary", got)
	}
}
