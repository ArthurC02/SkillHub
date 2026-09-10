package main

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

func withoutComments(path string, src []byte) string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {

		return string(src)
	}
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, file); err != nil {
		return string(src)
	}
	return buf.String()
}

func unguardedDBTestMains(root string) ([]string, error) {
	base := filepath.Join(root, "apps", "platform", "internal")

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
