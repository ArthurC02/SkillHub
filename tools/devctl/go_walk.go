package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type platformGoFile struct {
	pkg  string
	rel  string
	fset *token.FileSet
	file *ast.File
}

func walkGoFiles(base string, visit func(path string) error) error {
	return filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		return visit(path)
	})
}

func walkPlatformGo(internal string, visit func(platformGoFile) error) error {
	return walkGoFiles(internal, func(path string) error {
		rel, err := filepath.Rel(internal, path)
		if err != nil {
			return err
		}
		pkg := filepath.ToSlash(filepath.Dir(rel))
		if generatedPackage(pkg) {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		return visit(platformGoFile{pkg: pkg, rel: filepath.ToSlash(rel), fset: fset, file: file})
	})
}
