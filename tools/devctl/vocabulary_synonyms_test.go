package main

import (
	"slices"
	"strings"
	"testing"
)

func TestIdentifierWordsSplitEveryNamingStyle(t *testing.T) {
	cases := []struct {
		name string
		want []string
	}{
		{"with_baseline", []string{"with", "baseline"}},
		{"WithoutSkill", []string{"without", "skill"}},
		{"runsWithoutSkill", []string{"runs", "without", "skill"}},
		{"HTTPBaseline", []string{"http", "baseline"}},
		{"skill-withheld", []string{"skill", "withheld"}},
		{"no skill", []string{"no", "skill"}},
	}
	for _, c := range cases {
		if got := identifierWords(c.name); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestASynonymMatchesOnlyWholeWordsInOrder(t *testing.T) {
	cases := []struct {
		name, synonym string
		want          bool
	}{
		{"baseline_better", "baseline", true},
		{"WithoutSkill", "without skill", true},
		{"skillWithout", "without skill", false},
		{"baselines", "baseline", false},
		{"rebaseline", "baseline", false},
		{"anything", "", false},
	}
	for _, c := range cases {
		if got := containsWords(identifierWords(c.name), identifierWords(c.synonym)); got != c.want {
			t.Errorf("%s ~ %q: %v, want %v", c.name, c.synonym, got, c.want)
		}
	}
}

func TestASynonymUseOutsideTheBaselineFailsAndAStaleEntryMustGo(t *testing.T) {
	terms := []vocabularyTerm{
		{Name: "Skill withheld", Synonyms: []string{"baseline", "without skill"}},
		{Name: "Reference batch", Synonyms: []string{"baseline"}},
	}
	names := map[string]string{
		"with_baseline":  "public.yaml with_baseline",
		"WithoutSkill":   "eval.WithoutSkill",
		"skill_withheld": "public.yaml skill_withheld",
	}
	uses := synonymUses(terms, names, nil)
	p := synonymProblems(uses, []string{"eval.WithoutSkill without skill", "eval.Gone baseline"})
	want := []string{
		`public.yaml with_baseline says "baseline", a name the Registry's vocabulary asks to avoid; say "Reference batch" or "Skill withheld" instead`,
		`"eval.Gone baseline" no longer uses that synonym`,
	}
	if len(p) != len(want) {
		t.Fatalf("problems: %v, want %d", p, len(want))
	}
	for i, w := range want {
		if !strings.Contains(p[i], w) {
			t.Errorf("problem %d: %q, want it to say %q", i, p[i], w)
		}
	}
}

func TestOnlyReviewedTermsWithSynonymsAreEnforced(t *testing.T) {
	root := t.TempDir()
	writeAt(t, root, vocabularyFile+".json", `{"terms": [
		{"id": "a", "name": "A", "synonyms": ["x"], "status": "reviewed"},
		{"id": "b", "name": "B", "synonyms": ["y"], "status": "candidate"},
		{"id": "c", "name": "C", "status": "reviewed"}
	]}`)
	terms, err := reviewedSynonymTerms(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(terms) != 1 || terms[0].ID != "a" {
		t.Errorf("terms: %+v, want only the reviewed one with synonyms", terms)
	}
}

func TestDeclaredNamesCoverEveryKindOfDeclaration(t *testing.T) {
	root := t.TempDir()
	writeAt(t, root, platformInternalDir+"/ctx/a/a.go", `package a

type Batch struct{ WithoutSkill int }

const Baseline = 1

var withBaseline = 2

func (b Batch) RunWithoutSkill(baselineID int) {}
`)
	writeAt(t, root, platformInternalDir+"/ctx/a/a_test.go", "package a\n\nconst TestOnlyBaseline = 1\n")
	identities := map[string]packageIdentity{"eval": {ID: "eval", Path: "ctx/a"}}
	names, err := contextIdentifiers(root, []string{"eval", "unknown"}, identities)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Batch", "WithoutSkill", "Baseline", "withBaseline", "RunWithoutSkill", "baselineID"} {
		if names[want] != "ctx/a."+want {
			t.Errorf("%s: %q, want ctx/a.%s", want, names[want], want)
		}
	}
	if _, ok := names["TestOnlyBaseline"]; ok {
		t.Error("a test file's declarations were scanned")
	}
}

func TestAContractKeyIsNamedWithoutItsLineSoTheBaselineSurvivesEdits(t *testing.T) {
	root := t.TempDir()
	writeAt(t, root, publicContractFile, "paths:\n  /a:\n    with_baseline: 1\nschemas:\n  B:\n    with_baseline: 2\n")
	names, err := contractKeyNames(root + "/" + publicContractFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := names["with_baseline"]; got != publicContractFile+" with_baseline" {
		t.Errorf("with_baseline: %q, want it named by file and key only", got)
	}
}

func TestTheRepositoryAvoidsTheNamesItsVocabularyRetired(t *testing.T) {
	for _, problem := range vocabularySynonymProblems("../..") {
		t.Error(problem)
	}
}
