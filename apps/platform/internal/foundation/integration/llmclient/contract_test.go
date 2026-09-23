package llmclient

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var wireTypes = map[string]reflect.Type{
	"CreationDraftValidation":       reflect.TypeOf(CreationDraftValidation{}),
	"CreationDiagramDecomposition":  reflect.TypeOf(DiagramDecomposition{}),
	"CreationDiagramInterpretation": reflect.TypeOf(DiagramInterpretation{}),
	"CreationDiagramUncertainty":    reflect.TypeOf(DiagramUncertainty{}),
	"CreationMessage":               reflect.TypeOf(CreationMessage{}),
	"CreationStepRequest":           reflect.TypeOf(CreationStepRequest{}),
	"CreationStepResponse":          reflect.TypeOf(CreationStepResponse{}),
	"CreationToolIntent":            reflect.TypeOf(CreationToolIntent{}),
	"CriterionVerdict":              reflect.TypeOf(CriterionVerdict{}),
	"DatasetField":                  reflect.TypeOf(DatasetField{}),
	"DatasetOutline":                reflect.TypeOf(DatasetOutline{}),
	"EmbedRequest":                  reflect.TypeOf(EmbedRequest{}),
	"EmbedResponse":                 reflect.TypeOf(EmbedResponse{}),
	"EnrichCheck":                   reflect.TypeOf(EnrichCheck{}),
	"EnrichSkillRequest":            reflect.TypeOf(EnrichSkillRequest{}),
	"EnrichSkillResponse":           reflect.TypeOf(EnrichSkillResponse{}),
	"GatewayUsage":                  reflect.TypeOf(GatewayUsage{}),
	"GenerateDiagram":               reflect.TypeOf(GenerateDiagram{}),
	"GenerateReference":             reflect.TypeOf(GenerateReference{}),
	"GenerateSkillRequest":          reflect.TypeOf(GenerateSkillRequest{}),
	"GenerateSkillResponse":         reflect.TypeOf(GenerateSkillResponse{}),
	"GeneratedFile":                 reflect.TypeOf(GeneratedFile{}),
	"GeneratedSkill":                reflect.TypeOf(GeneratedSkill{}),
	"ImprovementProposal":           reflect.TypeOf(ImprovementProposal{}),
	"JudgeArtifact":                 reflect.TypeOf(JudgeArtifact{}),
	"JudgeCriterion":                reflect.TypeOf(JudgeCriterion{}),
	"JudgeEvidenceRef":              reflect.TypeOf(JudgeEvidenceRef{}),
	"JudgeRunRequest":               reflect.TypeOf(JudgeRunRequest{}),
	"JudgeRunResponse":              reflect.TypeOf(JudgeRunResponse{}),
	"JudgeSkill":                    reflect.TypeOf(JudgeSkill{}),
	"JudgeVerdict":                  reflect.TypeOf(JudgeVerdict{}),
	"MatchReason":                   reflect.TypeOf(MatchReason{}),
	"MatchReasonsRequest":           reflect.TypeOf(MatchReasonsRequest{}),
	"MatchReasonsResponse":          reflect.TypeOf(MatchReasonsResponse{}),
	"Rubric":                        reflect.TypeOf(Rubric{}),
	"RubricItem":                    reflect.TypeOf(RubricItem{}),
	"SkillCandidate":                reflect.TypeOf(SkillCandidate{}),
	"SkillTags":                     reflect.TypeOf(SkillTags{}),
	"SuggestCriteriaRequest":        reflect.TypeOf(SuggestCriteriaRequest{}),
	"SuggestCriteriaResponse":       reflect.TypeOf(SuggestCriteriaResponse{}),
	"SuggestImprovementsRequest":    reflect.TypeOf(SuggestImprovementsRequest{}),
	"SuggestImprovementsResponse":   reflect.TypeOf(SuggestImprovementsResponse{}),
	"SuggestedCriterion":            reflect.TypeOf(SuggestedCriterion{}),
	"TargetFile":                    reflect.TypeOf(TargetFile{}),
	"TaskExample":                   reflect.TypeOf(TaskExample{}),
	"TraceDigest":                   reflect.TypeOf(TraceDigest{}),
	"TraceDigestEntry":              reflect.TypeOf(TraceDigestEntry{}),
}

var notModelledInGo = map[string]string{
	"Error":  "an error body is read as text, never decoded into fields",
	"Health": "the liveness probe's body is not read at all",
}

const samplingIsReportedButNotStored = "reported so a caller could record which sampling was asked " +
	"for; nothing stores it, because a verdict names its ruler with the judge model, the prompt " +
	"version and the rubric version, and the tiers this is reported by drop the parameter anyway"

var ignoredResponseFields = map[string]string{
	"EnrichSkillResponse.temperature":         samplingIsReportedButNotStored,
	"EnrichSkillResponse.seed":                samplingIsReportedButNotStored,
	"GenerateSkillResponse.temperature":       samplingIsReportedButNotStored,
	"GenerateSkillResponse.seed":              samplingIsReportedButNotStored,
	"JudgeRunResponse.temperature":            samplingIsReportedButNotStored,
	"JudgeRunResponse.seed":                   samplingIsReportedButNotStored,
	"SuggestImprovementsResponse.temperature": samplingIsReportedButNotStored,
	"SuggestImprovementsResponse.seed":        samplingIsReportedButNotStored,
}

type contractSchema struct {
	Properties map[string]yaml.Node `yaml:"properties"`
	Required   []string             `yaml:"required"`
}

type contractSpec struct {
	Components struct {
		Schemas map[string]contractSchema `yaml:"schemas"`
	} `yaml:"components"`
}

func readContract(t *testing.T) (contractSpec, map[string]any) {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "..", "..",
		"contracts", "openapi", "llm-internal.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the internal contract: %v", err)
	}
	var typed contractSpec
	if err := yaml.Unmarshal(raw, &typed); err != nil {
		t.Fatalf("parsing the internal contract: %v", err)
	}
	var tree map[string]any
	if err := yaml.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("parsing the internal contract: %v", err)
	}
	return typed, tree
}

func collectRefs(node any, out map[string]bool) {
	switch n := node.(type) {
	case map[string]any:
		if ref, ok := n["$ref"].(string); ok {
			if name, found := strings.CutPrefix(ref, "#/components/schemas/"); found {
				out[name] = true
			}
		}
		for _, v := range n {
			collectRefs(v, out)
		}
	case []any:
		for _, v := range n {
			collectRefs(v, out)
		}
	}
}

// schemasUnder walks the operations and returns every named schema reachable
// from the `side` half of them, following refs through other schemas.
func schemasUnder(tree map[string]any, side string) map[string]bool {
	seed := map[string]bool{}
	paths, _ := tree["paths"].(map[string]any)
	for _, methods := range paths {
		byMethod, ok := methods.(map[string]any)
		if !ok {
			continue
		}
		for _, op := range byMethod {
			operation, ok := op.(map[string]any)
			if !ok {
				continue
			}
			collectRefs(operation[side], seed)
		}
	}

	components, _ := tree["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	reached := map[string]bool{}
	pending := make([]string, 0, len(seed))
	for name := range seed {
		pending = append(pending, name)
	}
	for len(pending) > 0 {
		name := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if reached[name] {
			continue
		}
		reached[name] = true
		next := map[string]bool{}
		collectRefs(schemas[name], next)
		for n := range next {
			pending = append(pending, n)
		}
	}
	return reached
}

// wireFields maps each field's name on the wire to whether it may be omitted.
// A field tagged `-` is not on the wire at all.
func wireFields(t reflect.Type) map[string]bool {
	fields := map[string]bool{}
	for i := range t.NumField() {
		tag, ok := t.Field(i).Tag.Lookup("json")
		if !ok {
			continue
		}
		parts := strings.Split(tag, ",")
		if parts[0] == "" || parts[0] == "-" {
			continue
		}
		fields[parts[0]] = slicesContains(parts[1:], "omitempty")
	}
	return fields
}

func slicesContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func reportSorted(t *testing.T, problems []string, lead string) {
	t.Helper()
	if len(problems) == 0 {
		return
	}
	sort.Strings(problems)
	t.Errorf("%s\n  %s", lead, strings.Join(problems, "\n  "))
}

func TestEverySchemaTheServiceExchangesHasATypeOnThisSide(t *testing.T) {
	spec, tree := readContract(t)
	exchanged := schemasUnder(tree, "requestBody")
	for name := range schemasUnder(tree, "responses") {
		exchanged[name] = true
	}

	var problems []string
	for name := range exchanged {
		if _, modelled := wireTypes[name]; modelled {
			continue
		}
		if _, declared := notModelledInGo[name]; declared {
			continue
		}
		problems = append(problems, name+": the contract exchanges it and nothing here decodes it; "+
			"add it to wireTypes, or to notModelledInGo with the reason it is never read")
	}
	for name := range wireTypes {
		if _, inContract := spec.Components.Schemas[name]; !inContract {
			problems = append(problems, name+": named in wireTypes but the contract has no such "+
				"schema; a renamed schema leaves this type comparing against nothing")
		}
	}
	reportSorted(t, problems, "the roster and the contract disagree about what exists:")
}

func TestTheClientPutsNothingOnTheWireTheContractDoesNotDeclare(t *testing.T) {
	spec, _ := readContract(t)

	var problems []string
	for name, goType := range wireTypes {
		schema := spec.Components.Schemas[name]
		for field := range wireFields(goType) {
			if _, declared := schema.Properties[field]; !declared {
				problems = append(problems, name+"."+field+": no such property in the contract; "+
					"sent it is ignored, read it is always the zero value, and neither shows a symptom")
			}
		}
	}
	reportSorted(t, problems, "fields with no counterpart in the contract:")
}

func TestEveryRequiredPropertyIsAFieldThatAlwaysShips(t *testing.T) {
	spec, _ := readContract(t)

	var problems []string
	for name, goType := range wireTypes {
		fields := wireFields(goType)
		for _, required := range spec.Components.Schemas[name].Required {
			omitempty, present := fields[required]
			switch {
			case !present:
				problems = append(problems, name+"."+required+": the contract requires it and there "+
					"is no field for it, so it can never be sent and is discarded when received")
			case omitempty:
				problems = append(problems, name+"."+required+": the contract requires it but the "+
					"tag is omitempty, so a zero value leaves it out of the body entirely")
			}
		}
	}
	reportSorted(t, problems, "required properties this side cannot be trusted to carry:")
}

func TestEveryFieldTheClientIgnoresSaysWhyItIsIgnored(t *testing.T) {
	spec, tree := readContract(t)
	received := schemasUnder(tree, "responses")

	var problems []string
	claimed := map[string]bool{}
	for name, goType := range wireTypes {
		if !received[name] {
			continue
		}
		schema := spec.Components.Schemas[name]
		required := map[string]bool{}
		for _, r := range schema.Required {
			required[r] = true
		}
		fields := wireFields(goType)
		for property := range schema.Properties {
			if required[property] {
				continue
			}
			if _, read := fields[property]; read {
				continue
			}
			key := name + "." + property
			claimed[key] = true
			if _, declared := ignoredResponseFields[key]; !declared {
				problems = append(problems, key+": the service sends it and nothing here reads it; "+
					"add a field, or an entry to ignoredResponseFields saying why it is dropped")
			}
		}
	}
	for key := range ignoredResponseFields {
		if !claimed[key] {
			problems = append(problems, key+": listed as ignored but it is read now, or gone from "+
				"the contract; a stale entry silences a field nobody decided to drop")
		}
	}
	reportSorted(t, problems, "what this side drops on the floor:")
}
