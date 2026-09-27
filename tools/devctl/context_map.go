package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	contextMapDoc          = "apps/platform/architecture-identity.yaml"
	dependencyPoliciesFile = "docs/domain-memory/registry/dependency-policies.json"
	registryContextsFile   = "docs/domain-memory/registry/contexts.json"
	identityListKey        = "packages:"

	identityHomes = registryContextsFile + " or " + contextMapDoc
)

var subdomainKinds = map[string]architectureKind{
	"core":       architectureCore,
	"supporting": architectureSupporting,
}

type architectureKind string

const (
	architectureCore         architectureKind = "Core"
	architectureSupporting   architectureKind = "Supporting"
	architectureSharedKernel architectureKind = "Shared Kernel"
	architectureGeneric      architectureKind = "Generic"
)

type packageIdentity struct {
	Product string
	Kind    architectureKind
	ID      string
	Path    string
	Source  string
}

var (
	boundaryIDPattern  = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	contextPathPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(?:/[a-z][a-z0-9_]*)*(?:/\*)?$`)

	depguardFilePattern     = regexp.MustCompile(`(?m)^\s*-\s*"\*\*/internal/([a-z][a-z0-9_]*(?:/[a-z][a-z0-9_]*)*)/\*\*"\s*$`)
	depguardSelectorPattern = regexp.MustCompile(`^\*\*/internal/([a-z][a-z0-9_]*(?:/[a-z][a-z0-9_]*)*)/\*\*$`)
)

func contextMapProblems(root string) []string {

	const mapPath, lintPath = contextMapDoc, "apps/platform/.golangci.yml"

	declared, problems := architectureIdentities(root)
	if len(declared) == 0 {
		return append(problems, fmt.Sprintf("%s: %s has no package rows", mapPath, identityListKey))
	}

	lint, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(lintPath)))
	if err != nil {
		return append(problems, fmt.Sprintf("%s: %v", lintPath, err))
	}
	guarded := map[string]bool{}

	for _, match := range depguardFilePattern.FindAllStringSubmatch(stripYAMLComments(string(lint)), -1) {
		guarded[match[1]] = true
	}

	internal := filepath.Join(root, "apps", "platform", "internal")
	present, err := goPackageDirs(internal)
	if err != nil {
		return append(problems, fmt.Sprintf("apps/platform/internal: %v", err))
	}
	for _, path := range sortedKeys(present) {
		if _, ok := resolveContextPath(path, declared); !ok {
			problems = append(problems, fmt.Sprintf(
				"apps/platform/internal/%s is not listed in %s; register it before adding the package (AGENTS.md 第 11 條)",
				path, identityHomes))
		}
	}
	for _, id := range sortedKeys(declared) {
		identity := declared[id]
		switch {
		case !selectorExists(identity.Path, present):
			problems = append(problems, fmt.Sprintf(
				"%s lists Boundary ID %q at internal/%s but no Go package directory exists there", identity.Source, id, identity.Path))
		case architectureNeedsDepguard(identity) && !guardCovers(identity.Path, guarded):
			problems = append(problems, fmt.Sprintf(
				"%s gives Boundary ID %q architecture kind %q but %s has no depguard rule covering internal/%s",
				identity.Source, id, identity.Kind, lintPath, identity.Path))
		}
	}
	for _, path := range sortedKeys(guarded) {
		if !guardedPathDeclared(path, declared) {
			problems = append(problems, fmt.Sprintf(
				"%s guards apps/platform/internal/%s but no Boundary ID in %s declares that path", lintPath, path, identityHomes))
		}
	}
	return problems
}

func architectureIdentities(root string) (map[string]packageIdentity, []string) {
	var problems []string
	var entries []identityEntry

	reviewed, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(registryContextsFile)))
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: %v", registryContextsFile, err)}
	}
	entries, problems = reviewedContextEntries(string(reviewed), problems)

	layout, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(contextMapDoc)))
	if err != nil {
		return nil, []string{fmt.Sprintf("%s: %v", contextMapDoc, err)}
	}
	technical, problems := identityEntries(string(layout), contextMapDoc, problems)

	declared := map[string]packageIdentity{}
	for _, entry := range append(entries, technical...) {
		relative := entry.source
		if problem := identityEntryShapeProblem(entry); problem != "" {
			problems = append(problems, problem)
			continue
		}
		architecture, context := architectureKind(entry.kind), entry.context
		id, currentPath := entry.id, entry.path
		if _, duplicate := declared[id]; duplicate {
			problems = append(problems, fmt.Sprintf("%s declares Boundary ID %q twice", relative, id))
			continue
		}
		identity := packageIdentity{Product: context, Kind: architecture, ID: id, Path: currentPath, Source: relative}
		for _, previous := range declared {
			if previous.Path == identity.Path {
				problems = append(problems, fmt.Sprintf("%s declares internal path %q twice (%s and %s)", relative, identity.Path, previous.ID, identity.ID))
				break
			}
			if selectorsOverlap(previous.Path, identity.Path) {
				problems = append(problems, fmt.Sprintf("%s internal paths %q (%s) and %q (%s) overlap", relative, previous.Path, previous.ID, identity.Path, identity.ID))
				break
			}
		}
		declared[id] = identity
	}
	return declared, problems
}

func identityEntryShapeProblem(entry identityEntry) string {
	relative := entry.source
	architecture := architectureKind(entry.kind)
	switch architecture {
	case architectureCore, architectureSupporting, architectureSharedKernel, architectureGeneric:
	default:
		return fmt.Sprintf("%s has unknown architecture kind %q", relative, entry.kind)
	}
	boundedContext := architecture == architectureCore || architecture == architectureSupporting
	switch {
	case relative == contextMapDoc && boundedContext:
		return fmt.Sprintf(
			"%s gives %q architecture kind %q; a Bounded Context is declared in %s",
			relative, entry.id, architecture, registryContextsFile)
	case boundedContext && entry.context == "":
		return fmt.Sprintf("%s kind %q requires a context name", relative, architecture)
	case !boundedContext && entry.context != "":
		return fmt.Sprintf("%s kind %q must not name a Bounded Context", relative, architecture)
	case !boundaryIDPattern.MatchString(entry.id):
		return fmt.Sprintf("%s has invalid Boundary ID %q", relative, entry.id)
	case !contextPathPattern.MatchString(entry.path):
		return fmt.Sprintf("%s has invalid internal path %q", relative, entry.path)
	}
	return ""
}

type identityEntry struct {
	id      string
	kind    string
	path    string
	context string
	source  string
}

func reviewedContextEntries(data string, problems []string) ([]identityEntry, []string) {
	var doc struct {
		Contexts []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Subdomain string `json:"subdomain"`
			Path      string `json:"implementation_path"`
			Status    string `json:"status"`
		} `json:"contexts"`
	}
	if err := json.Unmarshal([]byte(data), &doc); err != nil {
		return nil, append(problems, fmt.Sprintf("%s: %v", registryContextsFile, err))
	}
	var entries []identityEntry
	for _, context := range doc.Contexts {
		if context.Status != "reviewed" {
			problems = append(problems, fmt.Sprintf(
				"%s:%s is %q; only a reviewed Context carries an architecture identity",
				registryContextsFile, context.ID, context.Status))
			continue
		}
		kind, ok := subdomainKinds[context.Subdomain]
		if !ok {
			problems = append(problems, fmt.Sprintf(
				"%s:%s has subdomain %q; a Bounded Context is core or supporting",
				registryContextsFile, context.ID, context.Subdomain))
			continue
		}
		entries = append(entries, identityEntry{
			id: context.ID, kind: string(kind), path: context.Path,
			context: context.Name, source: registryContextsFile,
		})
	}
	if len(entries) == 0 {
		problems = append(problems, fmt.Sprintf("%s declares no reviewed Bounded Context", registryContextsFile))
	}
	return entries, problems
}

func identityEntries(data, relative string, problems []string) ([]identityEntry, []string) {
	var entries []identityEntry
	inPackages := false
	for number, line := range strings.Split(data, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") {
			inPackages = trimmed == identityListKey
			continue
		}
		if !inPackages {
			continue
		}
		key, value, found := strings.Cut(strings.TrimPrefix(trimmed, "- "), ":")
		if !found {
			problems = append(problems, fmt.Sprintf("%s:%d is not a key and value", relative, number+1))
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if strings.HasPrefix(trimmed, "- ") {
			if key != "id" {
				problems = append(problems, fmt.Sprintf("%s:%d starts an entry with %q; every entry starts with id", relative, number+1, key))
				continue
			}
			entries = append(entries, identityEntry{id: value, source: relative})
			continue
		}
		if len(entries) == 0 {
			problems = append(problems, fmt.Sprintf("%s:%d sets %q before any entry started", relative, number+1, key))
			continue
		}
		entry := &entries[len(entries)-1]
		switch key {
		case "kind":
			entry.kind = value
		case "path":
			entry.path = value
		case "context":
			entry.context = value
		default:
			problems = append(problems, fmt.Sprintf("%s:%d has unknown field %q", relative, number+1, key))
		}
	}
	return entries, problems
}

func architectureNeedsDepguard(identity packageIdentity) bool {
	if identity.Kind != architectureGeneric {
		return true
	}
	return identity.ID != "api" && !isCompositionRoot(identity.ID)
}

func knownBoundaryID(identities map[string]packageIdentity, id string) bool {
	_, ok := identities[id]
	return ok
}

func resolveContextPath(path string, identities map[string]packageIdentity) (packageIdentity, bool) {
	path = strings.Trim(filepath.ToSlash(path), "/")
	var match packageIdentity
	found, longest := false, -1
	for _, identity := range identities {
		selector := strings.TrimSuffix(identity.Path, "/*")
		if path != selector && !strings.HasPrefix(path, selector+"/") {
			continue
		}
		if strings.HasSuffix(identity.Path, "/*") && path == selector {
			continue
		}
		if len(selector) > longest {
			match, found, longest = identity, true, len(selector)
		}
	}
	return match, found
}

func goPackageDirs(internal string) (map[string]bool, error) {
	present := map[string]bool{}
	err := filepath.WalkDir(internal, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			return err
		}
		relative, err := filepath.Rel(internal, filepath.Dir(path))
		if err != nil {
			return err
		}
		present[filepath.ToSlash(relative)] = true
		return nil
	})
	return present, err
}

func selectorExists(selector string, present map[string]bool) bool {
	for path := range present {
		if _, ok := resolveContextPath(path, map[string]packageIdentity{"": {Path: selector}}); ok {
			return true
		}
	}
	return false
}

func guardCovers(selector string, guarded map[string]bool) bool {
	return guarded[strings.TrimSuffix(selector, "/*")]
}

func guardedPathDeclared(path string, identities map[string]packageIdentity) bool {
	for _, identity := range identities {
		if strings.TrimSuffix(identity.Path, "/*") == path {
			return true
		}
	}
	return false
}

func selectorsOverlap(a, b string) bool {
	a, b = strings.TrimSuffix(a, "/*"), strings.TrimSuffix(b, "/*")
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}
