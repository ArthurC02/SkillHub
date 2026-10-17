package operations

import (
	"context"
	"strings"
	"testing"
)

func TestAnAgentMayNotProposeARegisteredActionItsDefinitionDoesNotList(t *testing.T) {
	previewed := map[string]bool{}
	registered := func(name string) Action {
		return Action{Name: name, Preview: func(context.Context) (Preview, error) {
			previewed[name] = true
			return Preview{Counts: []PreviewCount{{Key: "rows", Count: 3}}}, nil
		}}
	}
	def := Definition{Actions: []string{"run-listed"}}
	actions := []Action{registered("run-listed"), registered("run-unlisted")}

	listed, err := prepareProposals(context.Background(), def, actions, []ProposalRequest{{Action: "run-listed"}})
	if err != nil || len(listed.proposals) != 1 {
		t.Fatalf("a listed action: got %+v, %v; want one proposal", listed, err)
	}

	_, err = prepareProposals(context.Background(), def, actions, []ProposalRequest{{Action: "run-listed"}, {Action: "run-unlisted"}})
	if err == nil || !strings.Contains(err.Error(), "not allowed to propose") {
		t.Fatalf("a registered but unlisted action: err = %v, want the conclusion refused", err)
	}
	if previewed["run-unlisted"] {
		t.Error("the unlisted action was previewed before being refused")
	}
}
