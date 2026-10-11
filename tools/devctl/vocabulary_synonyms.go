package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

const (
	vocabularyFile     = "docs/domain-memory/registry/vocabulary.json"
	publicContractFile = "contracts/openapi/public.yaml"
)

var yamlKeyPattern = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*):`)

type vocabularyTerm struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Contexts []string `json:"contexts"`
	Synonyms []string `json:"synonyms"`
	Status   string   `json:"status"`
}

type synonymUse struct {
	where   string
	synonym string
	names   []string
}

func vocabularySynonymProblems(root string) []string {
	terms, err := reviewedSynonymTerms(root)
	if err != nil {
		return []string{err.Error()}
	}
	identities, problems := architectureIdentities(root)
	if len(problems) > 0 {
		return problems
	}
	var baseline struct {
		SynonymUses []string `json:"synonym_uses"`
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(architectureBudgetFile)))
	if err == nil {
		err = json.Unmarshal(data, &baseline)
	}
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", architectureBudgetFile, err)}
	}
	names, err := contractKeyNames(filepath.Join(root, filepath.FromSlash(publicContractFile)))
	if err != nil {
		return []string{err.Error()}
	}
	uses := synonymUses(terms, names, nil)
	for _, term := range terms {
		declared, err := contextIdentifiers(root, term.Contexts, identities)
		if err != nil {
			return []string{err.Error()}
		}
		uses = synonymUses([]vocabularyTerm{term}, declared, uses)
	}
	return synonymProblems(uses, baseline.SynonymUses)
}

func reviewedSynonymTerms(root string) ([]vocabularyTerm, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(vocabularyFile)))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", vocabularyFile, err)
	}
	var doc struct {
		Terms []vocabularyTerm `json:"terms"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", vocabularyFile, err)
	}
	var out []vocabularyTerm
	for _, term := range doc.Terms {
		if term.Status == registryReviewed && len(term.Synonyms) > 0 {
			out = append(out, term)
		}
	}
	return out, nil
}

func contractKeyNames(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if m := yamlKeyPattern.FindStringSubmatch(line); m != nil {
			names[m[1]] = publicContractFile + " " + m[1]
		}
	}
	return names, nil
}

func contextIdentifiers(root string, contexts []string, identities map[string]packageIdentity) (map[string]string, error) {
	internal := filepath.Join(root, filepath.FromSlash(platformInternalDir))
	names := map[string]string{}
	for _, id := range contexts {
		identity, ok := identities[id]
		if !ok {
			continue
		}
		dir := filepath.Join(internal, filepath.FromSlash(strings.TrimSuffix(identity.Path, "/*")))
		if err := collectDeclaredNames(internal, dir, names); err != nil {
			return nil, err
		}
	}
	return names, nil
}

func collectDeclaredNames(internal, dir string, names map[string]string) error {
	return filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, err := filepath.Rel(internal, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := filepath.ToSlash(rel)
		if generatedPackage(pkg) {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		for _, name := range declaredNames(file) {
			names[name] = pkg + "." + name
		}
		return nil
	})
}

func declaredNames(file *ast.File) []string {
	var out []string
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.FuncDecl:
			out = append(out, n.Name.Name)
		case *ast.TypeSpec:
			out = append(out, n.Name.Name)
		case *ast.ValueSpec:
			for _, name := range n.Names {
				out = append(out, name.Name)
			}
		case *ast.Field:
			for _, name := range n.Names {
				out = append(out, name.Name)
			}
		}
		return true
	})
	return out
}

func synonymUses(terms []vocabularyTerm, names map[string]string, uses []synonymUse) []synonymUse {
	for name, where := range names {
		words := identifierWords(name)
		for _, term := range terms {
			for _, synonym := range term.Synonyms {
				if containsWords(words, identifierWords(synonym)) {
					uses = addSynonymUse(uses, where, synonym, term.Name)
				}
			}
		}
	}
	return uses
}

func addSynonymUse(uses []synonymUse, where, synonym, name string) []synonymUse {
	for i := range uses {
		if uses[i].where == where && uses[i].synonym == synonym {
			if !slices.Contains(uses[i].names, name) {
				uses[i].names = append(uses[i].names, name)
			}
			return uses
		}
	}
	return append(uses, synonymUse{where: where, synonym: synonym, names: []string{name}})
}

func synonymProblems(uses []synonymUse, baseline []string) []string {
	held := map[string]bool{}
	for _, key := range baseline {
		held[key] = true
	}
	found := map[string]synonymUse{}
	for _, use := range uses {
		found[use.where+" "+use.synonym] = use
	}
	var problems []string
	for _, key := range sortedKeys(found) {
		if held[key] {
			continue
		}
		use := found[key]
		slices.Sort(use.names)
		problems = append(problems, fmt.Sprintf(
			"%s says %q, a name the Registry's vocabulary asks to avoid; say %s instead",
			use.where, use.synonym, strings.Join(quoted(use.names), " or ")))
	}
	for _, key := range sortedKeys(held) {
		if _, present := found[key]; !present {
			problems = append(problems, fmt.Sprintf(
				"%s: %q no longer uses that synonym; remove it from synonym_uses", architectureBudgetFile, key))
		}
	}
	return problems
}

func quoted(values []string) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = fmt.Sprintf("%q", v)
	}
	return out
}

func identifierWords(name string) []string {
	var words []string
	var current []rune
	runes := []rune(name)
	flush := func() {
		if len(current) > 0 {
			words = append(words, strings.ToLower(string(current)))
			current = nil
		}
	}
	for i, r := range runes {
		switch {
		case !unicode.IsLetter(r) && !unicode.IsDigit(r):
			flush()
			continue
		case unicode.IsUpper(r) && i > 0 && (unicode.IsLower(runes[i-1]) || (i+1 < len(runes) && unicode.IsLower(runes[i+1]) && unicode.IsUpper(runes[i-1]))):
			flush()
		}
		current = append(current, r)
	}
	flush()
	return words
}

func containsWords(words, phrase []string) bool {
	if len(phrase) == 0 {
		return false
	}
	for i := 0; i+len(phrase) <= len(words); i++ {
		if slices.Equal(words[i:i+len(phrase)], phrase) {
			return true
		}
	}
	return false
}
