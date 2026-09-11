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

type sseEvent struct {
	ID   int64
	View creation.View
}

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

func TestCreationStreamCarriesTheSameDocumentAsGet(t *testing.T) {
	a, _, _ := creationFixture(t)
	c := a.login(t, "creation-stream-shape")
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_credits": 650}, 200)

	events, stop := readSSE(t, c, v.ID, "")
	defer stop()

	first := waitEvent(t, events, "the stream's opening document")
	if first.ID != first.View.Revision {
		t.Errorf("id: %d does not match the document's revision %d", first.ID, first.View.Revision)
	}
	if first.View.ID != v.ID {
		t.Errorf("streamed a different session: %s want %s", first.View.ID, v.ID)
	}

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

func TestCreationStreamDeliversTheNextStepWithoutPolling(t *testing.T) {
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-stream-step")
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_credits": 650}, 200)

	events, stop := readSSE(t, c, v.ID, "")
	defer stop()
	first := waitEvent(t, events, "the opening document")

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

func TestCreationStreamResumesFromLastEventID(t *testing.T) {
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-stream-resume")
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_credits": 650}, 200)
	v = creationStep(t, s, v)

	events, stop := readSSE(t, c, v.ID, strconv.FormatInt(v.Revision, 10))
	select {
	case e, ok := <-events:
		if ok {
			t.Fatalf("resumed at revision %d and was sent %d anyway", v.Revision, e.View.Revision)
		}
	case <-time.After(1500 * time.Millisecond):

	}
	stop()

	events, stop = readSSE(t, c, v.ID, strconv.FormatInt(v.Revision-1, 10))
	defer stop()
	e := waitEvent(t, events, "the one revision this client is missing")
	if e.View.Revision != v.Revision {
		t.Errorf("resumed at %d and got revision %d, want %d", v.Revision-1, e.View.Revision, v.Revision)
	}
}

func TestCreationStreamEndsWhenTheSessionDoes(t *testing.T) {
	a, s, _ := creationFixture(t)
	c := a.login(t, "creation-stream-ends")
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_credits": 650}, 200)
	v = creationStep(t, s, v)
	v = creationAct(t, c, v, "cancel")
	if v.State != "cancelled" {
		t.Fatalf("cancel did not cancel: %s", v.State)
	}

	events, stop := readSSE(t, c, v.ID, "")
	defer stop()

	for {
		select {
		case _, ok := <-events:
			if !ok {
				return
			}
		case <-time.After(10 * time.Second):
			t.Fatal("the stream stayed open on a cancelled session")
		}
	}
}

func TestCreationStreamRefusesAnotherWorkspace(t *testing.T) {
	a, _, _ := creationFixture(t)
	mine := a.login(t, "creation-stream-owner")
	v := creationPost(t, mine, "/creation-sessions", map[string]any{"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_credits": 650}, 200)

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
