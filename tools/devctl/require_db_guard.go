package main

// requireDBGuard checks that every test package which disables itself when the
// test database URL is unset also honours SKILLHUB_REQUIRE_DB.
//
// 02:PORT-004. Five packages currently do this, and each one is a place where a
// misspelled variable, a service that failed to start, or a CI edit that drops
// the env block turns 281 assertions into silence while go test still prints
// ok. The switch is what makes that silence red; this check is what stops the
// sixth package from being added without it.
//
// A comment asking people to remember would have been cheaper and would have
// worked until the first time someone did not.

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	dbURLLiteral    = "SKILLHUB_TEST_DATABASE_URL"
	requireDBSwitch = `os.Getenv("SKILLHUB_REQUIRE_DB")`
	requireDBName   = "SKILLHUB_REQUIRE_DB"
	testMainExit    = "os.Exit(m.Run())"
)

// withoutComments reprints a Go file from its AST, which drops comments.
//
// The first version of this check searched the raw file for the switch's name,
// and its own mutation test stayed green: the guard's explanatory comment
// mentions SKILLHUB_REQUIRE_DB, so deleting the actual call changed nothing the
// check could see. A check a comment can satisfy is the thing it exists to
// prevent.
func withoutComments(path string, src []byte) string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		// Unparseable test files are the compiler's problem, not this check's;
		// fall back to the raw text rather than hiding the package.
		return string(src)
	}
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, file); err != nil {
		return string(src)
	}
	return buf.String()
}

// unguardedDBTestMains returns, relative to root, the test files that hand a
// database-gated package straight to m.Run() without consulting the switch.
func unguardedDBTestMains(root string) ([]string, error) {
	base := filepath.Join(root, "apps", "platform", "internal")
	// A package's env constant and its TestMain do not have to share a file, so
	// membership is decided per directory.
	dbPackages := map[string]bool{}
	type candidate struct{ dir, path, body string }
	var candidates []candidate

	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		body := withoutComments(path, b)
		dir := filepath.Dir(path)
		if strings.Contains(body, dbURLLiteral) {
			dbPackages[dir] = true
		}
		if strings.Contains(body, testMainExit) {
			candidates = append(candidates, candidate{dir, path, body})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var missing []string
	for _, c := range candidates {
		if !dbPackages[c.dir] || strings.Contains(c.body, requireDBSwitch) {
			continue
		}
		rel, err := filepath.Rel(root, c.path)
		if err != nil {
			rel = c.path
		}
		missing = append(missing, filepath.ToSlash(rel))
	}
	sort.Strings(missing)
	return missing, nil
}

func requireDBGuardCheck(root string) error {
	missing, err := unguardedDBTestMains(root)
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"these test packages skip every database test when %s is unset but ignore %s=1, "+
			"so a database that never came up reports success (02:PORT-004): %s",
		dbURLLiteral, requireDBName, strings.Join(missing, ", "))
}
