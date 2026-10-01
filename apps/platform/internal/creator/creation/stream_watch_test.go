package creation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type fakeRevisions struct {
	mu        sync.Mutex
	revisions map[[16]byte]int64
	calls     []int
	polled    chan struct{}
}

func newFakeRevisions() *fakeRevisions {
	return &fakeRevisions{revisions: map[[16]byte]int64{}, polled: make(chan struct{}, 1000)}
}

func (f *fakeRevisions) read(_ context.Context, workspaces, sessions []pgtype.UUID) ([]gen.CreationSessionRevisionsRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, len(sessions))
	var rows []gen.CreationSessionRevisionsRow
	for i, session := range sessions {
		if revision, ok := f.revisions[session.Bytes]; ok {
			rows = append(rows, gen.CreationSessionRevisionsRow{ID: session, WorkspaceID: workspaces[i], Revision: revision})
		}
	}
	f.polled <- struct{}{}
	return rows, nil
}

func (f *fakeRevisions) set(session pgtype.UUID, revision int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revisions[session.Bytes] = revision
}

func (f *fakeRevisions) remove(session pgtype.UUID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.revisions, session.Bytes)
}

func (f *fakeRevisions) forgetPolls() {
	for len(f.polled) > 0 {
		<-f.polled
	}
}

func (f *fakeRevisions) awaitPolls(t *testing.T, n int) {
	t.Helper()
	for range n {
		select {
		case <-f.polled:
		case <-time.After(5 * time.Second):
			t.Fatal("the watch stopped polling")
		}
	}
}

func uuidN(n byte) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{15: n, 14: n >> 4}, Valid: true}
}

func workspaceN(n int) pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{0: 1, 1: byte(n >> 8), 2: byte(n)}, Valid: true}
}

func drain(ch <-chan struct{}) {
	select {
	case <-ch:
	default:
	}
}

func notified(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func TestEveryOpenStreamIsWatchedByOneQueryPerTick(t *testing.T) {
	fake := newFakeRevisions()
	w := newRevisionWatch(fake.read, time.Millisecond)
	const streams = 50
	for i := range streams {
		session := uuidN(byte(i + 1))
		fake.set(session, 1)
		_, stop, err := w.Subscribe(workspaceN(i), session)
		if err != nil {
			t.Fatal(err)
		}
		defer stop()
	}
	fake.awaitPolls(t, 5)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, watched := range fake.calls[len(fake.calls)-3:] {
		if watched != streams {
			t.Fatalf("a poll read %d sessions; one query per tick must cover all %d streams", watched, streams)
		}
	}
}

func TestOnlyTheSessionThatMovedIsNotified(t *testing.T) {
	fake := newFakeRevisions()
	w := newRevisionWatch(fake.read, time.Millisecond)
	moved, still := uuidN(1), uuidN(2)
	fake.set(moved, 1)
	fake.set(still, 1)
	movedCh, stopMoved, _ := w.Subscribe(workspaceN(1), moved)
	defer stopMoved()
	stillCh, stopStill, _ := w.Subscribe(workspaceN(2), still)
	defer stopStill()
	fake.awaitPolls(t, 2)
	drain(movedCh)
	drain(stillCh)

	fake.forgetPolls()
	fake.set(moved, 2)
	fake.awaitPolls(t, 3)
	if !notified(movedCh) {
		t.Fatal("the session whose revision moved was not notified")
	}
	if notified(stillCh) {
		t.Fatal("a session whose revision did not move was notified")
	}
}

func TestADeletedSessionIsNotifiedSoItsStreamCanClose(t *testing.T) {
	fake := newFakeRevisions()
	w := newRevisionWatch(fake.read, time.Millisecond)
	session := uuidN(1)
	fake.set(session, 1)
	ch, stop, _ := w.Subscribe(workspaceN(1), session)
	defer stop()
	fake.awaitPolls(t, 2)
	drain(ch)

	fake.forgetPolls()
	fake.remove(session)
	fake.awaitPolls(t, 3)
	if !notified(ch) {
		t.Fatal("a deleted session's stream was never told to look")
	}
}

func TestAWorkspaceOpensAtMostItsShareOfStreams(t *testing.T) {
	w := newRevisionWatch(newFakeRevisions().read, time.Hour)
	ws := workspaceN(1)
	var stops []func()
	for i := range MaxStreamsPerWorkspace {
		_, stop, err := w.Subscribe(ws, uuidN(byte(i+1)))
		if err != nil {
			t.Fatalf("stream %d of %d was refused: %v", i+1, MaxStreamsPerWorkspace, err)
		}
		stops = append(stops, stop)
	}
	if _, _, err := w.Subscribe(ws, uuidN(99)); !errors.Is(err, ErrTooManyStreams) {
		t.Fatalf("stream %d err = %v, want ErrTooManyStreams", MaxStreamsPerWorkspace+1, err)
	}
	if _, stop, err := w.Subscribe(workspaceN(2), uuidN(99)); err != nil {
		t.Fatalf("another workspace was refused because of this one: %v", err)
	} else {
		stop()
	}
	stops[0]()
	stops[0]()
	if _, _, err := w.Subscribe(ws, uuidN(99)); err != nil {
		t.Fatalf("a closed stream did not give its place back: %v", err)
	}
	if _, _, err := w.Subscribe(ws, uuidN(98)); !errors.Is(err, ErrTooManyStreams) {
		t.Fatal("stopping one stream twice gave back two places")
	}
}

func TestTheProcessOpensAtMostItsStreamCeiling(t *testing.T) {
	w := newRevisionWatch(newFakeRevisions().read, time.Hour)
	for i := range MaxStreamsPerProcess {
		if _, _, err := w.Subscribe(workspaceN(i), uuidN(1)); err != nil {
			t.Fatalf("stream %d of %d was refused: %v", i+1, MaxStreamsPerProcess, err)
		}
	}
	if _, _, err := w.Subscribe(workspaceN(MaxStreamsPerProcess), uuidN(1)); !errors.Is(err, ErrTooManyStreams) {
		t.Fatalf("stream %d err = %v, want ErrTooManyStreams", MaxStreamsPerProcess+1, err)
	}
}

func TestTheWatchStopsQueryingOnceNoStreamIsOpen(t *testing.T) {
	fake := newFakeRevisions()
	w := newRevisionWatch(fake.read, time.Millisecond)
	_, stop, _ := w.Subscribe(workspaceN(1), uuidN(1))
	fake.awaitPolls(t, 2)
	stop()
	time.Sleep(20 * time.Millisecond)
	fake.forgetPolls()
	time.Sleep(20 * time.Millisecond)
	if n := len(fake.polled); n != 0 {
		t.Fatalf("%d queries ran with no stream open", n)
	}
}
