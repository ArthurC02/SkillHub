package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Identifiers a live document names must exist in the tree.
//
// The drift this catches is the one adversarial review kept finding by hand: a
// document asserting something about `GenerateQuotaFor` after the function was
// deleted, citing a test that was merged away, or arguing from a type name
// (`PublicSearchHit`, `ApplyPreview`) that never existed in any file. Prose
// cannot be checked, but the identifiers inside it can, and a stale identifier
// is usually a stale claim wearing it.
//
// SCOPE IS LIVE DOCUMENTS ONLY, and that limit is the whole design. The same
// check over paths in ADRs and frozen milestone reports produced 18 hits and
// every one of them was correct-when-written history — a DDD move, a deleted
// spike — which AGENTS.md explicitly says not to "helpfully fix". A check that
// asks people to break a stated rule is worse than no check: they learn to
// ignore it. So ADRs, mvp/mX/ reports and runbooks are out of scope, and stay
// out.
//
// Measured when written: 410 references, 6 flagged, 3 real. The false ones are
// prose artifacts (an elided list of test names sharing a prefix, ADR status
// vocabulary), which is what allowedDocWords is for — it is a drift ledger like
// db/query-owners.yaml's, not an extension point.
var docIdentifierScope = []string{
	"AGENTS.md",
	"docs/plans/01-goals-and-plan.md",
	"docs/plans/02-specifications-and-acceptance-criteria.md",
	"docs/plans/03-work-items.md",
	"docs/plans/04-backlog-and-handoffs.md",
	"docs/plans/05-pending-rulings.md",
	"docs/design/system.md",
	"docs/design/information-architecture.md",
	"docs/plans/mvp/m5/README.md",
}

// Backticked tokens that look like a declared name: an exported-style word, a
// Go test, or a Python test. Deliberately narrow — a lowercase English word in
// backticks is prose, and treating it as an identifier is how this becomes a
// check nobody believes.
var docIdentifierPattern = regexp.MustCompile("`(Test[A-Za-z0-9_]{3,}|test_[a-z0-9_]{3,}|[A-Z][A-Za-z0-9]{4,})`")

// Words the pattern matches that are not identifiers. Each needs a reason; the
// list may shrink without ceremony and should not grow without one.
var allowedDocWords = map[string]string{
	"Superseded":     "ADR status vocabulary (AGENTS.md), not a symbol",
	"Proposed":       "ADR status vocabulary",
	"Accepted":       "ADR status vocabulary",
	"FileCountLimit": "tail of an elided list: TestDatasetUploadEnforcesPerFileSizeLimit／FileCountLimit／TotalSizeLimit",
	"TotalSizeLimit": "tail of the same elided list",
}

var codeExtensions = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".py": true, ".sql": true, ".yaml": true, ".yml": true, ".json": true,
}

func docIdentifierProblems(root string) []string {
	declared := map[string]bool{}
	word := regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{3,}`)
	skip := map[string]bool{".git": true, "node_modules": true, ".venv": true, ".devctl": true, "dist": true, "__pycache__": true}

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skip[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !codeExtensions[strings.ToLower(filepath.Ext(path))] {
			return nil
		}
		// Not this file. Its rationale comment names the very identifiers it
		// exists to catch (GenerateQuotaFor, PublicSearchHit, ApplyPreview), and
		// a scan that counts any word in any code file would read those as
		// declarations and permanently whitelist its own examples.
		if filepath.Base(path) == "doc_identifiers.go" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, w := range word.FindAllString(string(body), -1) {
			declared[w] = true
		}
		return nil
	})

	missing := map[string][]string{}
	for _, rel := range docIdentifierScope {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue // a document that moved is the doc map's problem, not this check's
		}
		for _, m := range docIdentifierPattern.FindAllStringSubmatch(string(body), -1) {
			name := m[1]
			if declared[name] || allowedDocWords[name] != "" {
				continue
			}
			if !contains(missing[name], rel) {
				missing[name] = append(missing[name], rel)
			}
		}
	}

	names := make([]string, 0, len(missing))
	for n := range missing {
		names = append(names, n)
	}
	sort.Strings(names)

	var problems []string
	for _, n := range names {
		problems = append(problems, fmt.Sprintf(
			"doc-identifier: %s is named in %s but declared in no file. "+
				"Correct the document, or add it to allowedDocWords with the reason it is prose.",
			n, strings.Join(missing[n], ", ")))
	}
	return problems
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
