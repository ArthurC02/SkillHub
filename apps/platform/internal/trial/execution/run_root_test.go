package run

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

var (
	firstAttempt    = pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	secondAttempt   = pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	thirdAttempt    = pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	strangerAttempt = pgtype.UUID{Bytes: [16]byte{9}, Valid: true}
)

func runIn(status gen.RunStatus, attempts ...gen.RunAttempt) *Run {
	return &Run{row: gen.Run{Status: status, RuntimeSnapshot: []byte("{}")}, attempts: attempts}
}

func attemptWith(id pgtype.UUID, grants ObjectGrantState, finished bool) gen.RunAttempt {
	return gen.RunAttempt{ID: id, ObjectGrantsState: string(grants), FinishedAt: pgtype.Timestamptz{Valid: finished}}
}

func assertRunEvents(t *testing.T, r *Run, want ...Event) {
	t.Helper()
	if got := r.Events(); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %#v, want %#v", got, want)
	}
}

func moved(from, to gen.RunStatus, reason string) outbox.RunStatusChanged {
	return outbox.RunStatusChanged{ToStatus: string(to), FromStatus: string(from), Reason: reason}
}

func TestANewRunStartsQueuedAndSaysItWasRequested(t *testing.T) {
	r := startRun(gen.Run{Status: gen.RunStatusRunning})

	if r.Status() != gen.RunStatusQueued {
		t.Fatalf("a new run is %q, want queued", r.Status())
	}
	assertRunEvents(t, r, StatusChanged{RunStatusChanged: outbox.RunStatusChanged{
		ToStatus: string(gen.RunStatusQueued), Reason: requestedReason,
	}})
}

func TestARunMovesOnlyAlongTheTransitionTable(t *testing.T) {
	cases := []struct {
		name     string
		from, to gen.RunStatus
		want     Event
		after    gen.RunStatus
	}{
		{"queued to provisioning", gen.RunStatusQueued, gen.RunStatusProvisioning,
			StatusChanged{RunStatusChanged: moved(gen.RunStatusQueued, gen.RunStatusProvisioning, "why"), attemptID: firstAttempt},
			gen.RunStatusProvisioning},
		{"evaluating to succeeded", gen.RunStatusEvaluating, gen.RunStatusSucceeded,
			StatusChanged{RunStatusChanged: moved(gen.RunStatusEvaluating, gen.RunStatusSucceeded, "why"), attemptID: firstAttempt},
			gen.RunStatusSucceeded},
		{"queued straight to succeeded", gen.RunStatusQueued, gen.RunStatusSucceeded,
			Refused{RefusedIllegalTransition}, gen.RunStatusQueued},
		{"a finished run goes nowhere", gen.RunStatusFailed, gen.RunStatusCancelled,
			Refused{RefusedIllegalTransition}, gen.RunStatusFailed},
		{"a run does not move to where it already is", gen.RunStatusRunning, gen.RunStatusRunning,
			Refused{RefusedIllegalTransition}, gen.RunStatusRunning},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := runIn(tc.from)

			r.Transition(tc.to, "why", "", firstAttempt)

			assertRunEvents(t, r, tc.want)
			if r.Status() != tc.after {
				t.Fatalf("status = %q, want %q", r.Status(), tc.after)
			}
		})
	}
}

func TestAFailureClassStaysOnTheRun(t *testing.T) {
	r := runIn(gen.RunStatusRunning)

	r.Transition(gen.RunStatusFailed, "the workload reported failure", failureWorkload, firstAttempt)

	if class := r.Row().FailureClass; class == nil || *class != string(failureWorkload) {
		t.Fatalf("failure class = %v, want %q", class, failureWorkload)
	}
	assertRunEvents(t, r, StatusChanged{
		RunStatusChanged: moved(gen.RunStatusRunning, gen.RunStatusFailed, "the workload reported failure"),
		attemptID:        firstAttempt, failure: failureWorkload,
	})
}

func TestEndingARunClosesOnlyGrantsThatWereNeverIssued(t *testing.T) {
	for _, to := range []gen.RunStatus{gen.RunStatusFailed, gen.RunStatusCancelled, gen.RunStatusTimedOut} {
		t.Run(string(to), func(t *testing.T) {
			r := runIn(gen.RunStatusRunning,
				attemptWith(firstAttempt, ObjectGrantStateUnissued, false),
				attemptWith(secondAttempt, ObjectGrantStateRecorded, true),
				attemptWith(thirdAttempt, ObjectGrantStateLegacyUnknown, false))

			r.Transition(to, "stopped", "", pgtype.UUID{})

			got := []string{
				r.Attempt(firstAttempt).ObjectGrantsState, r.Attempt(secondAttempt).ObjectGrantsState,
				r.Attempt(thirdAttempt).ObjectGrantsState,
			}
			want := []string{string(ObjectGrantStateClosed), string(ObjectGrantStateRecorded), string(ObjectGrantStateLegacyUnknown)}
			if !slices.Equal(got, want) {
				t.Fatalf("grant states after %s = %v, want %v", to, got, want)
			}
		})
	}
	t.Run("a run still in progress keeps its unissued grants", func(t *testing.T) {
		r := runIn(gen.RunStatusPreparing, attemptWith(firstAttempt, ObjectGrantStateUnissued, false))

		r.Transition(gen.RunStatusRunning, "started", "", firstAttempt)

		if got := r.Attempt(firstAttempt).ObjectGrantsState; got != string(ObjectGrantStateUnissued) {
			t.Fatalf("grant state = %q, want unissued", got)
		}
	})
}

func TestCancelIsRecordedOnceAndNeverOnAFinishedRun(t *testing.T) {
	t.Run("a run in progress", func(t *testing.T) {
		r := runIn(gen.RunStatusRunning)
		r.RequestCancel()
		assertRunEvents(t, r, CancelRequested{})
		if !r.Row().CancelRequestedAt.Valid {
			t.Fatal("the cancel request was not kept on the run")
		}
	})
	t.Run("a run already asked to stop", func(t *testing.T) {
		r := runIn(gen.RunStatusQueued)
		r.row.CancelRequestedAt = pgtype.Timestamptz{Time: time.Unix(100, 0), Valid: true}
		r.RequestCancel()
		assertRunEvents(t, r)
		if r.Row().CancelRequestedAt.Time != time.Unix(100, 0) {
			t.Fatal("asking again moved the time of the first request")
		}
	})
	t.Run("a finished run", func(t *testing.T) {
		r := runIn(gen.RunStatusSucceeded)
		r.RequestCancel()
		assertRunEvents(t, r, Refused{RefusedFinished})
		if r.Row().CancelRequestedAt.Valid {
			t.Fatal("a finished run was marked as asked to stop")
		}
	})
}

func TestAProviderIsAssignedOnceAndNeverToAFinishedRun(t *testing.T) {
	t.Run("an unassigned run", func(t *testing.T) {
		r := runIn(gen.RunStatusQueued)
		r.AssignProvider("fake_sandbox", []byte(`{"provider":"fake_sandbox"}`))
		assertRunEvents(t, r, ProviderAssigned{Provider: "fake_sandbox"})
		if r.Row().Provider != "fake_sandbox" || string(r.Row().RuntimeSnapshot) != `{"provider":"fake_sandbox"}` {
			t.Fatalf("run = %q %s, want the provider and its snapshot", r.Row().Provider, r.Row().RuntimeSnapshot)
		}
	})
	t.Run("a run already assigned", func(t *testing.T) {
		r := runIn(gen.RunStatusProvisioning)
		r.row.Provider, r.row.RuntimeSnapshot = "first", []byte(`{"provider":"first"}`)
		r.AssignProvider("second", []byte(`{"provider":"second"}`))
		assertRunEvents(t, r)
		if r.Row().Provider != "first" {
			t.Fatalf("provider = %q, want the first assignment kept", r.Row().Provider)
		}
	})
	t.Run("a finished run", func(t *testing.T) {
		r := runIn(gen.RunStatusCancelled)
		r.AssignProvider("fake_sandbox", []byte(`{"provider":"fake_sandbox"}`))
		assertRunEvents(t, r, Refused{RefusedFinished})
	})
}

func TestAnAttemptStartsOnlyOnARunStillInProgress(t *testing.T) {
	r := runIn(gen.RunStatusProvisioning)
	r.StartAttempt("fake_sandbox")
	assertRunEvents(t, r, AttemptStarted{Provider: "fake_sandbox"})
	if a := r.LatestAttempt(); a.Provider != "fake_sandbox" || a.ObjectGrantsState != string(ObjectGrantStateUnissued) {
		t.Fatalf("the new attempt = %+v, want the provider with unissued grants", a)
	}

	finished := runIn(gen.RunStatusTimedOut)
	finished.StartAttempt("fake_sandbox")
	assertRunEvents(t, finished, Refused{RefusedFinished})
	if (finished.LatestAttempt() != gen.RunAttempt{}) {
		t.Fatal("a finished run gained an attempt")
	}
}

func TestADispatchIsRecordedOnlyOnItsOwnAttempt(t *testing.T) {
	r := runIn(gen.RunStatusProvisioning, attemptWith(firstAttempt, ObjectGrantStateRecorded, false))
	r.RecordDispatch(firstAttempt, "sandbox-1")
	assertRunEvents(t, r, AttemptDispatched{AttemptID: firstAttempt})
	if handle := r.Attempt(firstAttempt).ProviderRunID; handle == nil || *handle != "sandbox-1" {
		t.Fatalf("provider handle = %v, want sandbox-1", handle)
	}

	stranger := runIn(gen.RunStatusProvisioning, attemptWith(firstAttempt, ObjectGrantStateRecorded, false))
	stranger.RecordDispatch(strangerAttempt, "sandbox-1")
	assertRunEvents(t, stranger, Refused{RefusedUnknownAttempt})
}

func TestAnAttemptFinishesOnceAndClosesGrantsItNeverIssued(t *testing.T) {
	execution := string(errClassExecution)
	cases := []struct {
		name          string
		held          gen.RunAttempt
		target        pgtype.UUID
		class         string
		want          Event
		grantsAfter   ObjectGrantState
		finishedAfter bool
	}{
		{"an attempt that never issued its grants", attemptWith(firstAttempt, ObjectGrantStateUnissued, false),
			firstAttempt, execution, AttemptFinished{AttemptID: firstAttempt, ErrorClass: &execution},
			ObjectGrantStateClosed, true},
		{"an attempt whose grants were issued", attemptWith(firstAttempt, ObjectGrantStateRecorded, false),
			firstAttempt, "", AttemptFinished{AttemptID: firstAttempt},
			ObjectGrantStateRecorded, true},
		{"an attempt that already finished", attemptWith(firstAttempt, ObjectGrantStateRecorded, true),
			firstAttempt, execution, Refused{RefusedAttemptFinished},
			ObjectGrantStateRecorded, true},
		{"an attempt this run does not hold", attemptWith(firstAttempt, ObjectGrantStateUnissued, false),
			strangerAttempt, execution, Refused{RefusedUnknownAttempt},
			ObjectGrantStateUnissued, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := runIn(gen.RunStatusRunning, tc.held)

			r.FinishAttempt(tc.target, tc.class, "")

			assertRunEvents(t, r, tc.want)
			a := r.Attempt(firstAttempt)
			if ObjectGrantState(a.ObjectGrantsState) != tc.grantsAfter || a.FinishedAt.Valid != tc.finishedAfter {
				t.Fatalf("attempt = grants %q finished %v, want %q %v",
					a.ObjectGrantsState, a.FinishedAt.Valid, tc.grantsAfter, tc.finishedAfter)
			}
		})
	}
}

func TestAnAttemptKeepsItsOutcome(t *testing.T) {
	r := runIn(gen.RunStatusRunning, attemptWith(firstAttempt, ObjectGrantStateRecorded, false))

	r.FinishAttempt(firstAttempt, errClassTimeout, "the wall clock ran out")

	a := r.Attempt(firstAttempt)
	if a.ErrorClass == nil || *a.ErrorClass != errClassTimeout || a.ErrorMessage == nil || *a.ErrorMessage != "the wall clock ran out" {
		t.Fatalf("outcome = %v %v, want the class and message given", a.ErrorClass, a.ErrorMessage)
	}
}

func TestGrantExpiryIsRecordedOnlyWhereTheGrantStateAllowsIt(t *testing.T) {
	expires := time.Unix(1_800_000_000, 0).UTC()
	cases := []struct {
		name   string
		from   ObjectGrantState
		target pgtype.UUID
		want   Event
		after  ObjectGrantState
	}{
		{"unissued grants", ObjectGrantStateUnissued, firstAttempt, ObjectGrantsRecorded{AttemptID: firstAttempt}, ObjectGrantStateRecorded},
		{"grants recorded again", ObjectGrantStateRecorded, firstAttempt, ObjectGrantsRecorded{AttemptID: firstAttempt}, ObjectGrantStateRecorded},
		{"closed grants", ObjectGrantStateClosed, firstAttempt, Refused{RefusedGrantsCannotMove}, ObjectGrantStateClosed},
		{"grants from before the state was kept", ObjectGrantStateLegacyUnknown, firstAttempt, Refused{RefusedGrantsCannotMove}, ObjectGrantStateLegacyUnknown},
		{"an attempt this run does not hold", ObjectGrantStateUnissued, strangerAttempt, Refused{RefusedUnknownAttempt}, ObjectGrantStateUnissued},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := runIn(gen.RunStatusProvisioning, attemptWith(firstAttempt, tc.from, false))

			r.RecordGrantExpiry(tc.target, expires)

			assertRunEvents(t, r, tc.want)
			a := r.Attempt(firstAttempt)
			if ObjectGrantState(a.ObjectGrantsState) != tc.after {
				t.Fatalf("grants = %q, want %q", a.ObjectGrantsState, tc.after)
			}
			if _, refused := tc.want.(Refused); !refused && a.ObjectGrantsExpireAt.Time != expires {
				t.Fatalf("expiry = %v, want %v", a.ObjectGrantsExpireAt.Time, expires)
			}
		})
	}
}

func TestEachRunRefusalAnswersWithItsOwnError(t *testing.T) {
	for reason, want := range map[Refusal]error{
		RefusedIllegalTransition: ErrIllegalTransition,
		RefusedFinished:          ErrRunFinished,
		RefusedUnknownAttempt:    errUnknownAttempt,
		RefusedAttemptFinished:   errAttemptFinished,
		RefusedGrantsCannotMove:  ErrObjectGrantTransition,
	} {
		if got := (Refused{reason}).err(); !errors.Is(got, want) {
			t.Errorf("%s answers %v, want %v", reason, got, want)
		}
	}
}

func TestEveryRunEventHasACatalogueNameAndAFixedPayloadShape(t *testing.T) {
	cases := []struct {
		event Event
		name  string
		keys  []string
	}{
		{StatusChanged{RunStatusChanged: moved(gen.RunStatusRunning, gen.RunStatusFailed, "boom")}, outbox.RunFailed,
			[]string{"from_status", "reason", "to_status"}},
		{CancelRequested{}, outbox.RunCancelRequested, []string{}},
		{ProviderAssigned{}, outbox.RunProviderAssigned, []string{"provider"}},
		{AttemptStarted{}, outbox.RunAttemptStarted, []string{"attempt_id", "attempt_number", "provider"}},
		{AttemptDispatched{}, outbox.RunAttemptDispatched, []string{"attempt_id"}},
		{AttemptFinished{}, outbox.RunAttemptFinished, []string{"attempt_id", "error_class"}},
		{ObjectGrantsRecorded{}, outbox.RunObjectGrantsRecorded, []string{"attempt_id"}},
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
			t.Errorf("%T payload keys = %v, want %v", tc.event, keys, tc.keys)
		}
	}
	if got := (Refused{}).eventType(); got != "" {
		t.Errorf("a refusal is an answer, not a fact to publish, yet it names %q", got)
	}
}
