package apiserver_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newActFixture(t *testing.T) (*pgxpool.Pool, identity.Workspace, *creation.Service) {
	t.Helper()
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	svc := newCreateService(pool, &jobRecorder{})
	return pool, ws, svc
}

func newActSession(t *testing.T, svc *creation.Service, ws identity.Workspace) (creation.View, pgtype.UUID) {
	t.Helper()
	id := creationID(t)
	v, err := svc.Create(context.Background(), ws, id, "", .5)
	if err != nil {
		t.Fatal(err)
	}
	return v, id
}

func setCreationSnapshotField(t *testing.T, pool *pgxpool.Pool, id pgtype.UUID, field string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	path := "{snapshot," + field + "}"
	if _, err := pool.Exec(context.Background(), `UPDATE creation_sessions SET snapshot = jsonb_set(snapshot, $2::text[], $3::jsonb) WHERE id=$1`, id, path, string(raw)); err != nil {
		t.Fatal(err)
	}
}

func setCreationEnvelopeDeadline(t *testing.T, pool *pgxpool.Pool, id pgtype.UUID, when time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `UPDATE creation_sessions SET snapshot = jsonb_set(snapshot, '{deadline}', to_jsonb($2::text)) WHERE id=$1`, id, when.UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
}

func setCreationRowState(t *testing.T, pool *pgxpool.Pool, id pgtype.UUID, state string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "UPDATE creation_sessions SET state=$2 WHERE id=$1", id, state); err != nil {
		t.Fatal(err)
	}
}

func expireCreationSession(t *testing.T, pool *pgxpool.Pool, id pgtype.UUID) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "UPDATE creation_sessions SET expires_at = now() - interval '1 minute' WHERE id=$1", id); err != nil {
		t.Fatal(err)
	}
}

func creationRowRevision(t *testing.T, pool *pgxpool.Pool, id pgtype.UUID) int64 {
	t.Helper()
	var rev int64
	if err := pool.QueryRow(context.Background(), "SELECT revision FROM creation_sessions WHERE id=$1", id).Scan(&rev); err != nil {
		t.Fatal(err)
	}
	return rev
}

func creationRowSnapshot(t *testing.T, pool *pgxpool.Pool, id pgtype.UUID) []byte {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(context.Background(), "SELECT snapshot FROM creation_sessions WHERE id=$1", id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertRevisionUnchanged(t *testing.T, pool *pgxpool.Pool, id pgtype.UUID, want int64) {
	t.Helper()
	if got := creationRowRevision(t, pool, id); got != want {
		t.Fatalf("revision moved from %d to %d", want, got)
	}
}

func fillerMessages(n int) []creation.Message {
	msgs := make([]creation.Message, n)
	for i := range msgs {
		msgs[i] = creation.Message{Role: "user", Content: "filler"}
	}
	return msgs
}

func failReadRun(t *testing.T) func(context.Context, identity.Workspace, string, creation.Candidate) (string, error) {
	return func(context.Context, identity.Workspace, string, creation.Candidate) (string, error) {
		t.Fatal("ReadRun called")
		return "", nil
	}
}

func TestActInvalidCommandPreconditionsAreRejected(t *testing.T) {
	pool, ws, svc := newActFixture(t)
	v, id := newActSession(t, svc, ws)
	cases := []struct {
		name string
		cmd  creation.Command
	}{
		{"invalid command id", creation.Command{ID: pgtype.UUID{}, ExpectedRevision: v.Revision, Kind: "cancel"}},
		{"expected revision zero", creation.Command{ID: creationID(t), ExpectedRevision: 0, Kind: "cancel"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := svc.Act(context.Background(), ws, id, tc.cmd)
			if !errors.Is(err, creation.ErrInvalidCommand) {
				t.Fatalf("got %v, want ErrInvalidCommand", err)
			}
		})
	}
	assertRevisionUnchanged(t, pool, id, v.Revision)
}

func TestActUnknownOrExpiredSessionIsNotFound(t *testing.T) {
	pool, ws, svc := newActFixture(t)
	t.Run("unknown session id", func(t *testing.T) {
		_, _, err := svc.Act(context.Background(), ws, creationID(t), creation.Command{ID: creationID(t), ExpectedRevision: 1, Kind: "cancel"})
		if !errors.Is(err, creation.ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}
	})
	t.Run("expired session", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		expireCreationSession(t, pool, id)
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "cancel"})
		if !errors.Is(err, creation.ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
}

func TestActUndecodableSnapshotSurfacesADecodeError(t *testing.T) {
	pool, ws, svc := newActFixture(t)
	v, id := newActSession(t, svc, ws)
	if _, err := pool.Exec(context.Background(), `UPDATE creation_sessions SET snapshot = '{"snapshot": 5}'::jsonb WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "cancel"})
	if err == nil || errors.Is(err, creation.ErrInvalidCommand) || errors.Is(err, creation.ErrNotFound) || errors.Is(err, creation.ErrConflict) {
		t.Fatalf("got %v, want a decode error that is none of the package sentinels", err)
	}
	assertRevisionUnchanged(t, pool, id, v.Revision)
}

func TestActPastDeadlineBlocksNonCancelButCancelStillEndsTheSession(t *testing.T) {
	pool, ws, svc := newActFixture(t)
	v, id := newActSession(t, svc, ws)
	setCreationEnvelopeDeadline(t, pool, id, time.Now().Add(-time.Minute))

	_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "message", Message: "hello"})
	if !errors.Is(err, creation.ErrDeadline) {
		t.Fatalf("got %v, want ErrDeadline", err)
	}
	assertRevisionUnchanged(t, pool, id, v.Revision)

	out, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "cancel"})
	if err != nil {
		t.Fatalf("cancel past the deadline: %v", err)
	}
	if out.State != "cancelled" {
		t.Fatalf("state = %q, want cancelled", out.State)
	}
}

func TestActMessageOnAQueuedSessionIsAConflict(t *testing.T) {
	pool, ws, svc := newActFixture(t)
	id := creationID(t)
	v, err := svc.Create(context.Background(), ws, id, "hello", .5)
	if err != nil {
		t.Fatal(err)
	}
	if v.State != "queued" {
		t.Fatalf("fixture state = %q, want queued", v.State)
	}
	_, _, err = svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "message", Message: "more"})
	if !errors.Is(err, creation.ErrConflict) {
		t.Fatalf("got %v, want ErrConflict", err)
	}
	assertRevisionUnchanged(t, pool, id, v.Revision)
}

func TestActAcceptedCommandBacksUpTheExistingDraft(t *testing.T) {
	pool, ws, svc := newActFixture(t)
	v, id := newActSession(t, svc, ws)
	draft := creation.Draft{Revision: 1, ContentHash: "hash-before", Skill: creation.GeneratedSkill{Name: "n", Description: "d"}}
	setCreationSnapshotField(t, pool, id, "draft", draft)

	_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "message", Message: "continue please"})
	if err != nil {
		t.Fatal(err)
	}

	var probe struct {
		PreviousDraft *creation.Draft `json:"previous_draft"`
	}
	if err := json.Unmarshal(creationRowSnapshot(t, pool, id), &probe); err != nil {
		t.Fatal(err)
	}
	if probe.PreviousDraft == nil || !reflect.DeepEqual(*probe.PreviousDraft, draft) {
		t.Fatalf("previous draft = %+v, want %+v", probe.PreviousDraft, draft)
	}
}

func TestActCancelWithNoActiveReceiptStillEndsTheSession(t *testing.T) {
	_, ws, svc := newActFixture(t)
	v, id := newActSession(t, svc, ws)
	out, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "cancel"})
	if err != nil {
		t.Fatal(err)
	}
	if out.State != "cancelled" {
		t.Fatalf("state = %q, want cancelled", out.State)
	}
}

func TestActMessageInputValidation(t *testing.T) {
	pool, ws, svc := newActFixture(t)
	t.Run("whitespace only", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "message", Message: "   "})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("over the rune limit", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "message", Message: strings.Repeat("字", 4001)})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("at the rune limit is accepted", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		out, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "message", Message: strings.Repeat("字", 4000)})
		if err != nil {
			t.Fatalf("message at the 4000-rune limit: %v", err)
		}
		if out.State != "queued" || len(out.Snapshot.Messages) != 1 {
			t.Fatalf("state=%q messages=%d, want queued/1", out.State, len(out.Snapshot.Messages))
		}
	})
	t.Run("messages already at the cap", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		setCreationSnapshotField(t, pool, id, "messages", fillerMessages(creation.MaxMessages))
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "message", Message: "one more"})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
}

func TestActConfirmDiagram(t *testing.T) {
	pool, ws, svc := newActFixture(t)
	const validDescription = "從申請到核准"
	t.Run("not pending confirm_diagram", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "confirm_diagram"})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("invalid interpretation", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		setCreationSnapshotField(t, pool, id, "pending_action", "confirm_diagram")
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "confirm_diagram"})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("valid confirmation queues a step", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		setCreationSnapshotField(t, pool, id, "pending_action", "confirm_diagram")
		setCreationSnapshotField(t, pool, id, "diagram_fingerprint", "fingerprint")
		setCreationSnapshotField(t, pool, id, "diagram_description", validDescription)
		out, job, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "confirm_diagram"})
		if err != nil {
			t.Fatal(err)
		}
		if !out.Snapshot.DiagramDescriptionConfirmed || out.Snapshot.DiagramConfirmed || out.Snapshot.PendingAction != "" || out.State != "queued" {
			t.Fatalf("out = %+v, want description confirmed, interpretation unconfirmed, pending cleared and queued", out)
		}
		if job != nil {
			t.Fatalf("confirm_diagram is not transient, job = %+v", job)
		}
		var queued int
		if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM creation_receipts WHERE session_id=$1 AND status='queued'", id).Scan(&queued); err != nil {
			t.Fatal(err)
		}
		if queued != 1 {
			t.Fatalf("queued receipts = %d, want 1", queued)
		}
	})
}

func TestActSelectReferences(t *testing.T) {
	pool, ws, svc := newActFixture(t)
	okResolve := func(context.Context, identity.Workspace, string, string) (creation.Reference, creation.ReferenceSkill, error) {
		return creation.Reference{}, creation.ReferenceSkill{}, nil
	}
	failResolve := func(context.Context, identity.Workspace, string, string) (creation.Reference, creation.ReferenceSkill, error) {
		return creation.Reference{}, creation.ReferenceSkill{}, errors.New("not found upstream")
	}

	t.Run("more than three ids", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		svc.ResolveReference = okResolve
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "select_references", ReferenceSkillIDs: []string{"a", "b", "c", "d"}})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("no ResolveReference capability", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		svc.ResolveReference = nil
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "select_references", ReferenceSkillIDs: []string{"a"}})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("duplicate id in the request", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		svc.ResolveReference = okResolve
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "select_references", ReferenceSkillIDs: []string{"a", "a"}})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("ResolveReference errors", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		svc.ResolveReference = failResolve
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "select_references", ReferenceSkillIDs: []string{"a"}})
		if !errors.Is(err, creation.ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("note over the rune limit", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		called := false
		svc.ResolveReference = func(context.Context, identity.Workspace, string, string) (creation.Reference, creation.ReferenceSkill, error) {
			called = true
			return creation.Reference{}, creation.ReferenceSkill{}, nil
		}
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "select_references", Message: strings.Repeat("字", 4001)})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		if called {
			t.Fatal("ResolveReference was called before the over-limit note was rejected")
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
}

func TestActAdoptReferenceWhenAdoptErrorsIsNotFound(t *testing.T) {
	pool, ws, svc := newActFixture(t)
	v, id := newActSession(t, svc, ws)
	setCreationSnapshotField(t, pool, id, "pending_action", "confirm_references")
	setCreationSnapshotField(t, pool, id, "references", []creation.Reference{{SkillID: "adopt-fail-skill"}})
	svc.Adopt = func(context.Context, identity.Workspace, string) (creation.Candidate, error) {
		return creation.Candidate{}, errors.New("adopt boom")
	}
	_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "adopt_reference", ReferenceSkillIDs: []string{"adopt-fail-skill"}})
	if !errors.Is(err, creation.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
	assertRevisionUnchanged(t, pool, id, v.Revision)
}

func TestActDeclineReferences(t *testing.T) {
	pool, ws, svc := newActFixture(t)
	t.Run("not pending confirm_references", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "decline_references"})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("messages already at the cap", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		setCreationSnapshotField(t, pool, id, "pending_action", "confirm_references")
		setCreationSnapshotField(t, pool, id, "messages", fillerMessages(creation.MaxMessages))
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "decline_references"})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
}

func TestActConfirmReferences(t *testing.T) {
	pool, ws, svc := newActFixture(t)
	t.Run("not pending confirm_references", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		svc.ResolveReference = func(context.Context, identity.Workspace, string, string) (creation.Reference, creation.ReferenceSkill, error) {
			return creation.Reference{}, creation.ReferenceSkill{}, nil
		}
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "confirm_references"})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("no ResolveReference capability", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		setCreationSnapshotField(t, pool, id, "pending_action", "confirm_references")
		svc.ResolveReference = nil
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "confirm_references"})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("ResolveReference errors", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		setCreationSnapshotField(t, pool, id, "pending_action", "confirm_references")
		setCreationSnapshotField(t, pool, id, "references", []creation.Reference{{SkillID: "r1"}})
		svc.ResolveReference = func(context.Context, identity.Workspace, string, string) (creation.Reference, creation.ReferenceSkill, error) {
			return creation.Reference{}, creation.ReferenceSkill{}, errors.New("resolve boom")
		}
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "confirm_references"})
		if !errors.Is(err, creation.ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
}

func TestActDiagramValidation(t *testing.T) {
	pool, ws, svc := newActFixture(t)
	png := base64.StdEncoding.EncodeToString([]byte("png"))

	t.Run("nil diagram", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "diagram"})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("invalid base64", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "diagram", Diagram: &creation.Diagram{MediaType: "image/png", Data: "not-base64!!"}})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("empty bytes", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "diagram", Diagram: &creation.Diagram{MediaType: "image/png", Data: ""}})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("exactly the byte limit is accepted", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		data := base64.StdEncoding.EncodeToString(make([]byte, creation.MaxDiagramBytes))
		out, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "diagram", Diagram: &creation.Diagram{MediaType: "image/png", Data: data}})
		if err != nil {
			t.Fatalf("diagram at the byte limit: %v", err)
		}
		if out.Snapshot.DiagramBytes != creation.MaxDiagramBytes {
			t.Fatalf("diagram bytes = %d, want %d", out.Snapshot.DiagramBytes, creation.MaxDiagramBytes)
		}
	})
	t.Run("one byte over the limit is rejected", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		data := base64.StdEncoding.EncodeToString(make([]byte, creation.MaxDiagramBytes+1))
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "diagram", Diagram: &creation.Diagram{MediaType: "image/png", Data: data}})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("rejected media type", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "diagram", Diagram: &creation.Diagram{MediaType: "image/gif", Data: png}})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("note over the rune limit", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "diagram", Message: strings.Repeat("字", 4001), Diagram: &creation.Diagram{MediaType: "image/png", Data: png}})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
}

func TestActAttachRunPreconditions(t *testing.T) {
	pool, ws, svc := newActFixture(t)
	t.Run("no candidate", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		svc.ReadRun = failReadRun(t)
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "attach_run", RunID: "run-1"})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("ReadRun capability missing", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		setCreationSnapshotField(t, pool, id, "candidate", creation.Candidate{SkillID: "s1", VersionID: "v1"})
		svc.ReadRun = nil
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "attach_run", RunID: "run-1"})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("empty run id", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		setCreationSnapshotField(t, pool, id, "candidate", creation.Candidate{SkillID: "s1", VersionID: "v1"})
		svc.ReadRun = failReadRun(t)
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "attach_run", RunID: ""})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("messages already at the cap", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		setCreationSnapshotField(t, pool, id, "candidate", creation.Candidate{SkillID: "s1", VersionID: "v1"})
		setCreationSnapshotField(t, pool, id, "messages", fillerMessages(creation.MaxMessages))
		svc.ReadRun = failReadRun(t)
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "attach_run", RunID: "run-1"})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
}

func TestActConfirmFetch(t *testing.T) {
	pool, ws, svc := newActFixture(t)
	t.Run("not pending confirm_fetch", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "confirm_fetch"})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("empty pending fetch url", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		setCreationSnapshotField(t, pool, id, "pending_action", "confirm_fetch")
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "confirm_fetch"})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("valid confirmation queues a step", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		const url = "https://example.test/doc"
		setCreationSnapshotField(t, pool, id, "pending_action", "confirm_fetch")
		setCreationSnapshotField(t, pool, id, "pending_fetch_url", url)
		out, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "confirm_fetch"})
		if err != nil {
			t.Fatal(err)
		}
		if out.Snapshot.PendingAction != "" || out.Snapshot.PendingFetchURL != url || out.State != "queued" {
			t.Fatalf("out = %+v, want pending cleared/url kept/queued", out)
		}
	})
}

func TestActDeclineFetch(t *testing.T) {
	pool, ws, svc := newActFixture(t)
	const url = "https://example.test/doc"
	t.Run("not pending confirm_fetch", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "decline_fetch"})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("empty pending fetch url", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		setCreationSnapshotField(t, pool, id, "pending_action", "confirm_fetch")
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "decline_fetch"})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("valid decline records the fetch and queues a step", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		setCreationSnapshotField(t, pool, id, "pending_action", "confirm_fetch")
		setCreationSnapshotField(t, pool, id, "pending_fetch_url", url)
		out, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "decline_fetch"})
		if err != nil {
			t.Fatal(err)
		}
		if out.Snapshot.PendingAction != "" || out.Snapshot.PendingFetchURL != "" || out.State != "queued" {
			t.Fatalf("out = %+v, want pending and url cleared/queued", out)
		}
		if len(out.Snapshot.Fetches) != 1 || out.Snapshot.Fetches[0] != (creation.Fetch{URL: url, Status: "declined"}) {
			t.Fatalf("fetches = %+v, want one declined record for %s", out.Snapshot.Fetches, url)
		}
	})
	t.Run("messages already at the cap overflows the limit instead of being rejected up front", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		setCreationSnapshotField(t, pool, id, "pending_action", "confirm_fetch")
		setCreationSnapshotField(t, pool, id, "pending_fetch_url", url)
		setCreationSnapshotField(t, pool, id, "messages", fillerMessages(creation.MaxMessages))
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "decline_fetch"})
		if !errors.Is(err, creation.ErrLimit) {
			t.Fatalf("got %v, want ErrLimit (decline_fetch has no cap check of its own, so canSpend catches the overflow after the append)", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
}

func TestActRaiseBudgetOnAFailedSessionResumesWaitingInput(t *testing.T) {
	pool, ws, svc := newActFixture(t)
	v, id := newActSession(t, svc, ws)
	setCreationRowState(t, pool, id, "failed")
	out, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "raise_budget", BudgetUSD: .7})
	if err != nil {
		t.Fatal(err)
	}
	if out.Snapshot.BudgetUSD != .7 || out.State != "waiting_input" {
		t.Fatalf("out = %+v, want budget 0.7 / waiting_input", out)
	}
}

func TestActMaterializeGroupPreconditions(t *testing.T) {
	pool, ws, svc := newActFixture(t)
	confirmedBase := func(t *testing.T, id pgtype.UUID) {
		setCreationSnapshotField(t, pool, id, "brief_confirmed", true)
		setCreationSnapshotField(t, pool, id, "brief", "整理輸入並輸出摘要")
	}
	draft := func(hash string, blocked bool) creation.Draft {
		return creation.Draft{Revision: 1, ContentHash: hash, Skill: creation.GeneratedSkill{Name: "n", Description: "d"}, Blocked: blocked}
	}

	t.Run("no draft", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		confirmedBase(t, id)
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "materialize", ContentHash: "h1"})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("draft blocked", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		confirmedBase(t, id)
		setCreationSnapshotField(t, pool, id, "draft", draft("h1", true))
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "materialize", ContentHash: "h1"})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("empty content hash", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		confirmedBase(t, id)
		setCreationSnapshotField(t, pool, id, "draft", draft("", false))
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "materialize", ContentHash: ""})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("content hash mismatch", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		confirmedBase(t, id)
		setCreationSnapshotField(t, pool, id, "draft", draft("h1", false))
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "materialize", ContentHash: "h2"})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("not confirmed", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		setCreationSnapshotField(t, pool, id, "draft", draft("h1", false))
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "materialize", ContentHash: "h1"})
		if !errors.Is(err, creation.ErrInvalidCommand) {
			t.Fatalf("got %v, want ErrInvalidCommand", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("references present without a ResolveReference capability", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		confirmedBase(t, id)
		setCreationSnapshotField(t, pool, id, "draft", draft("h1", false))
		setCreationSnapshotField(t, pool, id, "references", []creation.Reference{{SkillID: "r1", Confirmed: true, Available: true}})
		svc.ResolveReference = nil
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "materialize", ContentHash: "h1"})
		if !errors.Is(err, creation.ErrUnavailable) {
			t.Fatalf("got %v, want ErrUnavailable", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("ResolveReference errors", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		confirmedBase(t, id)
		setCreationSnapshotField(t, pool, id, "draft", draft("h1", false))
		setCreationSnapshotField(t, pool, id, "references", []creation.Reference{{SkillID: "r1", Confirmed: true, Available: true}})
		svc.ResolveReference = func(context.Context, identity.Workspace, string, string) (creation.Reference, creation.ReferenceSkill, error) {
			return creation.Reference{}, creation.ReferenceSkill{}, errors.New("resolve boom")
		}
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "materialize", ContentHash: "h1"})
		if !errors.Is(err, creation.ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
	t.Run("no materialize capability and no existing candidate", func(t *testing.T) {
		v, id := newActSession(t, svc, ws)
		confirmedBase(t, id)
		setCreationSnapshotField(t, pool, id, "draft", draft("h1", false))
		svc.ResolveReference = nil
		_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "materialize", ContentHash: "h1"})
		if !errors.Is(err, creation.ErrUnavailable) {
			t.Fatalf("got %v, want ErrUnavailable", err)
		}
		assertRevisionUnchanged(t, pool, id, v.Revision)
	})
}

func TestActUnknownKindIsInvalidCommand(t *testing.T) {
	pool, ws, svc := newActFixture(t)
	v, id := newActSession(t, svc, ws)
	_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "not-a-real-kind"})
	if !errors.Is(err, creation.ErrInvalidCommand) {
		t.Fatalf("got %v, want ErrInvalidCommand", err)
	}
	assertRevisionUnchanged(t, pool, id, v.Revision)
}

func TestActWhenInsertFailsTheSessionIsUnchanged(t *testing.T) {
	pool, ws, svc := newActFixture(t)
	v, id := newActSession(t, svc, ws)
	sentinel := errors.New("insert boom")
	svc.Insert = func(context.Context, pgx.Tx, creation.JobArgs) error {
		return sentinel
	}
	_, _, err := svc.Act(context.Background(), ws, id, creation.Command{ID: creationID(t), ExpectedRevision: v.Revision, Kind: "message", Message: "please continue"})
	if !errors.Is(err, sentinel) {
		t.Fatalf("got %v, want %v", err, sentinel)
	}
	var state string
	var revision int64
	if err := pool.QueryRow(context.Background(), "SELECT state, revision FROM creation_sessions WHERE id=$1", id).Scan(&state, &revision); err != nil {
		t.Fatal(err)
	}
	if state != v.State || revision != v.Revision {
		t.Fatalf("session moved to state=%q revision=%d, want unchanged from state=%q revision=%d", state, revision, v.State, v.Revision)
	}
}
