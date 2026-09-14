package eval

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

type recorder struct {
	found    bool
	lookups  int
	inserted []JobArgs
	opts     []*river.InsertOpts
	err      error
}

func (r *recorder) consumer() *RunEventConsumer {
	return &RunEventConsumer{
		HasCurrentEvaluation: func(context.Context, pgtype.UUID, pgtype.UUID) (bool, error) {
			r.lookups++
			if r.err != nil {
				return false, r.err
			}
			return r.found, nil
		},
		Insert: func(_ context.Context, args river.JobArgs, opts *river.InsertOpts,
		) (*rivertype.JobInsertResult, error) {
			r.inserted = append(r.inserted, args.(JobArgs))
			r.opts = append(r.opts, opts)

			r.found = true
			return nil, nil
		},
	}
}

func runEvent(eventType string) outbox.Event {
	var runID, workspaceID pgtype.UUID
	_ = runID.Scan("11111111-1111-4111-8111-111111111111")
	_ = workspaceID.Scan("22222222-2222-4222-8222-222222222222")
	return outbox.Event{
		EventType: eventType, AggregateType: "run",
		AggregateID: runID, WorkspaceID: workspaceID,
	}
}

func TestRunEventConsumerEnqueuesOncePerRun(t *testing.T) {
	for _, eventType := range []string{"run.succeeded", "run.failed"} {
		r := &recorder{}
		c := r.consumer()
		for range 2 {
			if err := c.Deliver(context.Background(), runEvent(eventType)); err != nil {
				t.Fatalf("%s: deliver: %v", eventType, err)
			}
		}
		if len(r.inserted) != 1 {
			t.Fatalf("%s redelivered enqueued %d evaluations, want 1", eventType, len(r.inserted))
		}
		if r.inserted[0].RunID != "11111111-1111-4111-8111-111111111111" ||
			r.inserted[0].WorkspaceID != "22222222-2222-4222-8222-222222222222" {
			t.Errorf("%s enqueued %+v, want the event's own identifiers", eventType, r.inserted[0])
		}

		if r.opts[0] == nil || !r.opts[0].UniqueOpts.ByArgs {
			t.Errorf("%s enqueued without the per-run unique key", eventType)
		}
	}
}

func TestRunEventConsumerIgnoresOtherEvents(t *testing.T) {
	for _, eventType := range []string{
		"run.queued", "run.running", "run.cancelled", "run.timed_out",
		"run.cleanup_cleaned", "skill_version.created",
	} {
		r := &recorder{}
		if err := r.consumer().Deliver(context.Background(), runEvent(eventType)); err != nil {
			t.Fatalf("%s: deliver: %v", eventType, err)
		}
		if r.lookups != 0 || len(r.inserted) != 0 {
			t.Errorf("%s caused %d lookups and %d inserts, want none", eventType, r.lookups, len(r.inserted))
		}
	}
}

func TestRunEventConsumerRefusesToGuessWhenTheLookupFails(t *testing.T) {
	r := &recorder{err: errors.New("connection refused")}
	err := r.consumer().Deliver(context.Background(), runEvent("run.succeeded"))
	if err == nil {
		t.Fatal("a failed lookup was reported as a successful delivery")
	}
	if len(r.inserted) != 0 {
		t.Errorf("enqueued %d evaluations despite an unreadable lookup", len(r.inserted))
	}
}

type mailbox struct {
	posted []SuggestionsAppliedArgs
	opts   []*river.InsertOpts
}

func (m *mailbox) consumer() *SkillVersionConsumer {
	return &SkillVersionConsumer{Insert: func(_ context.Context, args river.JobArgs, opts *river.InsertOpts,
	) (*rivertype.JobInsertResult, error) {
		m.posted = append(m.posted, args.(SuggestionsAppliedArgs))
		m.opts = append(m.opts, opts)
		return nil, nil
	}}
}

func versionAddedEvent(payload string) outbox.Event {
	var skillID, workspaceID pgtype.UUID
	_ = skillID.Scan("33333333-3333-4333-8333-333333333333")
	_ = workspaceID.Scan("22222222-2222-4222-8222-222222222222")
	return outbox.Event{
		EventType: outbox.SkillVersionAdded, AggregateType: "skill",
		AggregateID: skillID, WorkspaceID: workspaceID, Payload: []byte(payload),
	}
}

const improvedVersion = `{"version_id":"44444444-4444-4444-8444-444444444444","version_number":2,` +
	`"content_hash":"sha256:ab","improved_by":{"evaluation_id":"55555555-5555-4555-8555-555555555555",` +
	`"suggestion_ids":["66666666-6666-4666-8666-666666666666","77777777-7777-4777-8777-777777777777"]}}`

func TestAVersionBuiltFromSuggestionsIsPostedToTheEvaluationsMailbox(t *testing.T) {
	m := &mailbox{}

	if err := m.consumer().Deliver(t.Context(), versionAddedEvent(improvedVersion)); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	if len(m.posted) != 1 {
		t.Fatalf("posted %d letters, want 1", len(m.posted))
	}
	got := m.posted[0]
	ids := []string{
		pgconv.UUIDString(got.WorkspaceID), pgconv.UUIDString(got.SkillID), pgconv.UUIDString(got.SkillVersionID),
		pgconv.UUIDString(got.EvaluationID),
	}
	want := []string{
		"22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333",
		"44444444-4444-4444-8444-444444444444", "55555555-5555-4555-8555-555555555555",
	}
	if !slices.Equal(ids, want) || len(got.SuggestionIDs) != 2 ||
		pgconv.UUIDString(got.SuggestionIDs[1]) != "77777777-7777-4777-8777-777777777777" {
		t.Errorf("posted %+v, want the event's workspace, skill, version, evaluation and both suggestions", got)
	}
	if m.opts[0] == nil || !m.opts[0].UniqueOpts.ByArgs || m.opts[0].MaxAttempts != suggestionsAppliedAttempts {
		t.Errorf("posted with %+v, want one letter per version and %d attempts", m.opts[0], suggestionsAppliedAttempts)
	}
}

func TestAVersionNotBuiltFromSuggestionsPostsNothing(t *testing.T) {
	anotherFact := versionAddedEvent(improvedVersion)
	anotherFact.EventType = outbox.SkillDescribed
	for name, event := range map[string]outbox.Event{
		"an uploaded version": versionAddedEvent(`{"version_id":"44444444-4444-4444-8444-444444444444",` +
			`"version_number":1,"content_hash":"sha256:ab","improved_by":null}`),
		"another skill fact": anotherFact,
	} {
		m := &mailbox{}
		if err := m.consumer().Deliver(t.Context(), event); err != nil {
			t.Fatalf("%s: deliver: %v", name, err)
		}
		if len(m.posted) != 0 {
			t.Errorf("%s posted %d letters, want none", name, len(m.posted))
		}
	}
}

func TestTheMailboxRefusesAnUnreadableOrUnwiredDelivery(t *testing.T) {
	m := &mailbox{}
	if err := m.consumer().Deliver(t.Context(), versionAddedEvent(`{"improved_by":`)); err == nil {
		t.Error("an unreadable payload was reported as delivered")
	}
	if err := (&SkillVersionConsumer{}).Deliver(t.Context(), versionAddedEvent(improvedVersion)); err == nil {
		t.Error("an improved version was accepted by a consumer with no mailbox to post to")
	}
	if len(m.posted) != 0 {
		t.Errorf("posted %d letters from a delivery that failed", len(m.posted))
	}
}

func TestRunEventConsumerFailsClosedWithoutDependencies(t *testing.T) {
	for _, consumer := range []*RunEventConsumer{
		{Insert: (&recorder{}).consumer().Insert},
		{HasCurrentEvaluation: (&recorder{}).consumer().HasCurrentEvaluation},
	} {
		if err := consumer.Deliver(t.Context(), runEvent(outbox.RunSucceeded)); err == nil {
			t.Error("terminal event was accepted with an unwired consumer")
		}
	}
}
