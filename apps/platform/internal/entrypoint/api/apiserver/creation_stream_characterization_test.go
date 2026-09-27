package apiserver_test

import (
	"testing"
	"time"
)

func TestCreationStreamDoesNotResendADocumentItAlreadySent(t *testing.T) {
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-stream-no-resend")
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_credits": 650}, 200)
	v = creationStep(t, s, v)

	events, stop := readSSE(t, c, v.ID, "")
	defer stop()
	first := waitEvent(t, events, "the opening document")
	if first.View.Revision != v.Revision {
		t.Fatalf("opened at revision %d, want the session's %d", first.View.Revision, v.Revision)
	}
	select {
	case e, ok := <-events:
		if ok {
			t.Fatalf("nothing changed, yet revision %d was sent again", e.View.Revision)
		}
		t.Fatal("the stream closed on a session that has not ended")
	case <-time.After(1500 * time.Millisecond):
	}
}

func TestCreationStreamAlreadyOpenRelaysTheEndingAndCloses(t *testing.T) {
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-stream-open-ends")
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_credits": 650}, 200)
	v = creationStep(t, s, v)

	events, stop := readSSE(t, c, v.ID, "")
	defer stop()
	last := waitEvent(t, events, "the opening document")
	if ended := creationAct(t, c, v, "cancel"); ended.State != "cancelled" {
		t.Fatalf("cancel did not cancel: %s", ended.State)
	}

	deadline := time.After(10 * time.Second)
	for {
		select {
		case e, ok := <-events:
			if !ok {
				if last.View.State != "cancelled" {
					t.Fatalf("the stream closed after sending state %q, want the cancelled document first", last.View.State)
				}
				return
			}
			last = e
		case <-deadline:
			t.Fatalf("the stream stayed open after its session ended (last state %q)", last.View.State)
		}
	}
}
