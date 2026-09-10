package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	embeddingMigration = "db/migrations/0007_search.sql"

	embeddingInvariant = "embeddingDimensions"
)

// Matches "embedding  vector(1536)," — the column definition, not a cast
// or a comment mentioning the type.
var pgvectorColumn = regexp.MustCompile(`(?mi)^\s*\w+\s+vector\((\d+)\)`)

func embeddingDimsProblems(root string) []string {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(embeddingMigration)))
	if err != nil {
		return []string{fmt.Sprintf("embedding-dims: %v", err)}
	}
	matches := pgvectorColumn.FindAllStringSubmatch(string(data), -1)
	if len(matches) == 0 {
		return []string{fmt.Sprintf(
			"embedding-dims: %s declares no `vector(N)` column; either the search index moved or this "+
				"check is looking at the wrong migration. Either way nothing is comparing the column "+
				"width with what apps/llm validates", embeddingMigration)}
	}
	widths := map[string]bool{}
	for _, m := range matches {
		widths[m[1]] = true
	}
	if len(widths) > 1 {
		var listed []string
		for w := range widths {
			listed = append(listed, w)
		}
		sort.Strings(listed)
		return []string{fmt.Sprintf(
			"embedding-dims: %s declares vector columns of differing widths (%s); this check compares one "+
				"number against apps/llm and there are now several", embeddingMigration,
			strings.Join(listed, ", "))}
	}
	var width string
	for w := range widths {
		width = w
	}

	sites, scanned := sharedNumberScan(root)
	var problems []string
	for _, problem := range scanned {
		if strings.Contains(problem, embeddingInvariant) {
			problems = append(problems, problem)
		}
	}
	marked := sites[embeddingInvariant]
	if len(marked) == 0 {
		return append(problems, fmt.Sprintf(
			"embedding-dims: no line is marked `one-number: %s`. %s says the vector column is %s wide and "+
				"apps/llm validates every returned vector against a literal of its own; unmarked, nothing "+
				"compares them, and a mismatch degrades search to its FTS half without an error",
			embeddingInvariant, embeddingMigration, width))
	}
	for _, site := range marked {
		if site.value != width {
			problems = append(problems, fmt.Sprintf(
				"embedding-dims: %s:%d says %s = %s while %s declares vector(%s). A vector of the wrong "+
					"width fails the insert per document and leaves the search leg running on FTS alone "+
					"(discovery/http.go records this)",
				site.file, site.line, embeddingInvariant, site.value, embeddingMigration, width))
		}
	}
	sort.Strings(problems)
	return problems
}
