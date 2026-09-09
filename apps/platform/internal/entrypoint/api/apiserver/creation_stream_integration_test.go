package apiserver_test

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
)

// sseEvent is one `id:`/`data:` pair off the wire.
type sseEvent struct {
	ID   int64
	View creation.View
}

// readSSE opens the stream and returns events as they arrive. The reader runs
// in its own goroutine because the point of the endpoint is that the body does
// not end when the first document does.
func readSSE(t *testing.T, c *client, sessionID, lastEventID string) (<-chan sseEvent, func()) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, c.base+"/creation-sessions/"+sessionID+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		res.Body.Close()
		t.Fatalf("GET events: %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		res.Body.Close()
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}
	out := make(chan sseEvent, 16)
	go func() {
		defer close(out)
		defer res.Body.Close()
		sc := bufio.NewScanner(res.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 8<<20)
		var id int64
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "id: "):
				id, _ = strconv.ParseInt(strings.TrimPrefix(line, "id: "), 10, 64)
			case strings.HasPrefix(line, "data: "):
				var v creation.View
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &v); err != nil {
					return
				}
				out <- sseEvent{ID: id, View: v}
			}
		}
	}()
	return out, func() { res.Body.Close() }
}

func waitEvent(t *testing.T, ch <-chan sseEvent, why string) sseEvent {
	t.Helper()
	select {
	case e, ok := <-ch:
		if !ok {
			t.Fatalf("%s: the stream closed instead", why)
		}
		return e
	case <-time.After(10 * time.Second):
		t.Fatalf("%s: nothing arrived in 10s", why)
	}
	return sseEvent{}
}

// ADR-069 / 05 R-71. What the contract promises about this endpoint, asserted
// against the wire: each event's `data:` is one CreationSession — the same
// document GET returns — and its `id:` is the revision.
//
// This test IS the schema. public.yaml declares no body schema for this
// operation, deliberately: OpenAPI can describe a document, not a stream of
// them, and naming CreationSession as the body would assert that the body IS
// one of those. So the payload is pinned here instead, by unmarshalling what
// the handler writes into the same type GET's response is built from, and
// comparing the two documents byte for byte.
func TestCreationStreamCarriesTheSameDocumentAsGet(t *testing.T) {
	a, _, _ := creationFixture(t)
	c := a.login(t, "creation-stream-shape")
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_usd": .5}, 200)

	events, stop := readSSE(t, c, v.ID, "")
	defer stop()

	first := waitEvent(t, events, "the stream's opening document")
	if first.ID != first.View.Revision {
		t.Errorf("id: %d does not match the document's revision %d", first.ID, first.View.Revision)
	}
	if first.View.ID != v.ID {
		t.Errorf("streamed a different session: %s want %s", first.View.ID, v.ID)
	}

	// The same read through the route the contract DOES describe. Compared as
	// JSON because that is what a client actually compares.
	res, err := c.Get(c.base + "/creation-sessions/" + v.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var got creation.View
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(got)
	have, _ := json.Marshal(first.View)
	if string(want) != string(have) {
		t.Errorf("the streamed document differs from GET's:\n stream %s\n get    %s", have, want)
	}
}

// A step the person did not cause still reaches the screen, and it reaches it
// as a new revision rather than a repeat of the old one.
func TestCreationStreamDeliversTheNextStepWithoutPolling(t *testing.T) {
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-stream-step")
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_usd": .5}, 200)

	events, stop := readSSE(t, c, v.ID, "")
	defer stop()
	first := waitEvent(t, events, "the opening document")

	// The worker settles the queued attempt. Nothing tells the handler; it is
	// watching the row the same way job.go's cancellation watcher does.
	after := creationStep(t, s, v)

	var last sseEvent
	deadline := time.After(15 * time.Second)
	for last.View.Revision < after.Revision {
		select {
		case e, ok := <-events:
			if !ok {
				t.Fatal("the stream closed before the step arrived")
			}
			last = e
		case <-deadline:
			t.Fatalf("revision %d never arrived (stream stopped at %d)", after.Revision, last.View.Revision)
		}
	}
	if last.View.Revision <= first.View.Revision {
		t.Fatalf("no revision after the opening one: %d", last.View.Revision)
	}
	if len(last.View.Snapshot.Messages) <= len(first.View.Snapshot.Messages) {
		t.Error("the streamed document did not grow the conversation the step wrote")
	}
}

// Last-Event-ID is the whole resume story: a reconnecting client says what it
// has and is sent nothing it already saw. Without it the browser's own
// reconnect would re-render the same state on every drop.
func TestCreationStreamResumesFromLastEventID(t *testing.T) {
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-stream-resume")
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_usd": .5}, 200)
	v = creationStep(t, s, v)

	// Claim to already have the current revision: there is nothing new, so the
	// stream must sit quiet rather than repeat itself.
	events, stop := readSSE(t, c, v.ID, strconv.FormatInt(v.Revision, 10))
	select {
	case e, ok := <-events:
		if ok {
			t.Fatalf("resumed at revision %d and was sent %d anyway", v.Revision, e.View.Revision)
		}
	case <-time.After(1500 * time.Millisecond):
		// Correct: silence.
	}
	stop()

	// One revision behind: exactly the missed one arrives.
	events, stop = readSSE(t, c, v.ID, strconv.FormatInt(v.Revision-1, 10))
	defer stop()
	e := waitEvent(t, events, "the one revision this client is missing")
	if e.View.Revision != v.Revision {
		t.Errorf("resumed at %d and got revision %d, want %d", v.Revision-1, e.View.Revision, v.Revision)
	}
}

// The stream is not immortal: it ends when the session reaches a state no
// command can leave. A page holding a socket open on a saved session is a leak
// that only shows up in production.
func TestCreationStreamEndsWhenTheSessionDoes(t *testing.T) {
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-stream-ends")
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_usd": .5}, 200)
	v = creationStep(t, s, v)
	v = creationAct(t, c, v, "cancel")
	if v.State != "cancelled" {
		t.Fatalf("cancel did not cancel: %s", v.State)
	}

	events, stop := readSSE(t, c, v.ID, "")
	defer stop()
	// It may send the terminal document first; what it must not do is stay open.
	for {
		select {
		case _, ok := <-events:
			if !ok {
				return // closed, which is the assertion
			}
		case <-time.After(10 * time.Second):
			t.Fatal("the stream stayed open on a cancelled session")
		}
	}
}

// Another workspace's session is not readable through a stream either. The
// route carries the same RequireSession/RequireInvited as the GET beside it;
// this pins the workspace scope the handler itself applies (iron rule 3).
func TestCreationStreamRefusesAnotherWorkspace(t *testing.T) {
	a, _, _ := creationFixture(t)
	mine := a.login(t, "creation-stream-owner")
	v := creationPost(t, mine, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_usd": .5}, 200)

	other := a.login(t, "creation-stream-stranger")
	res, err := other.Get(other.base + "/creation-sessions/" + v.ID + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(res.Body)
		t.Fatalf("a stranger got %d from another workspace's stream: %s", res.StatusCode, body)
	}
}
