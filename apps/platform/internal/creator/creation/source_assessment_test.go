package creation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation/webfetch"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

func sourceValue[T any](value T) *T { return &value }

func sourcedDraft() (Snapshot, *StepResult) {
	p := Snapshot{ChallengeCases: pairedEvaluationCases(), Brief: "Compute grades", BriefConfirmed: true, Messages: []Message{{Role: "user", Content: "數學占 40%、自然占 60%。"}}}
	r := &StepResult{Outcome: outcomeDraft, Message: "已起草", Draft: &GeneratedSkill{
		Name: "weighted-grades", Body: "Read references/grading.md, then run scripts/grade.py.",
		Files: []GeneratedFile{{Path: "references/grading.md", Content: "Math 40%, science 60%."}},
	}, SourceAssessment: &SourceAssessment{Rules: []SourceRule{{
		Rule: "Course weights", SourceMessageIndex: sourceValue(0), Quote: sourceValue("數學占 40%、自然占 60%。"), Path: sourceValue("references/grading.md"),
	}}}}
	return p, r
}

func (s *Service) saveWithCheckedSources(ctx context.Context, ws identity.Workspace, p *Snapshot, c Command) (commandOutcome, error) {
	e := envelope{Snapshot: *p}
	if p.Draft != nil {
		e.SourceCheckedHash = sourceDraftHash(*p, p.Draft.Skill)
	}
	outcome, err := s.save(ctx, ws, &e, c)
	*p = e.Snapshot
	return outcome, err
}

func TestSourceAssessmentRequiresExactAuthorEvidenceAndPackagedRules(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Snapshot, *StepResult)
	}{
		{"missing assessment", func(_ *Snapshot, r *StepResult) { r.SourceAssessment = nil }},
		{"null inventory", func(_ *Snapshot, r *StepResult) { r.SourceAssessment.Rules = nil }},
		{"missing source index", func(_ *Snapshot, r *StepResult) { r.SourceAssessment.Rules[0].SourceMessageIndex = nil }},
		{"negative source index", func(_ *Snapshot, r *StepResult) { r.SourceAssessment.Rules[0].SourceMessageIndex = sourceValue(-1) }},
		{"index after last message", func(_ *Snapshot, r *StepResult) { r.SourceAssessment.Rules[0].SourceMessageIndex = sourceValue(1) }},
		{"model's own statement", func(p *Snapshot, _ *StepResult) { p.Messages[0].Role = "assistant" }},
		{"invented quote", func(_ *Snapshot, r *StepResult) {
			r.SourceAssessment.Rules[0].Quote = sourceValue("Math and science are equally weighted.")
		}},
		{"blank quote", func(_ *Snapshot, r *StepResult) { r.SourceAssessment.Rules[0].Quote = sourceValue(" ") }},
		{"missing quote", func(_ *Snapshot, r *StepResult) { r.SourceAssessment.Rules[0].Quote = nil }},
		{"missing file path", func(_ *Snapshot, r *StepResult) { r.SourceAssessment.Rules[0].Path = nil }},
		{"file outside source directories", func(_ *Snapshot, r *StepResult) { r.SourceAssessment.Rules[0].Path = sourceValue("scripts/grade.py") }},
		{"traversing file path", func(_ *Snapshot, r *StepResult) {
			r.SourceAssessment.Rules[0].Path = sourceValue("references/../assets/rules.json")
		}},
		{"file not shipped", func(_ *Snapshot, r *StepResult) { r.Draft.Files = nil }},
		{"blank file", func(_ *Snapshot, r *StepResult) { r.Draft.Files[0].Content = " " }},
		{"file not linked", func(_ *Snapshot, r *StepResult) { r.Draft.Body = "Compute grades." }},
		{"blank rule", func(_ *Snapshot, r *StepResult) { r.SourceAssessment.Rules[0].Rule = " " }},
		{"duplicate rule", func(_ *Snapshot, r *StepResult) {
			r.SourceAssessment.Rules = append(r.SourceAssessment.Rules, r.SourceAssessment.Rules[0])
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, r := sourcedDraft()
			tc.change(&p, r)
			r.Brief, r.SampleInput, r.AcceptanceCriteria = p.Brief, "untrusted replacement", []string{"untrusted criterion"}
			for _, tool := range []bool{false, true} {
				copy := *r
				if tool {
					copy.Outcome, copy.ToolIntent = outcomeToolIntent, &ToolIntent{Kind: validateDraftTool}
				}
				e := fundedSession(p)
				calls := 0
				state, queued, err := countingValidator(&calls, false).proposal(t.Context(), 2, &e, &copy, nil)
				if err != nil || queued || state != StateWaitingInput || calls != 0 || e.Snapshot.Draft != nil {
					t.Fatalf("unverifiable sources reached draft validation: state=%s queued=%v calls=%d draft=%v err=%v", state, queued, calls, e.Snapshot.Draft, err)
				}
				if e.Snapshot.Brief != p.Brief || e.Snapshot.SampleInput != p.SampleInput || !reflect.DeepEqual(e.Snapshot.AcceptanceCriteria, p.AcceptanceCriteria) || lastMessage(e.Snapshot) != "草稿還缺少可核對的規則來源；請補充規則或提供來源，確認後再起草。" {
					t.Fatalf("source refusal changed the confirmed requirements or lost its explanation: %+v", e.Snapshot)
				}
			}
		})
	}
}

func TestSourcedDraftRecordsItsCheckPrivatelyAndReusesOnlyIdenticalContent(t *testing.T) {
	p, r := sourcedDraft()
	e := fundedSession(p)
	calls := 0
	state, _, err := countingValidator(&calls, false).proposal(t.Context(), 2, &e, r, nil)
	if err != nil || state != StateDraftReady || calls != 1 || !sourcesChecked(e) {
		t.Fatalf("a sourced draft was refused: state=%s calls=%d checked=%v err=%v", state, calls, sourcesChecked(e), err)
	}
	encoded, err := json.Marshal(View{Snapshot: e.Snapshot})
	if err != nil || strings.Contains(string(encoded), "source_checked_hash") || strings.Contains(string(encoded), "source_message_index") {
		t.Fatalf("private source metadata leaked into the public view: %s, %v", encoded, err)
	}
	again := &StepResult{Outcome: outcomeToolIntent, ToolIntent: &ToolIntent{Kind: validateDraftTool}}
	screenReply(e, again, []string{validateDraftTool})
	if again.Reason != "" || again.Draft == nil || again.Outcome != outcomeToolIntent {
		t.Fatalf("the checked stored draft could not be revalidated: %+v", again)
	}
	changed := e.Snapshot.Draft.Skill
	changed.Body += " Use equal weights instead."
	replacement := &StepResult{Outcome: outcomeDraft, Draft: &changed}
	screenReply(e, replacement, nil)
	if replacement.Reason != reasonSourcesMissing || replacement.Draft != nil {
		t.Fatalf("a changed draft reused another draft's source check: %+v", replacement)
	}
	e.Snapshot.Brief = "another task"
	if sourcesChecked(e) {
		t.Fatal("a different confirmed requirement reused the old source check")
	}
}

func TestSourceAssessmentAsksForMissingRulesWithoutAcceptingTheDraft(t *testing.T) {
	p, r := sourcedDraft()
	r.SourceAssessment.Rules = []SourceRule{{Rule: "Course weights", Question: sourceValue("數學與自然各占多少比例？")}, {Rule: "Grade scale", Question: sourceValue("成績滿分是多少？")}}
	e := fundedSession(p)
	calls := 0
	state, queued, err := countingValidator(&calls, false).proposal(t.Context(), 2, &e, r, nil)
	if err != nil || queued || state != StateWaitingInput || calls != 0 || e.Snapshot.Draft != nil || e.SourceCheckedHash != "" || lastMessage(e.Snapshot) != "數學與自然各占多少比例？\n成績滿分是多少？" {
		t.Fatalf("the missing rules were not returned to the author: state=%s queued=%v calls=%d snapshot=%+v err=%v", state, queued, calls, e.Snapshot, err)
	}
}

func TestSourceAssessmentAcceptsOnlyTheTextOfAnApprovedSuccessfulFetch(t *testing.T) {
	for _, change := range []string{"none", "missing record", "blocked", "metadata quote", "changed hash", "changed size", "not JSON"} {
		t.Run(change, func(t *testing.T) {
			p, r := sourcedDraft()
			record := Fetch{URL: "https://example.com/grading", SHA256: strings.Repeat("a", 64), Bytes: 42, Status: "ok"}
			p.Fetches = []Fetch{record}
			p.Messages = []Message{{Role: "tool", Content: webfetch.Observation(record, "Math 40%, science 60%.")}}
			r.SourceAssessment.Rules[0].Quote = sourceValue("Math 40%, science 60%.")
			switch change {
			case "missing record":
				p.Fetches = nil
			case "blocked":
				p.Fetches[0].Status = "blocked"
			case "metadata quote":
				r.SourceAssessment.Rules[0].Quote = sourceValue("https://example.com/grading")
			case "changed hash":
				p.Fetches[0].SHA256 = strings.Repeat("b", 64)
			case "changed size":
				p.Fetches[0].Bytes++
			case "not JSON":
				p.Messages[0].Content = "Math 40%, science 60%."
			}
			questions, valid := sourceQuestions(p, r)
			if valid != (change == "none") || len(questions) != 0 {
				t.Fatalf("fetch %s: valid=%v questions=%v", change, valid, questions)
			}
		})
	}
}

func TestSourceAssessmentBoundsAndExplicitEmptyInventory(t *testing.T) {
	for _, count := range []int{0, 64, 65} {
		p, r := sourcedDraft()
		base := r.SourceAssessment.Rules[0]
		r.SourceAssessment.Rules = make([]SourceRule, count)
		for i := range count {
			rule := base
			rule.Rule = fmt.Sprintf("Rule %d", i)
			r.SourceAssessment.Rules[i] = rule
		}
		if _, valid := sourceQuestions(p, r); valid != (count <= 64) {
			t.Fatalf("%d source rules: valid=%v", count, valid)
		}
	}
}

func TestSourceAssessmentTextAndQuestionBounds(t *testing.T) {
	for _, size := range []int{2000, 2001} {
		for _, field := range []string{"rule", "quote", "question"} {
			t.Run(fmt.Sprintf("%s/%d", field, size), func(t *testing.T) {
				p, r := sourcedDraft()
				text := strings.Repeat("規", size)
				switch field {
				case "rule":
					r.SourceAssessment.Rules[0].Rule = text
				case "quote":
					p.Messages[0].Content = text
					r.SourceAssessment.Rules[0].Quote = &text
				case "question":
					r.SourceAssessment.Rules = []SourceRule{{Rule: "missing rule", Question: &text}}
				}
				_, valid := sourceQuestions(p, r)
				if valid != (size == 2000) {
					t.Fatalf("%s length %d: valid=%v", field, size, valid)
				}
			})
		}
	}
}

func TestSourceMissingRuleQuestionsRequireTextAndNoSimultaneousEvidence(t *testing.T) {
	for _, tc := range []struct {
		name string
		rule SourceRule
	}{
		{"blank", SourceRule{Rule: "weights", Question: sourceValue("")}},
		{"spaces", SourceRule{Rule: "weights", Question: sourceValue(" ")}},
		{"source index", SourceRule{Rule: "weights", Question: sourceValue("Which weights?"), SourceMessageIndex: sourceValue(0)}},
		{"quote", SourceRule{Rule: "weights", Question: sourceValue("Which weights?"), Quote: sourceValue("40%")}},
		{"path", SourceRule{Rule: "weights", Question: sourceValue("Which weights?"), Path: sourceValue("references/grading.md")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, r := sourcedDraft()
			r.SourceAssessment.Rules = []SourceRule{tc.rule}
			if _, valid := sourceQuestions(p, r); valid {
				t.Fatal("a missing-rule question needs text and cannot also claim evidence")
			}
		})
	}
}

func TestMissingSourcesKeepThePreviouslySavedDraftAndCandidate(t *testing.T) {
	p, r := sourcedDraft()
	prior := &Draft{ContentHash: "prior", Skill: GeneratedSkill{Name: "prior", Body: "previous checked body"}}
	candidate := &Candidate{SkillID: "retained", VersionID: "immutable"}
	p.Draft, p.Candidate = prior, candidate
	e := fundedSession(p)
	e.SourceCheckedHash = sourceDraftHash(p, prior.Skill)
	r.SourceAssessment = nil
	calls := 0
	state, queued, err := countingValidator(&calls, false).proposal(t.Context(), 2, &e, r, nil)
	if err != nil || queued || state != StateWaitingInput || calls != 0 || e.Snapshot.Draft != prior || e.Snapshot.Candidate != candidate {
		t.Fatalf("source refusal replaced prior work: state=%s calls=%d snapshot=%+v err=%v", state, calls, e.Snapshot, err)
	}
}

func TestSourceAssessmentCrossesTheHTTPAdapterWithoutLosingEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"outcome":"draft","message":"checked","source_assessment":{"rules":[{"rule":"weights","source_message_index":0,"quote":"Math 40%","path":"references/weights.md"}]}}`))
	}))
	t.Cleanup(server.Close)
	r, err := ModelOrNone(&llmclient.Client{BaseURL: server.URL}).CreationStep(t.Context(), StepRequest{GatewayKey: "fixture-key"})
	if err != nil {
		t.Fatal(err)
	}
	want := &SourceAssessment{Rules: []SourceRule{{Rule: "weights", SourceMessageIndex: sourceValue(0), Quote: sourceValue("Math 40%"), Path: sourceValue("references/weights.md")}}}
	if !reflect.DeepEqual(r.SourceAssessment, want) {
		t.Fatalf("the HTTP adapter lost source evidence: got=%+v want=%+v", r.SourceAssessment, want)
	}
}

func TestSourceFilePathAcceptsTheLimitAndRejectsOneMoreRune(t *testing.T) {
	for _, size := range []int{256, 257} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			p, r := sourcedDraft()
			name := "references/" + strings.Repeat("規", size-11)
			r.SourceAssessment.Rules[0].Path = &name
			r.Draft.Body = "Read " + name
			r.Draft.Files[0].Path = name
			if _, valid := sourceQuestions(p, r); valid != (size == 256) {
				t.Fatalf("source path length %d: valid=%v", size, valid)
			}
		})
	}
}

func TestLegacyDraftNeedsItsSourceCheckBeforeAnyNewMaterialization(t *testing.T) {
	for _, kind := range []string{commandMaterialize, commandFinalize, commandConfirmDuplicate} {
		t.Run(kind, func(t *testing.T) {
			p := saveableSnapshot()
			if kind == commandConfirmDuplicate {
				p.PendingAction, p.PendingMaterialize = PendingDuplicateAcknowledgement, commandMaterialize
			}
			e := envelope{Snapshot: p}
			checks := 0
			s := &Service{Materialize: materializer(), DuplicateCheck: func(context.Context, identity.Workspace, string) ([]Reference, float64, error) {
				checks++
				return nil, 0, nil
			}}
			outcome, err := s.save(t.Context(), identity.Workspace{}, &e, Command{Kind: kind, ContentHash: p.Draft.ContentHash})
			if err != nil || !outcome.queueStep || outcome.materialize != "" || checks != 0 || e.Snapshot.Draft != p.Draft || e.Snapshot.Candidate != nil || !strings.Contains(lastMessage(e.Snapshot), "規則來源") {
				t.Fatalf("legacy source check was bypassed: outcome=%+v duplicateChecks=%d snapshot=%+v err=%v", outcome, checks, e.Snapshot, err)
			}
			e.SourceCheckedHash = sourceDraftHash(e.Snapshot, e.Snapshot.Draft.Skill)
			outcome, err = s.save(t.Context(), identity.Workspace{}, &e, Command{Kind: commandMaterialize, ContentHash: p.Draft.ContentHash})
			wantChecks := 1
			if kind == commandConfirmDuplicate {
				wantChecks = 0
			}
			if err != nil || outcome.materialize != commandMaterialize || outcome.queueStep || checks != wantChecks {
				t.Fatalf("verified sources did not resume saving: outcome=%+v checks=%d err=%v", outcome, checks, err)
			}
		})
	}
}

func TestExistingImmutableCandidateDoesNotNeedASourceCheckToBeSelected(t *testing.T) {
	e := envelope{Snapshot: saveableSnapshot()}
	e.Snapshot.Candidate = &Candidate{SkillID: "existing", VersionID: "immutable"}
	outcome, err := (&Service{}).save(t.Context(), identity.Workspace{}, &e, Command{Kind: commandFinalize, ContentHash: e.Snapshot.Draft.ContentHash})
	if err != nil || outcome.state != StateSaved || outcome.queueStep || outcome.materialize != "" || e.Snapshot.Candidate.VersionID != "immutable" {
		t.Fatalf("existing candidate was regenerated: outcome=%+v err=%v", outcome, err)
	}
}

func TestOnlyTheConfirmedDiagramCanSupplyASourceWithoutAMessageIndex(t *testing.T) {
	for _, change := range []string{"none", "unconfirmed", "missing interpretation", "open uncertainty", "invented quote"} {
		t.Run(change, func(t *testing.T) {
			p, r := sourcedDraft()
			attachConfirmedDiagram(&p)
			p.DiagramInterpretation.Nodes = []string{"數學占 40%、自然占 60%。"}
			r.SourceAssessment.Rules[0].SourceMessageIndex = nil
			switch change {
			case "unconfirmed":
				p.DiagramConfirmed = false
			case "missing interpretation":
				p.DiagramInterpretation = nil
			case "open uncertainty":
				p.DiagramInterpretation.Uncertainties[0].Answer = ""
			case "invented quote":
				r.SourceAssessment.Rules[0].Quote = sourceValue("每科都占一半。")
			}
			_, valid := sourceQuestions(p, r)
			if valid != (change == "none") {
				t.Fatalf("diagram %s: valid=%v", change, valid)
			}
		})
	}
}

func TestNewAuthorInputOrFetchedDataInvalidatesTheSavedSourceCheck(t *testing.T) {
	for _, change := range []string{"author", "fetch"} {
		t.Run(change, func(t *testing.T) {
			p, r := sourcedDraft()
			p.Draft = &Draft{Skill: *r.Draft}
			e := envelope{Snapshot: p, SourceCheckedHash: sourceDraftHash(p, *r.Draft)}
			if !sourcesChecked(e) {
				t.Fatal("the positive control did not have a valid source check")
			}
			if change == "author" {
				e.Snapshot.Messages = append(e.Snapshot.Messages, Message{Role: "user", Content: "改成兩科各占一半。"})
			} else {
				e.Snapshot.Fetches = append(e.Snapshot.Fetches, Fetch{URL: "https://example.com/new-rules", Status: "ok", SHA256: "new", Bytes: 20})
			}
			if sourcesChecked(e) {
				t.Fatal("changed source inputs retained the previous draft's source check")
			}
		})
	}
}

func TestStructuredRuleDataCanBeShippedAsALinkedAsset(t *testing.T) {
	p, r := sourcedDraft()
	r.Draft.Body = "Read assets/weights.csv."
	r.Draft.Files = []GeneratedFile{{Path: "assets/weights.csv", Content: "math,0.4\nscience,0.6\n"}}
	r.SourceAssessment.Rules[0].Path = sourceValue("assets/weights.csv")
	if questions, valid := sourceQuestions(p, r); !valid || len(questions) != 0 {
		t.Fatalf("a sourced lookup table was refused: valid=%v questions=%v", valid, questions)
	}
}

func TestManyMissingRulesAskWholeQuestionsWithinTheMessageLimit(t *testing.T) {
	p, r := sourcedDraft()
	r.SourceAssessment.Rules = make([]SourceRule, 20)
	for i := range 20 {
		r.SourceAssessment.Rules[i] = SourceRule{Rule: fmt.Sprintf("Rule %d", i), Question: sourceValue(fmt.Sprintf("Question %d ", i) + strings.Repeat("規", 1980) + "？")}
	}
	e := fundedSession(p)
	calls := 0
	state, _, err := countingValidator(&calls, false).proposal(t.Context(), 2, &e, r, nil)
	message := lastMessage(e.Snapshot)
	if err != nil || state != StateWaitingInput || calls != 0 || !strings.HasPrefix(message, "Question 0 ") || !strings.HasSuffix(message, "？\n其餘缺少的規則會在這批問題回答後繼續核對。") || len([]rune(message)) > 20000 || e.Snapshot.Draft != nil {
		t.Fatalf("many missing rules produced an unusable clarification: state=%s calls=%d message length=%d err=%v", state, calls, len([]rune(message)), err)
	}
}

func TestAModelShapeErrorCannotBypassTheSourceGateOrReplaceRequirements(t *testing.T) {
	for _, changed := range []bool{false, true} {
		p, r := sourcedDraft()
		r.Reason, r.SourceAssessment = "diagram_incomplete", nil
		if changed {
			r.Brief, r.AcceptanceCriteria, r.SampleInput = "untrusted replacement", []string{"untrusted criterion"}, "untrusted sample"
		}
		e := fundedSession(p)
		calls := 0
		state, _, err := countingValidator(&calls, false).proposal(t.Context(), 2, &e, r, nil)
		if err != nil || state != StateWaitingInput || calls != 0 || e.Snapshot.Draft != nil || e.Snapshot.Brief != p.Brief || len(e.Snapshot.AcceptanceCriteria) != 0 || e.Snapshot.SampleInput != "" {
			t.Fatalf("a provider error bypassed the source gate or changed the requirements: state=%s calls=%d snapshot=%+v err=%v", state, calls, e.Snapshot, err)
		}
	}
}
