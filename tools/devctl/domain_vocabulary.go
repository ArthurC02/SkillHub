package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type vocabularySource struct {
	label    string
	checkKey string
	read     func(root string) (map[string]bool, error)
}

type domainVocabulary struct {
	name    string
	sources []vocabularySource
	readers []vocabularySource
	absent  string
}

func goConstEnum(path, typeName string) vocabularySource {
	return vocabularySource{
		label: fmt.Sprintf("%s (%s constants)", path, typeName),
		read: func(root string) (map[string]bool, error) {
			byName, err := goConstStrings(filepath.Join(root, filepath.FromSlash(path)), typeName)
			if err != nil {
				return nil, err
			}
			values := map[string]bool{}
			for _, value := range byName {
				values[value] = true
			}
			return values, nil
		},
	}
}

func goListedConstEnum(listPath, listName, constPath, constType string) vocabularySource {
	return vocabularySource{
		label: fmt.Sprintf("%s (%s)", listPath, listName),
		read: func(root string) (map[string]bool, error) {
			names, err := goListedIdentifiers(filepath.Join(root, filepath.FromSlash(listPath)), listName)
			if err != nil {
				return nil, err
			}
			byName, err := goConstStrings(filepath.Join(root, filepath.FromSlash(constPath)), constType)
			if err != nil {
				return nil, err
			}
			values := map[string]bool{}
			for _, name := range names {
				value, declared := byName[name]
				if !declared {
					return nil, fmt.Errorf("%s: %s lists %s, which %s does not declare", listPath, listName, name, constPath)
				}
				values[value] = true
			}
			return values, nil
		},
	}
}

func sqlColumnCheck(table, column string) vocabularySource {
	key := table + "." + column
	return vocabularySource{
		label:    fmt.Sprintf("db/migrations (CHECK on %s)", key),
		checkKey: key,
		read: func(root string) (map[string]bool, error) {
			vocabularies, err := migrationVocabularies(root)
			if err != nil {
				return nil, err
			}
			values, declared := vocabularies[key]
			if !declared {
				return nil, fmt.Errorf("no migration leaves a CHECK listing the values of %s; the constraint moved or the column was dropped", key)
			}
			return values, nil
		},
	}
}

func postgresEnum(path, typeName string) vocabularySource {
	return vocabularySource{
		label: fmt.Sprintf("%s (enum %s)", path, typeName),
		read: func(root string) (map[string]bool, error) {
			return postgresEnumValues(filepath.Join(root, filepath.FromSlash(path)), typeName)
		},
	}
}

var domainVocabularies = []domainVocabulary{
	{
		name: "run status",
		sources: []vocabularySource{
			postgresEnum("db/migrations/0004_test_lab_and_runs.sql", "run_status"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "RunStatus"),
			goListedConstEnum(
				"apps/platform/internal/trial/execution/statemachine.go", "AllStatuses",
				"apps/platform/internal/foundation/persistence/db/gen/models.go", "RunStatus"),
		},
	},
	{
		name: "creation session state",
		sources: []vocabularySource{
			goConstEnum("apps/platform/internal/creator/creation/state.go", "State"),
			goListedConstEnum(
				"apps/platform/internal/creator/creation/state.go", "AllStates",
				"apps/platform/internal/creator/creation/state.go", "State"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "CreationSessionState"),
		},
		absent: "creation_sessions.state carries no CHECK; state.go is the only guard",
	},
	{
		name: "evaluation status",
		sources: []vocabularySource{
			sqlColumnCheck("evaluations", "status"),
			goConstEnum("apps/platform/internal/trial/improvement/status.go", "Status"),
			goListedConstEnum(
				"apps/platform/internal/trial/improvement/status.go", "AllStatuses",
				"apps/platform/internal/trial/improvement/status.go", "Status"),
		},
		readers: []vocabularySource{
			goConstEnum("apps/platform/internal/creator/creation/evaluation.go", "evaluationStatus"),
		},
	},
	{
		name: "evaluation overall",
		sources: []vocabularySource{
			sqlColumnCheck("evaluations", "overall"),
			goConstEnum("apps/platform/internal/trial/improvement/overall.go", "Overall"),
			goListedConstEnum(
				"apps/platform/internal/trial/improvement/overall.go", "AllOveralls",
				"apps/platform/internal/trial/improvement/overall.go", "Overall"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "EvaluationOverall"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "RunComparisonRunsItemEvaluationOverall"),
		},
		readers: []vocabularySource{
			goConstEnum("apps/platform/internal/creator/creation/evaluation.go", "evaluationOverall"),
		},
	},
	{
		name: "run attempt object grant state",
		sources: []vocabularySource{
			sqlColumnCheck("run_attempts", "object_grants_state"),
			goConstEnum("apps/platform/internal/trial/execution/grantstate.go", "ObjectGrantState"),
			goListedConstEnum(
				"apps/platform/internal/trial/execution/grantstate.go", "AllObjectGrantStates",
				"apps/platform/internal/trial/execution/grantstate.go", "ObjectGrantState"),
		},
	},
	{
		name: "cost event kind",
		sources: []vocabularySource{
			sqlColumnCheck("cost_events", "kind"),
			goListedConstEnum(
				"apps/platform/internal/creator/credit/store.go", "AllCostEventKinds",
				"apps/platform/internal/creator/credit/store.go", "CostKind"),
		},
	},
	{
		name: "cost statistic kind",
		sources: []vocabularySource{
			sqlColumnCheck("cost_statistics", "kind"),
			goConstEnum("apps/platform/internal/creator/credit/store.go", "CostKind"),
			goListedConstEnum(
				"apps/platform/internal/creator/credit/store.go", "AllStatisticKinds",
				"apps/platform/internal/creator/credit/store.go", "CostKind"),
		},
	},
	{
		name: "credit entry kind",
		sources: []vocabularySource{
			sqlColumnCheck("credit_entries", "kind"),
			goConstEnum("apps/platform/internal/creator/credit/store.go", "EntryKind"),
			goListedConstEnum(
				"apps/platform/internal/creator/credit/store.go", "AllEntryKinds",
				"apps/platform/internal/creator/credit/store.go", "EntryKind"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "CreditLedgerEntryKind"),
		},
	},
	{
		name: "model call cost source",
		sources: []vocabularySource{
			sqlColumnCheck("cost_events", "cost_source"),
			sqlColumnCheck("evaluations", "cost_source"),
			sqlColumnCheck("evaluation_model_usage", "cost_source"),
			goConstEnum("apps/platform/internal/creator/credit/store.go", "CostSource"),
			goListedConstEnum(
				"apps/platform/internal/creator/credit/store.go", "AllCostSources",
				"apps/platform/internal/creator/credit/store.go", "CostSource"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "TraceSummaryUsageCostSource"),
		},
		readers: []vocabularySource{
			goConstEnum("apps/platform/internal/foundation/integration/llmclient/client.go", "CostSource"),
		},
	},
	{
		name: "evaluation suggestion category",
		sources: []vocabularySource{
			sqlColumnCheck("evaluation_suggestions", "category"),
			goConstEnum("apps/platform/internal/trial/improvement/suggestion.go", "SuggestionCategory"),
			goListedConstEnum(
				"apps/platform/internal/trial/improvement/suggestion.go", "AllSuggestionCategories",
				"apps/platform/internal/trial/improvement/suggestion.go", "SuggestionCategory"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "ImprovementSuggestionCategory"),
		},
	},
	{
		name: "evaluation suggestion decision",
		sources: []vocabularySource{
			sqlColumnCheck("evaluation_suggestions", "decision"),
			goConstEnum("apps/platform/internal/trial/improvement/suggestion.go", "Decision"),
			goListedConstEnum(
				"apps/platform/internal/trial/improvement/suggestion.go", "AllDecisions",
				"apps/platform/internal/trial/improvement/suggestion.go", "Decision"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "ImprovementSuggestionDecision"),
		},
		readers: []vocabularySource{
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "DecideSuggestionReqDecision"),
		},
	},
	{
		name: "skill source type",
		sources: []vocabularySource{
			sqlColumnCheck("skill_sources", "source_type"),
			goConstEnum("apps/platform/internal/skill/admission/sources.go", "SourceType"),
			goListedConstEnum(
				"apps/platform/internal/skill/admission/sources.go", "AllSourceTypes",
				"apps/platform/internal/skill/admission/sources.go", "SourceType"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "SkillSourceType"),
		},
		readers: []vocabularySource{
			goConstEnum("apps/platform/internal/skill/discovery/trust.go", "sourceType"),
		},
	},
	{
		name: "skill version license source",
		sources: []vocabularySource{
			sqlColumnCheck("skill_versions", "license_source"),
			goConstEnum("apps/platform/internal/shared/skillpkg/skillpkg.go", "LicenseSource"),
			goListedConstEnum(
				"apps/platform/internal/shared/skillpkg/skillpkg.go", "AllLicenseSources",
				"apps/platform/internal/shared/skillpkg/skillpkg.go", "LicenseSource"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "SkillLicenseSource"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "SetSkillRedistributionReqLicenseSource"),
		},
	},
	{
		name: "skill redistribution",
		sources: []vocabularySource{
			sqlColumnCheck("skills", "redistribution"),
			goConstEnum("apps/platform/internal/skill/library/write.go", "Redistribution"),
			goListedConstEnum(
				"apps/platform/internal/skill/library/write.go", "AllRedistributions",
				"apps/platform/internal/skill/library/write.go", "Redistribution"),
			goConstEnum("apps/platform/internal/skill/discovery/trust.go", "Redistribution"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "SkillRedistribution"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "OwnSkillRedistribution"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "ForkSkillCreatedRedistribution"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "SetSkillRedistributionOKPreviousValue"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "SetSkillRedistributionOKRedistributionValue"),
		},
		readers: []vocabularySource{
			goConstEnum("apps/platform/internal/skill/delivery/packaging.go", "Redistribution"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "SetSkillRedistributionReqValue"),
		},
	},
	{
		name: "skill curation tier",
		sources: []vocabularySource{
			sqlColumnCheck("skills", "curation_tier"),
			goListedConstEnum(
				"apps/platform/internal/skill/discovery/tier.go", "AllCurationTiers",
				"apps/platform/internal/skill/discovery/tier.go", "Tier"),
			goConstEnum("apps/platform/internal/skill/library/curation.go", "CurationTier"),
			goListedConstEnum(
				"apps/platform/internal/skill/library/curation.go", "AllCurationTiers",
				"apps/platform/internal/skill/library/curation.go", "CurationTier"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "SetSkillCurationTierReqValue"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "BrowseCatalogTier"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "PublicSearchSkillsTier"),
		},
		readers: []vocabularySource{
			goConstEnum("apps/platform/internal/trial/execution/schedule.go", "curationTier"),
		},
	},
	{
		name: "search document enrichment status",
		sources: []vocabularySource{
			sqlColumnCheck("search_documents", "enrichment_status"),
			goConstEnum("apps/platform/internal/skill/discovery/enrichment.go", "EnrichmentStatus"),
			goListedConstEnum(
				"apps/platform/internal/skill/discovery/enrichment.go", "AllEnrichmentStatuses",
				"apps/platform/internal/skill/discovery/enrichment.go", "EnrichmentStatus"),
			goConstEnum("apps/platform/internal/skill/admission/enrich.go", "enrichmentStatus"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "SkillEnrichmentStatus"),
		},
	},
	{
		name: "artifact scan status",
		sources: []vocabularySource{
			sqlColumnCheck("artifacts", "scan_status"),
			goConstEnum("apps/platform/internal/skill/delivery/scan.go", "ScanStatus"),
			goListedConstEnum(
				"apps/platform/internal/skill/delivery/scan.go", "AllScanStatuses",
				"apps/platform/internal/skill/delivery/scan.go", "ScanStatus"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "DownloadArtifactStatus"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "CreateDownloadArtifactCreatedStatus"),
		},
	},
	{
		name: "publication status",
		sources: []vocabularySource{
			sqlColumnCheck("publications", "status"),
			goConstEnum("apps/platform/internal/skill/publishing/publication.go", "Status"),
			goListedConstEnum(
				"apps/platform/internal/skill/publishing/publication.go", "AllStatuses",
				"apps/platform/internal/skill/publishing/publication.go", "Status"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "PublicationStatus"),
		},
	},
	{
		name: "dispatch halt source",
		sources: []vocabularySource{
			sqlColumnCheck("dispatch_halts", "source"),
			goConstEnum("apps/platform/internal/trial/execution/halt.go", "HaltSource"),
			goListedConstEnum(
				"apps/platform/internal/trial/execution/halt.go", "AllHaltSources",
				"apps/platform/internal/trial/execution/halt.go", "HaltSource"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "GetDispatchStatusOKHaltsItemSource"),
		},
		readers: []vocabularySource{
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "DeclareDispatchHaltOKSource"),
		},
	},
	{
		name: "feedback report kind",
		sources: []vocabularySource{
			sqlColumnCheck("feedback_reports", "kind"),
			goConstEnum("apps/platform/internal/product/learning/feedback.go", "FeedbackKind"),
			goListedConstEnum(
				"apps/platform/internal/product/learning/feedback.go", "AllFeedbackKinds",
				"apps/platform/internal/product/learning/feedback.go", "FeedbackKind"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "SubmitFeedbackReqKind"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "DataRetentionPolicyFeedbackKindItem"),
		},
	},
	{
		name: "run failure class",
		sources: []vocabularySource{
			sqlColumnCheck("runs", "failure_class"),
			goConstEnum("apps/platform/internal/trial/execution/statemachine.go", "FailureClass"),
			goListedConstEnum(
				"apps/platform/internal/trial/execution/statemachine.go", "AllFailureClasses",
				"apps/platform/internal/trial/execution/statemachine.go", "FailureClass"),
		},
	},
	{
		name: "skill category",
		sources: []vocabularySource{
			sqlColumnCheck("skills", "category"),
			goConstEnum("apps/platform/internal/skill/library/category.go", "Category"),
			goListedConstEnum(
				"apps/platform/internal/skill/library/category.go", "AllCategories",
				"apps/platform/internal/skill/library/category.go", "Category"),
			goListedConstEnum(
				"apps/platform/internal/skill/discovery/category.go", "AllStoredCategories",
				"apps/platform/internal/skill/discovery/category.go", "Category"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "BrowseCatalogCategory"),
			goConstEnum("apps/platform/internal/entrypoint/api/gen/oas_schemas_gen.go", "PublicSearchSkillsCategory"),
		},
	},
	{
		name: "skill category source",
		sources: []vocabularySource{
			sqlColumnCheck("skills", "category_source"),
			goConstEnum("apps/platform/internal/skill/library/category.go", "CategorySource"),
			goListedConstEnum(
				"apps/platform/internal/skill/library/category.go", "AllCategorySources",
				"apps/platform/internal/skill/library/category.go", "CategorySource"),
		},
		readers: []vocabularySource{
			goConstEnum("apps/platform/internal/skill/discovery/category.go", "categorySource"),
		},
	},
	{
		name: "skill runtime compatibility capability",
		sources: []vocabularySource{
			sqlColumnCheck("skill_runtime_compatibility", "capability"),
			goConstEnum("apps/platform/internal/skill/library/compatibility.go", "Capability"),
			goListedConstEnum(
				"apps/platform/internal/skill/library/compatibility.go", "AllCapabilities",
				"apps/platform/internal/skill/library/compatibility.go", "Capability"),
		},
		readers: []vocabularySource{
			goConstEnum("apps/platform/internal/trial/improvement/deterministic.go", "capability"),
		},
	},
}

var unreconciledVocabularies = map[string]string{
	"artifacts.kind":                           "Go never reads or writes it; every query spells the kind itself (platform-ddd-convergence.md §5.1)",
	"analytics_events.event_name":              "Go writes it and never branches on it (platform-ddd-convergence.md §5.6)",
	"cost_events.ref_type":                     "Go writes it and never branches on it (platform-ddd-convergence.md §5.6)",
	"credit_entries.ref_type":                  "Go writes it and never branches on it (platform-ddd-convergence.md §5.6)",
	"creation_receipts.kind":                   "Go writes it and never branches on it (platform-ddd-convergence.md §5.6)",
	"evaluation_model_usage.operation":         "Go writes it and never branches on it (platform-ddd-convergence.md §5.6)",
	"object_reconcile_sightings.resource_kind": "Go writes it and never branches on it (platform-ddd-convergence.md §5.6)",
	"outbox_events.event_type":                 "the outbox package's own tests reconcile it against the migration and the event catalogue",
	"skill_runtime_compatibility.runtime":      "Go only displays it; nothing branches on its value (platform-ddd-convergence.md §5.6)",
}

func domainVocabularyProblems(root string) []string {
	problems := reconcileVocabularies(root, domainVocabularies)
	return append(problems, coverageProblems(root, domainVocabularies, unreconciledVocabularies)...)
}

func coverageProblems(root string, reconciled []domainVocabulary, unreconciled map[string]string) []string {
	vocabularies, err := migrationVocabularies(root)
	if err != nil {
		return []string{fmt.Sprintf("domain-vocabulary: %v", err)}
	}
	covered := map[string]bool{}
	for _, vocabulary := range reconciled {
		for _, source := range vocabulary.sources {
			if source.checkKey != "" {
				covered[source.checkKey] = true
			}
		}
	}
	var problems []string
	for _, key := range sortedKeys(vocabularies) {
		reason, declared := unreconciled[key]
		switch {
		case covered[key] && declared:
			problems = append(problems, fmt.Sprintf(
				"domain-vocabulary: %s is reconciled, so its entry in unreconciledVocabularies (%s) is stale",
				key, reason))
		case !covered[key] && !declared:
			problems = append(problems, fmt.Sprintf(
				"domain-vocabulary: %s is a closed vocabulary in db/migrations that nothing reconciles; reconcile it with its Go definition or say in unreconciledVocabularies why not",
				key))
		}
	}
	for _, key := range sortedKeys(unreconciled) {
		if _, stillClosed := vocabularies[key]; !stillClosed {
			problems = append(problems, fmt.Sprintf(
				"domain-vocabulary: unreconciledVocabularies names %s (%s), which no migration leaves as a closed vocabulary",
				key, unreconciled[key]))
		}
	}
	return problems
}

func reconcileVocabularies(root string, vocabularies []domainVocabulary) []string {
	var problems []string
	for _, vocabulary := range vocabularies {
		readings := map[string]map[string]bool{}
		union := map[string]bool{}
		failed := false
		for _, source := range vocabulary.sources {
			values, err := source.read(root)
			if err != nil {
				problems = append(problems, fmt.Sprintf("domain-vocabulary: %s: %v", vocabulary.name, err))
				failed = true
				continue
			}
			if len(values) == 0 {
				problems = append(problems, fmt.Sprintf(
					"domain-vocabulary: %s: %s declares no value; either the vocabulary moved or this check is now looking at the wrong place",
					vocabulary.name, source.label))
				failed = true
				continue
			}
			readings[source.label] = values
			for value := range values {
				union[value] = true
			}
		}
		if failed || len(readings) < 2 {
			continue
		}
		var values []string
		for value := range union {
			values = append(values, value)
		}
		sort.Strings(values)
		var labels []string
		for label := range readings {
			labels = append(labels, label)
		}
		sort.Strings(labels)
		for _, value := range values {
			for _, label := range labels {
				if readings[label][value] {
					continue
				}
				problems = append(problems, fmt.Sprintf(
					"domain-vocabulary: %s %q is missing from %s; a closed vocabulary declared in more than one place and reconciled in none is how the same concept ends up meaning two things",
					vocabulary.name, value, label))
			}
		}
		problems = append(problems, readerProblems(root, vocabulary, union)...)
	}
	return problems
}

func readerProblems(root string, vocabulary domainVocabulary, union map[string]bool) []string {
	var problems []string
	for _, reader := range vocabulary.readers {
		values, err := reader.read(root)
		if err != nil {
			problems = append(problems, fmt.Sprintf("domain-vocabulary: %s: %v", vocabulary.name, err))
			continue
		}
		if len(values) == 0 {
			problems = append(problems, fmt.Sprintf(
				"domain-vocabulary: %s: %s declares no value; either the vocabulary moved or this check is now looking at the wrong place",
				vocabulary.name, reader.label))
			continue
		}
		var named []string
		for value := range values {
			named = append(named, value)
		}
		sort.Strings(named)
		for _, value := range named {
			if !union[value] {
				problems = append(problems, fmt.Sprintf(
					"domain-vocabulary: %s: %s reads %q, which no source declares; a reader spelling a value nobody writes reads nothing",
					vocabulary.name, reader.label, value))
			}
		}
	}
	return problems
}

func goConstStrings(path, typeName string) (map[string]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, decl := range file.Decls {
		group, ok := decl.(*ast.GenDecl)
		if !ok || group.Tok != token.CONST {
			continue
		}
		for _, spec := range group.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			declared, ok := value.Type.(*ast.Ident)
			if !ok || declared.Name != typeName {
				continue
			}
			for i, name := range value.Names {
				if i >= len(value.Values) {
					continue
				}
				lit, ok := value.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				unquoted, err := strconv.Unquote(lit.Value)
				if err != nil {
					return nil, fmt.Errorf("%s: %s has an unreadable value: %w", path, name.Name, err)
				}
				values[name.Name] = unquoted
			}
		}
	}
	return values, nil
}

func goListedIdentifiers(path, listName string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, err
	}
	for _, decl := range file.Decls {
		switch declared := decl.(type) {
		case *ast.GenDecl:
			if declared.Tok != token.VAR {
				continue
			}
			for _, spec := range declared.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok || len(value.Names) != 1 || value.Names[0].Name != listName || len(value.Values) != 1 {
					continue
				}
				return compositeIdentifiers(path, listName, value.Values[0])
			}
		case *ast.FuncDecl:
			if declared.Recv != nil || declared.Name.Name != listName || declared.Body == nil || len(declared.Body.List) != 1 {
				continue
			}
			returned, ok := declared.Body.List[0].(*ast.ReturnStmt)
			if !ok || len(returned.Results) != 1 {
				continue
			}
			return compositeIdentifiers(path, listName, returned.Results[0])
		}
	}
	return nil, fmt.Errorf("%s declares no %s; either the list moved or this check is now looking at the wrong file", path, listName)
}

func compositeIdentifiers(path, listName string, expression ast.Expr) ([]string, error) {
	composite, ok := expression.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("%s: %s is not a list this check can read", path, listName)
	}
	var names []string
	for _, element := range composite.Elts {
		switch identifier := element.(type) {
		case *ast.Ident:
			names = append(names, identifier.Name)
		case *ast.SelectorExpr:
			names = append(names, identifier.Sel.Name)
		default:
			return nil, fmt.Errorf("%s: %s holds an entry this check cannot name", path, listName)
		}
	}
	return names, nil
}

var (
	migrationTablePattern  = regexp.MustCompile(`(?i)\b(?:CREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?|ALTER\s+TABLE(?:\s+IF\s+EXISTS)?(?:\s+ONLY)?)\s+(\w+)`)
	migrationCheckPattern  = regexp.MustCompile(`(?i)\bCHECK\s*\(\s*(?:(\w+)\s+IS\s+NULL\s+OR\s+)?(\w+)\s+IN\s*\(([^)]*)\)`)
	migrationDropPattern   = regexp.MustCompile(`(?i)\bDROP\s+COLUMN\s+(?:IF\s+EXISTS\s+)?(\w+)`)
	migrationRenamePattern = regexp.MustCompile(`(?i)\bRENAME\s+COLUMN\s+(\w+)\s+TO\s+(\w+)`)
)

type migrationStatement struct {
	at   int
	kind string
	args []string
}

func migrationVocabularies(root string) (map[string]map[string]bool, error) {
	paths, err := filepath.Glob(filepath.Join(root, "db", "migrations", "*.sql"))
	if err != nil {
		return nil, err
	}
	vocabularies := map[string]map[string]bool{}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		table := ""
		for _, statement := range migrationStatements(withoutSQLComments(string(raw))) {
			switch statement.kind {
			case "table":
				table = statement.args[0]
			case "check":
				nullableColumn, column, list := statement.args[0], statement.args[1], statement.args[2]
				if nullableColumn == "" || nullableColumn == column {
					vocabularies[table+"."+column] = quotedSQLValues(list)
				}
			case "drop":
				delete(vocabularies, table+"."+statement.args[0])
			case "rename":
				from, to := table+"."+statement.args[0], table+"."+statement.args[1]
				if values, declared := vocabularies[from]; declared {
					vocabularies[to] = values
					delete(vocabularies, from)
				}
			}
		}
	}
	return vocabularies, nil
}

func migrationStatements(sql string) []migrationStatement {
	var statements []migrationStatement
	for kind, pattern := range map[string]*regexp.Regexp{
		"table":  migrationTablePattern,
		"check":  migrationCheckPattern,
		"drop":   migrationDropPattern,
		"rename": migrationRenamePattern,
	} {
		for _, match := range pattern.FindAllStringSubmatchIndex(sql, -1) {
			var args []string
			for group := 2; group < len(match); group += 2 {
				if match[group] < 0 {
					args = append(args, "")
					continue
				}
				args = append(args, strings.ToLower(sql[match[group]:match[group+1]]))
			}
			statements = append(statements, migrationStatement{at: match[0], kind: kind, args: args})
		}
	}
	sort.Slice(statements, func(i, j int) bool { return statements[i].at < statements[j].at })
	return statements
}

func withoutSQLComments(sql string) string {
	var kept strings.Builder
	inString := false
	for i := 0; i < len(sql); i++ {
		if sql[i] == '\'' {
			inString = !inString
		}
		if !inString && strings.HasPrefix(sql[i:], "--") {
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			if i < len(sql) {
				kept.WriteByte('\n')
			}
			continue
		}
		kept.WriteByte(sql[i])
	}
	return kept.String()
}

func quotedSQLValues(list string) map[string]bool {
	values := map[string]bool{}
	for _, entry := range strings.Split(list, ",") {
		trimmed := strings.Trim(strings.TrimSpace(entry), "'")
		if trimmed != "" {
			values[trimmed] = true
		}
	}
	return values
}

func postgresEnumValues(path, typeName string) (map[string]bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pattern := regexp.MustCompile(fmt.Sprintf(`(?s)CREATE TYPE\s+%s\s+AS ENUM\s*\((.*?)\)\s*;`, regexp.QuoteMeta(typeName)))
	match := pattern.FindStringSubmatch(string(raw))
	if match == nil {
		return nil, fmt.Errorf("%s declares no enum %s; the type moved and this comparison has lost its subject", path, typeName)
	}
	return quotedSQLValues(match[1]), nil
}
