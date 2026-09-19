package creation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func withMessages(n int) Snapshot {
	return Snapshot{Messages: make([]Message, n)}
}

func TestASessionHasRoomForMessagesUpToTheCeilingAndNoFurther(t *testing.T) {
	cases := []struct {
		name string
		held int
		more int
		fits bool
	}{
		{"one more with none held", 0, 1, true},
		{"one more just below the ceiling", MaxMessages - 1, 1, true},
		{"one more at the ceiling", MaxMessages, 1, false},
		{"two more two below the ceiling", MaxMessages - 2, 2, true},
		{"two more one below the ceiling", MaxMessages - 1, 2, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := withMessages(tc.held).hasRoomFor(tc.more); got != tc.fits {
				t.Fatalf("holding %d, room for %d more = %v, want %v", tc.held, tc.more, got, tc.fits)
			}
		})
	}
}

func TestAPersonsNoteIsRefusedOnlyOnceTheSessionIsFull(t *testing.T) {
	s := &Service{}
	below := withMessages(MaxMessages - 1)
	if err := s.attachNote(&below, "one more thing"); err != nil || len(below.Messages) != MaxMessages {
		t.Fatalf("one below the ceiling: err = %v, messages = %d, want the note kept", err, len(below.Messages))
	}
	full := withMessages(MaxMessages)
	if err := s.attachNote(&full, "one more thing"); !errors.Is(err, ErrInvalidCommand) || len(full.Messages) != MaxMessages {
		t.Fatalf("at the ceiling: err = %v, messages = %d, want ErrInvalidCommand and nothing added", err, len(full.Messages))
	}
}

func TestStoppingAStepIsRefusedWhenTheSessionHasNoRoomToSaySo(t *testing.T) {
	e := &envelope{ActiveReceipt: pgtype.UUID{Valid: true}, Snapshot: withMessages(MaxMessages)}

	_, err := stopStep(context.Background(), nil, gen.CreationSession{}, e)

	if !errors.Is(err, ErrInvalidCommand) || !e.ActiveReceipt.Valid || len(e.Snapshot.Messages) != MaxMessages {
		t.Fatalf("at the ceiling: err = %v, receipt kept %v, messages = %d; want ErrInvalidCommand with nothing touched",
			err, e.ActiveReceipt.Valid, len(e.Snapshot.Messages))
	}
}

func TestADraftNameClashIsRaisedOnlyWhileTheSessionHasRoomToAsk(t *testing.T) {
	p := saveableSnapshot()
	p.Duplicates = []Reference{{Name: p.Draft.Skill.Name}}

	p.Messages = make([]Message, MaxMessages-1)
	if _, taken := draftNameTaken(p, p.Draft.ContentHash); !taken {
		t.Fatal("one below the ceiling, a draft named like a duplicate was not raised")
	}
	p.Messages = make([]Message, MaxMessages)
	if _, taken := draftNameTaken(p, p.Draft.ContentHash); taken {
		t.Fatal("at the ceiling, the session was asked to rename with no room left to ask")
	}
}

func TestEachPendingActionTravelsAsTheWordTheInterfaceReads(t *testing.T) {
	for action, word := range map[PendingAction]string{
		NothingPending:                  "",
		PendingBriefConfirmation:        "confirm_brief",
		PendingDiagramConfirmation:      "confirm_diagram",
		PendingReferenceChoice:          "confirm_references",
		PendingDuplicateAcknowledgement: "confirm_duplicate",
		PendingFetchPermission:          "confirm_fetch",
	} {
		encoded, err := json.Marshal(Snapshot{PendingAction: action})
		if err != nil {
			t.Fatal(err)
		}
		var wire struct {
			PendingAction *string `json:"pending_action"`
		}
		if err := json.Unmarshal(encoded, &wire); err != nil {
			t.Fatal(err)
		}
		if wire.PendingAction == nil || *wire.PendingAction != word {
			t.Errorf("%q travels as %v, want the key present with %q", action, wire.PendingAction, word)
		}
	}
}
