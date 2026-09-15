package eval

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func revisionIn(status Status, superseded bool) *Evaluation {
	return &Evaluation{row: gen.Evaluation{
		Status:       string(status),
		SupersededAt: pgtype.Timestamptz{Valid: superseded},
	}}
}

func assertEvents(t *testing.T, e *Evaluation, want ...Event) {
	t.Helper()
	if got := e.Events(); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %#v, want %#v", got, want)
	}
}

func TestAStartedEvaluationAwaitsTheJudgeAndDeclaresWhatItWillBeJudgedWith(t *testing.T) {
	model, rubric := "gpt-5.6-terra", "rubric-v1"
	declared := EvaluationStarted{JudgeModel: &model, RubricVersion: &rubric}

	e := startEvaluation(pgtype.UUID{}, pgtype.UUID{}, declared)

	if e.Status() != StatusPending || e.Superseded() {
		t.Fatalf("a new revision is %q (superseded %v), want pending and current", e.Status(), e.Superseded())
	}
	assertEvents(t, e, declared)
}

func TestEvaluationSnapshotsDoNotAliasInputsOrOutputs(t *testing.T) {
	t.Run("declared judge data and events keep independent pointers", func(t *testing.T) {
		model, prompt, rubric := "model", "prompt", "rubric"
		declared := EvaluationStarted{JudgeModel: &model, JudgePromptVersion: &prompt, RubricVersion: &rubric}
		e := startEvaluation(pgtype.UUID{}, pgtype.UUID{}, declared)

		model, prompt, rubric = "changed", "changed", "changed"
		if *e.row.JudgeModel != "model" || *e.row.JudgePromptVersion != "prompt" || *e.row.RubricVersion != "rubric" {
			t.Fatalf("evaluation after input mutation = %+v, want original judge declaration", e.row)
		}
		first := e.Events()[0].(EvaluationStarted)
		*first.JudgeModel, *first.JudgePromptVersion, *first.RubricVersion = "output", "output", "output"
		second := e.Events()[0].(EvaluationStarted)
		if *second.JudgeModel != "model" || *second.JudgePromptVersion != "prompt" || *second.RubricVersion != "rubric" {
			t.Fatalf("started event after output mutation = %+v, want original judge declaration", second)
		}
	})

	t.Run("completed and failed results keep nested evidence and cost copies", func(t *testing.T) {
		byteRange, charRange := &Range{Start: 1, End: 2}, &Range{Start: 3, End: 4}
		cost, usageCost := 1.5, 2.5
		v := verdict{
			results:  []CriterionResult{{CriterionID: "criterion-original", Evidence: []EvidenceRef{{Kind: "result-evidence", ByteRange: byteRange, CharRange: charRange}}}},
			findings: []Finding{{Message: "finding-original", Evidence: []EvidenceRef{{Kind: "finding-evidence", ByteRange: byteRange}}}},
			costUSD:  &cost, usage: &llmclient.GatewayUsage{CostUSD: &usageCost},
		}
		e := revisionIn(StatusPending, false)
		e.Complete(v)
		v.results[0].Evidence[0] = EvidenceRef{Kind: "result-evidence-changed"}
		v.results[0] = CriterionResult{CriterionID: "criterion-changed"}
		v.findings[0].Evidence[0] = EvidenceRef{Kind: "finding-evidence-changed"}
		v.findings[0] = Finding{Message: "finding-changed"}
		byteRange.Start, charRange.End, cost, usageCost = 9, 9, 9.5, 9.5
		if e.verdict.results[0].CriterionID != "criterion-original" || e.verdict.results[0].Evidence[0].Kind != "result-evidence" ||
			e.verdict.results[0].Evidence[0].ByteRange.Start != 1 || e.verdict.results[0].Evidence[0].CharRange.End != 4 ||
			e.verdict.findings[0].Message != "finding-original" || e.verdict.findings[0].Evidence[0].Kind != "finding-evidence" ||
			e.verdict.findings[0].Evidence[0].ByteRange.Start != 1 || *e.verdict.costUSD != 1.5 || *e.verdict.usage.CostUSD != 2.5 {
			t.Fatalf("completed verdict after input mutation = %+v, want original nested data", e.verdict)
		}

		failureRange := &Range{Start: 5, End: 6}
		f := failure{findings: []Finding{{Category: CategoryExecution, Message: "failure-finding-original", Evidence: []EvidenceRef{{Kind: "failure-evidence", ByteRange: failureRange}}}}}
		failed := revisionIn(StatusPending, false)
		failed.Fail(f)
		f.findings[0].Evidence[0] = EvidenceRef{Kind: "failure-evidence-changed"}
		f.findings[0] = Finding{Message: "failure-finding-changed"}
		failureRange.Start = 9
		if failed.failure.findings[0].Category != CategoryExecution || failed.failure.findings[0].Message != "failure-finding-original" ||
			failed.failure.findings[0].Evidence[0].Kind != "failure-evidence" || failed.failure.findings[0].Evidence[0].ByteRange.Start != 5 {
			t.Fatalf("failure after input mutation = %+v, want original nested evidence", failed.failure)
		}
	})

	t.Run("applied suggestion events keep independent ID slices and nil slices", func(t *testing.T) {
		firstID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
		secondID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
		versionID := pgtype.UUID{Bytes: [16]byte{9}, Valid: true}
		e := revisionIn(StatusCompleted, false)
		e.suggestions = map[pgtype.UUID]gen.EvaluationSuggestion{firstID: {ID: firstID}}
		e.RecordApplied(versionID, []pgtype.UUID{firstID})

		first := e.Events()[0].(SuggestionsApplied)
		first.SuggestionIDs[0] = secondID
		second := e.Events()[0].(SuggestionsApplied)
		if len(second.SuggestionIDs) != 1 || second.SuggestionIDs[0] != firstID {
			t.Fatalf("applied event after output mutation = %+v, want first suggestion", second)
		}

		if got := cloneEvidenceRefs(nil); got != nil {
			t.Fatalf("nil evidence refs = %#v, want nil", got)
		}
	})
}

func TestOnlyASettledCurrentRevisionCanBeSuperseded(t *testing.T) {
	cases := []struct {
		name       string
		status     Status
		superseded bool
		want       Event
	}{
		{"a pending revision is still awaiting its judge", StatusPending, false, Refused{RefusedAwaitingJudge}},
		{"a completed revision gives way", StatusCompleted, false, EvaluationSuperseded{}},
		{"a failed revision gives way", StatusFailed, false, EvaluationSuperseded{}},
		{"a superseded revision is not current any more", StatusCompleted, true, Refused{RefusedSuperseded}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := revisionIn(tc.status, tc.superseded)

			e.Supersede()

			assertEvents(t, e, tc.want)
			_, refused := tc.want.(Refused)
			if e.Superseded() != (tc.superseded || !refused) {
				t.Fatalf("superseded = %v after %T", e.Superseded(), tc.want)
			}
		})
	}
}

func TestOnlyAPendingRevisionSettles(t *testing.T) {
	v := aVerdict("the file is written", OverallMet)
	f := failure{summary: "the judge was unreachable", evidenceComplete: true}
	commands := []struct {
		name   string
		settle func(*Evaluation)
		to     Status
		event  Event
	}{
		{"complete", func(e *Evaluation) { e.Complete(v) }, StatusCompleted,
			EvaluationCompleted{Overall: OverallMet, EvidenceComplete: true}},
		{"fail", func(e *Evaluation) { e.Fail(f) }, StatusFailed, EvaluationFailed{EvidenceComplete: true}},
	}
	for _, command := range commands {
		t.Run(command.name+" a pending revision", func(t *testing.T) {
			e := revisionIn(StatusPending, false)

			command.settle(e)

			if e.Status() != command.to {
				t.Fatalf("status = %q, want %q", e.Status(), command.to)
			}
			assertEvents(t, e, command.event)
		})
		for _, settled := range []Status{StatusCompleted, StatusFailed} {
			t.Run(command.name+" a "+string(settled)+" revision", func(t *testing.T) {
				e := revisionIn(settled, false)

				command.settle(e)

				if e.Status() != settled {
					t.Fatalf("a settled revision became %q", e.Status())
				}
				assertEvents(t, e, Refused{RefusedSettled})
			})
		}
	}
}

func TestFeedbackLandsOnlyOnTheCurrentRevision(t *testing.T) {
	t.Run("an answer with a comment", func(t *testing.T) {
		e := revisionIn(StatusCompleted, false)
		e.RecordFeedback(true, "useful")
		assertEvents(t, e, FeedbackRecorded{Helpful: true, HasComment: true})
	})
	t.Run("an empty comment is no comment", func(t *testing.T) {
		e := revisionIn(StatusPending, false)
		e.RecordFeedback(false, "")
		assertEvents(t, e, FeedbackRecorded{Helpful: false, HasComment: false})
	})
	t.Run("a superseded revision takes no feedback", func(t *testing.T) {
		e := revisionIn(StatusCompleted, true)
		e.RecordFeedback(true, "too late")
		assertEvents(t, e, Refused{RefusedSuperseded})
	})
}

func TestADecisionMustBeAChoiceOnAKnownSuggestionAndAcceptingAnAppliedOneIsFinal(t *testing.T) {
	suggestionID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	stranger := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	cases := []struct {
		name    string
		applied bool
		target  pgtype.UUID
		to      Decision
		want    Event
		after   Decision
	}{
		{"accepting", false, suggestionID, DecisionAccepted,
			SuggestionDecided{suggestionID, DecisionAccepted}, DecisionAccepted},
		{"rejecting", false, suggestionID, DecisionRejected,
			SuggestionDecided{suggestionID, DecisionRejected}, DecisionRejected},
		{"going back to pending is not a choice", false, suggestionID, DecisionPending,
			Refused{RefusedNotAChoice}, DecisionPending},
		{"an empty decision is not a choice", false, suggestionID, "",
			Refused{RefusedNotAChoice}, DecisionPending},
		{"an undeclared word is not a choice", false, suggestionID, "maybe",
			Refused{RefusedNotAChoice}, DecisionPending},
		{"a suggestion this evaluation does not hold", false, stranger, DecisionAccepted,
			Refused{RefusedUnknownSuggestion}, DecisionPending},
		{"rejecting an applied suggestion", true, suggestionID, DecisionRejected,
			Refused{RefusedAcceptanceIsFinal}, DecisionAccepted},
		{"accepting an applied suggestion again", true, suggestionID, DecisionAccepted,
			SuggestionDecided{suggestionID, DecisionAccepted}, DecisionAccepted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			held := gen.EvaluationSuggestion{ID: suggestionID, Decision: string(DecisionPending)}
			if tc.applied {
				held.Decision = string(DecisionAccepted)
				held.AppliedSkillVersionID = pgtype.UUID{Bytes: [16]byte{9}, Valid: true}
			}
			e := revisionIn(StatusCompleted, true)
			e.suggestions = map[pgtype.UUID]gen.EvaluationSuggestion{suggestionID: held}

			e.Decide(tc.target, tc.to)

			assertEvents(t, e, tc.want)
			if got := e.Decision(suggestionID); got != tc.after {
				t.Fatalf("decision = %q, want %q", got, tc.after)
			}
		})
	}
}

func TestASuggestionThatWentIntoAVersionIsAppliedAndItsAcceptanceIsFinal(t *testing.T) {
	held := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	stranger := pgtype.UUID{Bytes: [16]byte{4}, Valid: true}
	version := pgtype.UUID{Bytes: [16]byte{9}, Valid: true}
	earlier := pgtype.UUID{Bytes: [16]byte{8}, Valid: true}
	cases := []struct {
		name      string
		decision  Decision
		alreadyOn pgtype.UUID
		named     []pgtype.UUID
		want      Event
		appliedTo pgtype.UUID
	}{
		{"an accepted suggestion goes into the version", DecisionAccepted, pgtype.UUID{}, []pgtype.UUID{held},
			SuggestionsApplied{SkillVersionID: version, SuggestionIDs: []pgtype.UUID{held}, witnessed: []pgtype.UUID{held}}, version},
		{"a rejection sent before the version was recorded does not stand", DecisionRejected, pgtype.UUID{},
			[]pgtype.UUID{held}, SuggestionsApplied{
				SkillVersionID: version, SuggestionIDs: []pgtype.UUID{held},
				newlyAccepted: []pgtype.UUID{held}, witnessed: []pgtype.UUID{held},
			}, version},
		{"a suggestion already in an earlier version goes into this one too", DecisionAccepted, earlier,
			[]pgtype.UUID{held}, SuggestionsApplied{SkillVersionID: version, SuggestionIDs: []pgtype.UUID{held}}, earlier},
		{"a suggestion this evaluation does not hold is left out", DecisionAccepted, pgtype.UUID{},
			[]pgtype.UUID{stranger, held}, SuggestionsApplied{
				SkillVersionID: version, SuggestionIDs: []pgtype.UUID{held}, witnessed: []pgtype.UUID{held},
			}, version},
		{"a letter naming only suggestions this evaluation does not hold", DecisionAccepted, pgtype.UUID{},
			[]pgtype.UUID{stranger}, Refused{RefusedNothingToApply}, pgtype.UUID{}},
		{"the same version delivered again records nothing new", DecisionAccepted, version,
			[]pgtype.UUID{held}, Refused{RefusedNothingToApply}, version},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := revisionIn(StatusCompleted, false)
			e.suggestions = map[pgtype.UUID]gen.EvaluationSuggestion{
				held: {ID: held, Decision: string(tc.decision), AppliedSkillVersionID: tc.alreadyOn},
			}
			if tc.alreadyOn.Valid {
				e.applied = map[pgtype.UUID]map[pgtype.UUID]struct{}{held: {tc.alreadyOn: {}}}
			}

			e.RecordApplied(version, tc.named)

			assertEvents(t, e, tc.want)
			if got := e.AppliedVersion(held); got != tc.appliedTo {
				t.Fatalf("the suggestion is applied to %v, want %v", got, tc.appliedTo)
			}
			if _, refused := tc.want.(Refused); !refused && e.Decision(held) != DecisionAccepted {
				t.Fatalf("a suggestion inside a version is %q, want accepted", e.Decision(held))
			}
		})
	}
}

func TestEverySuggestionAVersionCarriesIsRecordedInOneEvent(t *testing.T) {
	first := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	second := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	version := pgtype.UUID{Bytes: [16]byte{9}, Valid: true}
	e := revisionIn(StatusCompleted, false)
	e.suggestions = map[pgtype.UUID]gen.EvaluationSuggestion{
		first:  {ID: first, Decision: string(DecisionAccepted)},
		second: {ID: second, Decision: string(DecisionRejected)},
	}

	e.RecordApplied(version, []pgtype.UUID{first, second})

	assertEvents(t, e, SuggestionsApplied{
		SkillVersionID: version, SuggestionIDs: []pgtype.UUID{first, second},
		newlyAccepted: []pgtype.UUID{second}, witnessed: []pgtype.UUID{first, second},
	})
	for _, id := range []pgtype.UUID{first, second} {
		if e.AppliedVersion(id) != version || e.Decision(id) != DecisionAccepted {
			t.Fatalf("suggestion %v: applied to %v as %q, want the version and accepted", id, e.AppliedVersion(id), e.Decision(id))
		}
	}
}

func TestEachRefusalAnswersWithItsOwnError(t *testing.T) {
	cases := map[Refusal]error{
		RefusedAwaitingJudge:     errEvaluationInProgress,
		RefusedSettled:           errEvaluationSettled,
		RefusedSuperseded:        ErrNotFound,
		RefusedNotAChoice:        errNotAChoice,
		RefusedUnknownSuggestion: ErrNotFound,
		RefusedAcceptanceIsFinal: errAcceptanceIsFinal,
	}
	for reason, want := range cases {
		if got := reason.err(); !errors.Is(got, want) {
			t.Errorf("%s answers %v, want %v", reason, got, want)
		}
	}
}

func TestARefusedCommandIsAnsweredWithoutTouchingTheTransaction(t *testing.T) {
	e := revisionIn(StatusCompleted, false)
	e.Complete(aVerdict("a second opinion", OverallNotMet))

	if err := saveUnlessRefused(t.Context(), nil, e); !errors.Is(err, errEvaluationSettled) {
		t.Fatalf("saving a refused completion: want errEvaluationSettled, got %v", err)
	}
}

func TestEveryEvaluationEventHasACatalogueNameAndAFixedPayloadShape(t *testing.T) {
	cases := []struct {
		event Event
		name  string
		keys  []string
	}{
		{EvaluationStarted{}, outbox.EvaluationStarted, []string{"judge_model", "judge_prompt_version", "rubric_version"}},
		{EvaluationSuperseded{}, outbox.EvaluationSuperseded, []string{}},
		{EvaluationCompleted{}, outbox.EvaluationCompleted, []string{"evidence_complete", "overall"}},
		{EvaluationFailed{}, outbox.EvaluationFailed, []string{"evidence_complete"}},
		{FeedbackRecorded{}, outbox.EvaluationFeedbackRecorded, []string{"has_comment", "helpful"}},
		{SuggestionDecided{}, outbox.EvaluationSuggestionDecided, []string{"decision", "suggestion_id"}},
		{SuggestionsApplied{}, outbox.EvaluationSuggestionsApplied, []string{"skill_version_id", "suggestion_ids"}},
	}
	for _, tc := range cases {
		if got := tc.event.eventType(); got != tc.name || !slices.Contains(outbox.EventTypes, got) {
			t.Errorf("%T is sent as %q, want %q from the closed set", tc.event, got, tc.name)
		}
		encoded, err := json.Marshal(tc.event)
		if err != nil {
			t.Fatalf("%T: %v", tc.event, err)
		}
		var payload map[string]any
		if err := json.Unmarshal(encoded, &payload); err != nil {
			t.Fatalf("%T: %v", tc.event, err)
		}
		keys := make([]string, 0, len(payload))
		for key := range payload {
			keys = append(keys, key)
		}
		if slices.Sort(keys); !slices.Equal(keys, tc.keys) {
			t.Errorf("%T payload keys = %v, want %v even when the values are empty", tc.event, keys, tc.keys)
		}
	}
	if got := (Refused{}).eventType(); got != "" {
		t.Errorf("a refusal is an answer, not a fact to publish, yet it names %q", got)
	}
}
