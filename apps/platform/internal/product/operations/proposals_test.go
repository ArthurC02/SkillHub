package operations

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

func TestTheProposalViewsAreTheContractsViews(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "contracts", "openapi", "public.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(`(?s)operationId: listPlatformAgentProposals\b.*?- name: view\n.*?enum: \[([^\]]*)\]`).FindSubmatch(body)
	if found == nil {
		t.Fatal("the contract's listPlatformAgentProposals declares no view enum")
	}
	want := strings.Split(string(found[1]), ", ")
	if !slices.Equal(proposalViews, want) {
		t.Errorf("proposalViews = %q, the contract declares %q", proposalViews, want)
	}
}
