package creation

import (
	"testing"
)

func TestADraftMustRestateEveryConfirmedInput(t *testing.T) {
	confirmedInputs := Snapshot{Brief: "b", BriefConfirmed: true, AcceptanceCriteria: []string{"c"}, SampleInput: "s"}
	restated := func() *StepResult {
		return &StepResult{Brief: "b", Draft: &GeneratedSkill{}}
	}
	if !draftFollowsConfirmation(confirmedInputs, restated()) {
		t.Fatal("a draft that leaves criteria and sample input empty restates them")
	}
	for name, change := range map[string]func(*StepResult){
		"other criteria":     func(r *StepResult) { r.AcceptanceCriteria = []string{"d"} },
		"other sample input": func(r *StepResult) { r.SampleInput = "t" },
		"no draft":           func(r *StepResult) { r.Draft = nil },
		"no brief":           func(r *StepResult) { r.Brief = "" },
	} {
		r := restated()
		change(r)
		if draftFollowsConfirmation(confirmedInputs, r) {
			t.Errorf("%s: accepted", name)
		}
	}
	same := restated()
	same.AcceptanceCriteria, same.SampleInput = []string{"c"}, "s"
	if !draftFollowsConfirmation(confirmedInputs, same) {
		t.Fatal("a draft that repeats the criteria and sample input was refused")
	}
}

func TestANewReadingOfTheDiagramDropsWhatWasBuiltOnTheOldOne(t *testing.T) {
	p := Snapshot{DiagramConfirmed: true, Draft: &Draft{}, Candidate: &Candidate{}, Duplicates: []Reference{{SkillID: "s"}}, PendingAction: "confirm_duplicate"}
	state := reinterpretDiagram(&p, "new reading")
	if state != StateWaitingConfirmation || p.DiagramUnderstanding != "new reading" || p.DiagramConfirmed || p.Draft != nil || p.Candidate != nil || p.Duplicates != nil || p.PendingAction != "confirm_diagram" {
		t.Fatalf("state = %s, snapshot = %+v", state, p)
	}
}

func TestSearchQueriesAreTheIntentThenUpToThreeDistinctRewrites(t *testing.T) {
	got := searchQueries(&ToolIntent{Query: " a ", Queries: []string{"b", " a", "", "c", "d", "e"}})
	if len(got) != 4 || got[0] != "a" || got[1] != "b" || got[2] != "c" || got[3] != "d" {
		t.Fatalf("queries = %q", got)
	}
}
