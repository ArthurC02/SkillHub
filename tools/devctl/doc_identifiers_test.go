package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDocScope lays out one live document plus the code files the scan reads
// declarations out of.
func writeDocScope(t *testing.T, doc string, code map[string]string) string {
	t.Helper()
	root := t.TempDir()
	write := func(relative, contents string) {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("AGENTS.md", doc)
	for relative, contents := range code {
		write(relative, contents)
	}
	return root
}

func TestDocIdentifierAcceptsNamesThatStillExist(t *testing.T) {
	t.Parallel()
	root := writeDocScope(t,
		"配額由 `GenerateQuotaFor` 決定，`TestDatasetUploadEnforces` 證明它。\n",
		map[string]string{
			"apps/platform/internal/policy/quota.go":      "package policy\n\nfunc GenerateQuotaFor(id string) int { return 0 }\n",
			"apps/platform/internal/policy/quota_test.go": "package policy\n\nfunc TestDatasetUploadEnforces(t *T) {}\n",
		})
	if problems := docIdentifierProblems(root); len(problems) != 0 {
		t.Fatalf("identifiers that exist were reported: %v", problems)
	}
}

func TestDocIdentifierRejectsAClaimWearingADeletedName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		doc  string
		code map[string]string
		want string
	}{{
		// The drift adversarial review kept finding by hand: the document still
		// argues from a function that was deleted.
		name: "a function the document names is gone",
		doc:  "配額由 `GenerateQuotaFor` 決定。\n",
		code: map[string]string{"apps/platform/internal/policy/quota.go": "package policy\n"},
		want: "GenerateQuotaFor is named in AGENTS.md but declared in no file",
	}, {
		// A test cited after it was merged away.
		name: "a test the document cites is gone",
		doc:  "`TestPackageDownloadHashes` 證明雙雜湊。\n",
		code: map[string]string{"apps/platform/internal/packaging/pack.go": "package packaging\n"},
		want: "TestPackageDownloadHashes is named in AGENTS.md",
	}, {
		// A type name that never existed in any file, argued from as if it did.
		name: "a type nobody ever declared",
		doc:  "搜尋回傳 `PublicSearchHit`。\n",
		code: map[string]string{"apps/platform/internal/catalog/search.go": "package catalog\n\ntype SearchHit struct{}\n"},
		want: "PublicSearchHit is named in AGENTS.md",
	}, {
		// Prose in backticks is not an identifier, and treating it as one is how
		// this becomes a check nobody believes: lower-case words are out, and the
		// ADR status vocabulary is on the named ledger.
		name: "prose and ledgered words are not identifiers",
		doc:  "ADR 標 `Superseded` 後不再引用；`run` 與 `eval` 是 context 名。\n",
		code: map[string]string{"apps/platform/internal/run/run.go": "package run\n"},
		want: "",
	}, {
		// The scan reads declarations from code files only. A name that lives
		// only in another document is not declared.
		name: "a name that exists only in prose elsewhere",
		doc:  "見 `ApplyPreview` 的行為。\n",
		code: map[string]string{"docs/plans/03-work-items.md": "`ApplyPreview` 的行為如下。\n"},
		want: "ApplyPreview is named in AGENTS.md",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			problems := docIdentifierProblems(writeDocScope(t, tc.doc, tc.code))
			if tc.want == "" {
				if len(problems) != 0 {
					t.Fatalf("expected no problems, got %v", problems)
				}
				return
			}
			if len(problems) != 1 {
				t.Fatalf("expected exactly one problem containing %q, got %v", tc.want, problems)
			}
			if !strings.Contains(problems[0], tc.want) {
				t.Fatalf("problem %q does not mention %q", problems[0], tc.want)
			}
		})
	}
}

// This file's own rationale comment names the identifiers it exists to catch, so
// a scan that counted any word in any code file would permanently whitelist its
// own examples. The exclusion is load-bearing and nothing else asserted it.
func TestDocIdentifierDoesNotDeclareItsOwnExamples(t *testing.T) {
	t.Parallel()
	root := writeDocScope(t, "見 `GenerateQuotaFor`。\n", nil)
	source, err := os.ReadFile("doc_identifiers.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "GenerateQuotaFor") {
		t.Skip("the rationale comment no longer names its examples; nothing to exclude")
	}
	copied := filepath.Join(root, "tools", "devctl", "doc_identifiers.go")
	if err := os.MkdirAll(filepath.Dir(copied), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(copied, source, 0o600); err != nil {
		t.Fatal(err)
	}
	if problems := docIdentifierProblems(root); len(problems) != 1 {
		t.Fatalf("doc_identifiers.go whitelisted its own examples: %v", problems)
	}
}

// The allowedDocWords ledger is a drift list, not an extension point: every
// entry carries the reason it is prose. An empty reason is an entry nobody has
// to justify, which is how a ledger turns into a place to make failures go away.
func TestAllowedDocWordsEachCarryAReason(t *testing.T) {
	t.Parallel()
	for word, reason := range allowedDocWords {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("allowedDocWords[%q] has no reason", word)
		}
	}
}
