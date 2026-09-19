package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	modelWirePackage = "github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	modelWireRoot    = "apps/platform/internal"
	modelWireAdapter = "_adapter.go"
	modelWireOwner   = "apps/platform/internal/entrypoint/"
)

func modelWireBoundaryProblems(root string) []string {
	var offenders []string
	base := filepath.Join(root, filepath.FromSlash(modelWireRoot))
	err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		name := d.Name()
		if strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, modelWireAdapter) {
			return nil
		}
		rel := filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))
		if strings.HasPrefix(rel, modelWireOwner) || strings.HasPrefix(rel, modelWireRoot+"/foundation/integration/") {
			return nil
		}
		if !importsModelWire(path) {
			return nil
		}
		offenders = append(offenders, rel)
		return nil
	})
	if err != nil {
		return []string{fmt.Sprintf("model-wire-boundary: %v", err)}
	}
	if len(offenders) == 0 {
		return nil
	}
	sort.Strings(offenders)
	return []string{fmt.Sprintf(
		"model-wire-boundary: %s the model service's wire types directly: %s. A domain package "+
			"states what it wants in its own words and one %s file translates; the composition root "+
			"under %s builds the client. Put the types the domain needs beside the port and map them "+
			"in the adapter, or this becomes five packages that all have to change when the model "+
			"service is replaced",
		plural(len(offenders), "file speaks", "files speak"), strings.Join(offenders, ", "), modelWireAdapter, modelWireOwner)}
}

func importsModelWire(path string) bool {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		return false
	}
	for _, spec := range file.Imports {
		if strings.Trim(spec.Path.Value, `"`) == modelWirePackage {
			return true
		}
	}
	return false
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
