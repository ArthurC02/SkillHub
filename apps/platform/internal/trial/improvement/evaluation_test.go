package eval

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

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
			SuggestionsApplied{version, []pgtype.UUID{held}}, version},
		{"a rejection sent before the version was recorded does not stand", DecisionRejected, pgtype.UUID{},
			[]pgtype.UUID{held}, SuggestionsApplied{version, []pgtype.UUID{held}}, version},
		{"a suggestion already in an earlier version goes into this one too", DecisionAccepted, earlier,
			[]pgtype.UUID{held}, SuggestionsApplied{version, []pgtype.UUID{held}}, version},
		{"a suggestion this evaluation does not hold is left out", DecisionAccepted, pgtype.UUID{},
			[]pgtype.UUID{stranger, held}, SuggestionsApplied{version, []pgtype.UUID{held}}, version},
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
