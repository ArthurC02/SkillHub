package eval

import (
	"errors"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type Refusal string

const (
	RefusedAwaitingJudge     Refusal = "awaiting_judge"
	RefusedSettled           Refusal = "settled"
	RefusedSuperseded        Refusal = "superseded"
	RefusedNotAChoice        Refusal = "not_a_choice"
	RefusedUnknownSuggestion Refusal = "unknown_suggestion"
	RefusedAcceptanceIsFinal Refusal = "acceptance_is_final"
)

var (
	errAcceptanceIsFinal = errors.New("this suggestion has already been built into a skill version, so its acceptance " +
		"cannot be withdrawn; create a further version to change the package again")
	errNotAChoice = errors.New("`decision` must be \"accepted\" or \"rejected\"")
)

func (r Refusal) err() error {
	switch r {
	case RefusedAwaitingJudge:
		return errEvaluationInProgress
	case RefusedSettled:
		return errEvaluationSettled
	case RefusedAcceptanceIsFinal:
		return errAcceptanceIsFinal
	case RefusedNotAChoice:
		return errNotAChoice
	}
	return ErrNotFound
}

type Event interface{ eventType() string }

type Refused struct{ Reason Refusal }

type EvaluationStarted struct {
	JudgeModel         *string `json:"judge_model"`
	JudgePromptVersion *string `json:"judge_prompt_version"`
	RubricVersion      *string `json:"rubric_version"`
}

type EvaluationSuperseded struct{}

type EvaluationCompleted struct {
	Overall          Overall `json:"overall"`
	EvidenceComplete bool    `json:"evidence_complete"`
}

type EvaluationFailed struct {
	EvidenceComplete bool `json:"evidence_complete"`
}

type FeedbackRecorded struct {
	Helpful    bool `json:"helpful"`
	HasComment bool `json:"has_comment"`
}

type SuggestionDecided struct {
	SuggestionID pgtype.UUID `json:"suggestion_id"`
	Decision     Decision    `json:"decision"`
}

func (Refused) eventType() string              { return "" }
func (EvaluationStarted) eventType() string    { return outbox.EvaluationStarted }
func (EvaluationSuperseded) eventType() string { return outbox.EvaluationSuperseded }
func (EvaluationCompleted) eventType() string  { return outbox.EvaluationCompleted }
func (EvaluationFailed) eventType() string     { return outbox.EvaluationFailed }
func (FeedbackRecorded) eventType() string     { return outbox.EvaluationFeedbackRecorded }
func (SuggestionDecided) eventType() string    { return outbox.EvaluationSuggestionDecided }

type failure struct {
	summary          string
	findings         []Finding
	evidenceComplete bool
}

type Evaluation struct {
	row         gen.Evaluation
	suggestions map[pgtype.UUID]gen.EvaluationSuggestion
	verdict     verdict
	failure     failure
	events      []Event
}

func startEvaluation(workspaceID, runID pgtype.UUID, declared EvaluationStarted) *Evaluation {
	e := &Evaluation{row: gen.Evaluation{
		WorkspaceID: workspaceID, RunID: runID, Status: string(StatusPending),
		JudgeModel: declared.JudgeModel, JudgePromptVersion: declared.JudgePromptVersion,
		RubricVersion: declared.RubricVersion,
	}}
	e.record(declared)
	return e
}

func (e *Evaluation) Status() Status { return Status(e.row.Status) }

func (e *Evaluation) Superseded() bool { return e.row.SupersededAt.Valid }

func (e *Evaluation) Decision(suggestionID pgtype.UUID) Decision {
	return Decision(e.suggestions[suggestionID].Decision)
}

func (e *Evaluation) Events() []Event { return slices.Clone(e.events) }

func (e *Evaluation) Refusal() (Refusal, bool) {
	for _, event := range e.events {
		if refused, ok := event.(Refused); ok {
			return refused.Reason, true
		}
	}
	return "", false
}

func (e *Evaluation) Supersede() {
	switch {
	case e.Superseded():
		e.refuse(RefusedSuperseded)
	case e.Status().AwaitsTheJudge():
		e.refuse(RefusedAwaitingJudge)
	default:
		e.row.SupersededAt = pgtype.Timestamptz{Valid: true}
		e.record(EvaluationSuperseded{})
	}
}

func (e *Evaluation) Complete(v verdict) {
	if !e.Status().AwaitsTheJudge() {
		e.refuse(RefusedSettled)
		return
	}
	e.row.Status, e.verdict = string(StatusCompleted), v
	e.record(EvaluationCompleted{Overall: v.overall, EvidenceComplete: v.evidenceComplete})
}

func (e *Evaluation) Fail(f failure) {
	if !e.Status().AwaitsTheJudge() {
		e.refuse(RefusedSettled)
		return
	}
	e.row.Status, e.failure = string(StatusFailed), f
	e.record(EvaluationFailed{EvidenceComplete: f.evidenceComplete})
}

func (e *Evaluation) RecordFeedback(helpful bool, comment string) {
	if e.Superseded() {
		e.refuse(RefusedSuperseded)
		return
	}
	e.row.FeedbackHelpful, e.row.FeedbackComment = &helpful, nil
	if comment != "" {
		e.row.FeedbackComment = &comment
	}
	e.record(FeedbackRecorded{Helpful: helpful, HasComment: comment != ""})
}

func (e *Evaluation) Decide(suggestionID pgtype.UUID, to Decision) {
	suggestion, known := e.suggestions[suggestionID]
	switch {
	case !to.chosen():
		e.refuse(RefusedNotAChoice)
	case !known:
		e.refuse(RefusedUnknownSuggestion)
	case suggestion.AppliedSkillVersionID.Valid && to != DecisionAccepted:
		e.refuse(RefusedAcceptanceIsFinal)
	default:
		suggestion.Decision = string(to)
		e.suggestions[suggestionID] = suggestion
		e.record(SuggestionDecided{SuggestionID: suggestionID, Decision: to})
	}
}

func (e *Evaluation) refuse(reason Refusal) { e.record(Refused{Reason: reason}) }

func (e *Evaluation) record(event Event) { e.events = append(e.events, event) }
