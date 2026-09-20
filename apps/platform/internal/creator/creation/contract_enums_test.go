package creation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"gopkg.in/yaml.v3"
)

func contractEnum(t *testing.T, schema, property string) []string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "..",
		"contracts", "openapi", "llm-internal.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the internal contract: %v", err)
	}
	var spec struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Enum []string `yaml:"enum"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parsing the internal contract: %v", err)
	}
	values := spec.Components.Schemas[schema].Properties[property].Enum
	if len(values) == 0 {
		t.Fatalf("%s.%s declares no values; this test would pass on an empty set", schema, property)
	}
	return values
}

func TestEveryReasonTheContractDeclaresHasASentenceForTheUser(t *testing.T) {
	for _, code := range contractEnum(t, "CreationStepResponse", "reason") {
		t.Run(code, func(t *testing.T) {
			sentence, err := reasonSentence(code)
			if err != nil {
				t.Fatalf("the service may send %q and this build has no sentence for it, so a "+
					"legitimate reply is refused as an invalid command: %v", code, err)
			}
			if sentence == "" {
				t.Errorf("%q maps to an empty sentence; the user would be shown nothing", code)
			}
		})
	}
}

func TestTheSentenceTableNamesNoReasonTheContractDropped(t *testing.T) {
	declared := map[string]bool{}
	for _, code := range contractEnum(t, "CreationStepResponse", "reason") {
		declared[code] = true
	}
	for code := range reasonSentences {
		if !declared[code] {
			t.Errorf("%q has a sentence but the contract no longer declares it; the sentence is "+
				"then unreachable and nothing says which code replaced it", code)
		}
	}
}

func TestEveryOutcomeTheContractDeclaresReachesABranch(t *testing.T) {
	for _, outcome := range contractEnum(t, "CreationStepResponse", "outcome") {
		t.Run(outcome, func(t *testing.T) {
			s := &Service{}
			e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Brief: "b", BriefConfirmed: true}}
			r := &StepResult{Message: "m", Outcome: outcome, Brief: "b"}
			_, _, err := s.proposal(context.Background(), identity.Workspace{}, 3, &e, r)
			if errors.Is(err, ErrUnknownOutcome) {
				t.Fatalf("the service may send %q and this build has no branch for it, so the "+
					"session ends on an invalid command instead of advancing", outcome)
			}
		})
	}
}

func TestEveryToolTheContractDeclaresHasSomethingToRun(t *testing.T) {
	for _, kind := range contractEnum(t, "CreationToolIntent", "kind") {
		t.Run(kind, func(t *testing.T) {
			s := &Service{}
			e := envelope{Limits: testLimitsForProposal(), Snapshot: Snapshot{Brief: "b", BriefConfirmed: true}}
			r := &StepResult{Message: "m", Outcome: "tool_intent", ToolIntent: &ToolIntent{Kind: kind, Query: "q"}}
			if s.toolFor(context.Background(), identity.Workspace{}, 3, &e, r) == nil {
				t.Fatalf("the service may ask for %q and this build has nothing to run for it, so the "+
					"request is refused as an invalid command", kind)
			}
		})
	}
}
