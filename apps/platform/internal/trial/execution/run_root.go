package run

import (
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type Refusal string

const (
	RefusedIllegalTransition Refusal = "illegal_transition"
	RefusedFinished          Refusal = "finished"
	RefusedUnknownAttempt    Refusal = "unknown_attempt"
	RefusedAttemptFinished   Refusal = "attempt_finished"
	RefusedGrantsCannotMove  Refusal = "grants_cannot_move"
)

var (
	errUnknownAttempt  = errors.New("run: the attempt does not belong to this run")
	errAttemptFinished = errors.New("run: the attempt has already finished")
)

const requestedReason = "已收到這次 Run 的請求"

type Event interface{ eventType() string }

type Refused struct{ Reason Refusal }

func (Refused) eventType() string { return "" }

func (r Refused) err() error {
	switch r.Reason {
	case RefusedIllegalTransition:
		return ErrIllegalTransition
	case RefusedFinished:
		return ErrRunFinished
	case RefusedAttemptFinished:
		return errAttemptFinished
	case RefusedGrantsCannotMove:
		return ErrObjectGrantTransition
	}
	return errUnknownAttempt
}

type StatusChanged struct {
	outbox.RunStatusChanged
	attemptID    pgtype.UUID
	failure      FailureClass
	closedGrants []pgtype.UUID
}

func (e StatusChanged) eventType() string {
	eventType, _ := outbox.StatusEvent(e.ToStatus)
	return eventType
}

type CancelRequested struct{}

type ProviderAssigned struct {
	Provider string `json:"provider"`
}

type AttemptStarted struct {
	AttemptID     pgtype.UUID `json:"attempt_id"`
	AttemptNumber int32       `json:"attempt_number"`
	Provider      string      `json:"provider"`
}

type AttemptDispatched struct {
	AttemptID pgtype.UUID `json:"attempt_id"`
}

type AttemptFinished struct {
	AttemptID  pgtype.UUID `json:"attempt_id"`
	ErrorClass *string     `json:"error_class"`
}

type ObjectGrantsRecorded struct {
	AttemptID pgtype.UUID `json:"attempt_id"`
}

func (CancelRequested) eventType() string      { return outbox.RunCancelRequested }
func (ProviderAssigned) eventType() string     { return outbox.RunProviderAssigned }
func (AttemptStarted) eventType() string       { return outbox.RunAttemptStarted }
func (AttemptDispatched) eventType() string    { return outbox.RunAttemptDispatched }
func (AttemptFinished) eventType() string      { return outbox.RunAttemptFinished }
func (ObjectGrantsRecorded) eventType() string { return outbox.RunObjectGrantsRecorded }

type Run struct {
	row      gen.Run
	attempts []gen.RunAttempt
	events   []Event
	saved    int
}

func startRun(row gen.Run) *Run {
	row = cloneRun(row)
	row.Status = gen.RunStatusQueued
	r := &Run{row: row}
	r.record(StatusChanged{RunStatusChanged: outbox.RunStatusChanged{
		ToStatus: string(row.Status), Reason: requestedReason,
	}})
	return r
}

func (r *Run) Row() gen.Run { return cloneRun(r.row) }

func (r *Run) Status() gen.RunStatus { return r.row.Status }

func (r *Run) Attempt(id pgtype.UUID) gen.RunAttempt {
	if a := r.attempt(id); a != nil {
		return cloneAttempt(*a)
	}
	return gen.RunAttempt{}
}

func (r *Run) LatestAttempt() gen.RunAttempt {
	if len(r.attempts) == 0 {
		return gen.RunAttempt{}
	}
	return cloneAttempt(r.attempts[len(r.attempts)-1])
}

func (r *Run) Refusal() (Refused, bool) {
	for _, event := range r.events {
		if refused, ok := event.(Refused); ok {
			return refused, true
		}
	}
	return Refused{}, false
}

func (r *Run) Transition(to gen.RunStatus, reason statusReason, failure FailureClass, attemptID pgtype.UUID) {
	from := r.row.Status
	if !CanTransition(from, to) {
		r.refuse(RefusedIllegalTransition)
		return
	}
	r.row.Status = to
	if failure != "" {
		class := string(failure)
		r.row.FailureClass = &class
	}
	changed := StatusChanged{
		RunStatusChanged: outbox.RunStatusChanged{ToStatus: string(to), FromStatus: string(from), Reason: string(reason)},
		attemptID:        attemptID, failure: failure,
	}
	if to == gen.RunStatusRunning && !r.row.StartedAt.Valid {
		r.row.StartedAt = stampNow()
	}
	if IsTerminal(to) {
		r.row.FinishedAt = stampNow()
		for i := range r.attempts {
			if closeUnissuedGrants(&r.attempts[i]) {
				changed.closedGrants = append(changed.closedGrants, r.attempts[i].ID)
			}
		}
	}
	r.record(changed)
}

func (r *Run) RequestCancel() {
	switch {
	case IsTerminal(r.row.Status):
		r.refuse(RefusedFinished)
	case !r.row.CancelRequestedAt.Valid:
		r.row.CancelRequestedAt = stampNow()
		r.record(CancelRequested{})
	}
}

func (r *Run) AssignProvider(provider string, runtimeSnapshot []byte) {
	switch {
	case IsTerminal(r.row.Status):
		r.refuse(RefusedFinished)
	case !alreadyPinned(r.row) || r.row.Provider != provider:
		r.row.Provider, r.row.RuntimeSnapshot = provider, slices.Clone(runtimeSnapshot)
		r.record(ProviderAssigned{Provider: provider})
	}
}

func (r *Run) StartAttempt(provider string) {
	if IsTerminal(r.row.Status) {
		r.refuse(RefusedFinished)
		return
	}
	attempt := gen.RunAttempt{
		RunID: r.row.ID, WorkspaceID: r.row.WorkspaceID, Provider: provider, AttemptNumber: r.nextAttemptNumber(),
		ObjectGrantsState: string(ObjectGrantStateUnissued), ObjectGrantsExpireAt: unissuedGrantsFence,
	}
	r.attempts = append(r.attempts, attempt)
	r.record(AttemptStarted{AttemptNumber: attempt.AttemptNumber, Provider: provider})
}

func (r *Run) RecordDispatch(attemptID pgtype.UUID, providerRunID string) {
	a := r.attempt(attemptID)
	if a == nil {
		r.refuse(RefusedUnknownAttempt)
		return
	}
	a.ProviderRunID = &providerRunID
	if !a.StartedAt.Valid {
		a.StartedAt = stampNow()
	}
	r.record(AttemptDispatched{AttemptID: attemptID})
}

func (r *Run) FinishAttempt(attemptID pgtype.UUID, errorClass, message string) {
	a := r.attempt(attemptID)
	switch {
	case a == nil:
		r.refuse(RefusedUnknownAttempt)
	case a.FinishedAt.Valid:
		r.refuse(RefusedAttemptFinished)
	default:
		a.FinishedAt = stampNow()
		a.ErrorClass, a.ErrorMessage = nonEmpty(errorClass), nonEmpty(message)
		closeUnissuedGrants(a)
		r.record(AttemptFinished{AttemptID: attemptID, ErrorClass: a.ErrorClass})
	}
}

func (r *Run) RecordGrantExpiry(attemptID pgtype.UUID, expires time.Time) {
	a := r.attempt(attemptID)
	switch {
	case a == nil:
		r.refuse(RefusedUnknownAttempt)
	case !CanTransitionObjectGrant(ObjectGrantState(a.ObjectGrantsState), ObjectGrantStateRecorded):
		r.refuse(RefusedGrantsCannotMove)
	default:
		a.ObjectGrantsState = string(ObjectGrantStateRecorded)
		a.ObjectGrantsExpireAt = pgtype.Timestamptz{Time: expires, Valid: true}
		r.record(ObjectGrantsRecorded{AttemptID: attemptID})
	}
}

func (r *Run) attempt(id pgtype.UUID) *gen.RunAttempt {
	for i := range r.attempts {
		if r.attempts[i].ID == id {
			return &r.attempts[i]
		}
	}
	return nil
}

func (r *Run) nextAttemptNumber() int32 {
	var latest int32
	for _, a := range r.attempts {
		latest = max(latest, a.AttemptNumber)
	}
	return latest + 1
}

func closeUnissuedGrants(a *gen.RunAttempt) bool {
	if ObjectGrantState(a.ObjectGrantsState) != ObjectGrantStateUnissued {
		return false
	}
	a.ObjectGrantsState = string(ObjectGrantStateClosed)
	a.ObjectGrantsExpireAt = pgtype.Timestamptz{Time: objectGrantsExpiredOnArrival(), Valid: true}
	return true
}

func stampNow() pgtype.Timestamptz { return pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true} }

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (r *Run) refuse(reason Refusal) { r.record(Refused{Reason: reason}) }

func (r *Run) record(event Event) { r.events = append(r.events, cloneEvent(event)) }

func cloneRun(row gen.Run) gen.Run {
	row.StatusReason = pgconv.Clone(row.StatusReason)
	row.RuntimeSnapshot = slices.Clone(row.RuntimeSnapshot)
	row.PolicySnapshot = slices.Clone(row.PolicySnapshot)
	row.FailureClass = pgconv.Clone(row.FailureClass)
	return row
}

func cloneAttempt(attempt gen.RunAttempt) gen.RunAttempt {
	attempt.ProviderRunID = pgconv.Clone(attempt.ProviderRunID)
	attempt.ErrorClass = pgconv.Clone(attempt.ErrorClass)
	attempt.ErrorMessage = pgconv.Clone(attempt.ErrorMessage)
	return attempt
}

func cloneEvent(event Event) Event {
	switch event := event.(type) {
	case StatusChanged:
		event.closedGrants = slices.Clone(event.closedGrants)
		return event
	case AttemptFinished:
		event.ErrorClass = pgconv.Clone(event.ErrorClass)
		return event
	}
	return event
}
