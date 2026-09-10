package main

import (
	"strings"
	"testing"
)

func TestTheRealEmbeddingWidthAgrees(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := embeddingDimsProblems(root); len(problems) > 0 {
		t.Fatalf("%s", strings.Join(problems, "\n"))
	}

	sites, _ := sharedNumberScan(root)
	if len(sites[embeddingInvariant]) == 0 {
		t.Fatalf("no line is marked `one-number: %s`; the Python half of this comparison is gone",
			embeddingInvariant)
	}
}

func writeDimsFixture(t *testing.T, migration, python string) string {
	t.Helper()
	root := t.TempDir()
	writeAt(t, root, embeddingMigration, migration)
	writeAt(t, root, "apps/llm/src/skillhub_llm/app.py", python)
	return root
}

const goodMigration = "CREATE TABLE search_documents (\n    id         uuid PRIMARY KEY,\n" +
	"    embedding  vector(1536),\n    fts        tsvector\n);\n"

func TestEmbeddingDimsAcceptsAMatchingPair(t *testing.T) {
	t.Parallel()
	root := writeDimsFixture(t, goodMigration, "dims = 1536  # one-number: embeddingDimensions\n")
	if problems := embeddingDimsProblems(root); len(problems) != 0 {
		t.Fatalf("1536 against vector(1536) was rejected: %v", problems)
	}
}

func TestEmbeddingDimsNamesTheDrift(t *testing.T) {
	t.Parallel()
	root := writeDimsFixture(t, goodMigration, "dims = 3072  # one-number: embeddingDimensions\n")
	problems := embeddingDimsProblems(root)
	if len(problems) != 1 || !strings.Contains(problems[0], "embeddingDimensions = 3072 while") {
		t.Fatalf("want the drift named, got %v", problems)
	}
	if !strings.Contains(problems[0], "vector(1536)") {
		t.Fatalf("the problem must name the column width it disagrees with: %q", problems[0])
	}
}

func TestEmbeddingDimsSaysSoWhenItHasLostItsSubject(t *testing.T) {
	t.Parallel()
	t.Run("no vector column", func(t *testing.T) {
		t.Parallel()
		root := writeDimsFixture(t,
			"CREATE TABLE search_documents (id uuid PRIMARY KEY, fts tsvector);\n",
			"dims = 1536  # one-number: embeddingDimensions\n")
		problems := embeddingDimsProblems(root)
		if len(problems) != 1 || !strings.Contains(problems[0], "declares no `vector(N)` column") {
			t.Fatalf("a migration with no vector column was accepted: %v", problems)
		}
	})
	t.Run("two widths", func(t *testing.T) {
		t.Parallel()
		root := writeDimsFixture(t,
			goodMigration+"CREATE TABLE other (\n    v  vector(3072)\n);\n",
			"dims = 1536  # one-number: embeddingDimensions\n")
		problems := embeddingDimsProblems(root)
		if len(problems) != 1 || !strings.Contains(problems[0], "differing widths") {
			t.Fatalf("two column widths were accepted: %v", problems)
		}
	})
	t.Run("every marker fell off", func(t *testing.T) {
		t.Parallel()
		root := writeDimsFixture(t, goodMigration, "dims = 1536\n")
		problems := embeddingDimsProblems(root)
		if len(problems) == 0 || !strings.Contains(strings.Join(problems, "\n"),
			"no line is marked `one-number: embeddingDimensions`") {
			t.Fatalf("an unmarked tree was accepted: %v", problems)
		}
	})
	t.Run("a width mentioned only in a comment is not a column", func(t *testing.T) {
		t.Parallel()
		root := writeDimsFixture(t,
			"-- the embedding column used to be vector(3072)\n"+goodMigration,
			"dims = 1536  # one-number: embeddingDimensions\n")
		if problems := embeddingDimsProblems(root); len(problems) != 0 {
			t.Fatalf("a commented-out width voted: %v", problems)
		}
	})
}
