package sandbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

const traceBatchLimit = 200

type failingPushSink struct {
	recordingSink
	pushMu     sync.Mutex
	pushes     int
	failOnPush int
}

func (s *failingPushSink) Push(ctx context.Context, url string, events []json.RawMessage) error {
	s.pushMu.Lock()
	s.pushes++
	failThis := s.pushes == s.failOnPush
	s.pushMu.Unlock()
	if failThis {
		return errors.New("ingestion unavailable")
	}
	return s.recordingSink.Push(ctx, url, events)
}

func (s *recordingSink) batchSizes() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	sizes := make([]int, 0, len(s.batches))
	for _, batch := range s.batches {
		sizes = append(sizes, len(batch))
	}
	return sizes
}

func numberedEvents(n int) []string {
	events := make([]string, 0, n)
	for seq := 1; seq <= n; seq++ {
		events = append(events, fmt.Sprintf(`{"event_id":"e-%d","seq":%d}`, seq, seq))
	}
	return events
}

func runTraceToCompletion(t *testing.T, sink sandbox.TraceSink, write func(drv *fakeDriver, id string)) {
	t.Helper()
	drv, h := newTracingServer(t, sink)
	_, run := do(t, h, "POST", "/runs", tracedRequest(), testToken)
	write(drv, run.ProviderRunID)
	drv.exit(run.ProviderRunID, sandbox.Outcome{ExitCode: 0})
	waitForTerminal(t, h, run.ProviderRunID)
}

func TestTraceBatchesHoldAtMostTheBatchLimit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		events int
		want   []int
	}{
		{"exactly the limit is one batch", traceBatchLimit, []int{traceBatchLimit}},
		{"one past the limit spills into a second batch", traceBatchLimit + 1, []int{traceBatchLimit, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sink := &recordingSink{}
			runTraceToCompletion(t, sink, func(drv *fakeDriver, id string) {
				drv.writeTrace(id, numberedEvents(tc.events)...)
			})
			waitFor(t, func() bool { return len(sink.received()) >= tc.events })
			if got := sink.batchSizes(); !slices.Equal(got, tc.want) {
				t.Fatalf("batch sizes = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAFailedLaterBatchDoesNotResendAnEarlierOne(t *testing.T) {
	sink := &failingPushSink{failOnPush: 2}
	want := numberedEvents(traceBatchLimit + 1)
	runTraceToCompletion(t, sink, func(drv *fakeDriver, id string) {
		drv.writeTrace(id, want...)
	})

	waitFor(t, func() bool { return len(sink.received()) >= len(want) })
	if got := sink.received(); !slices.Equal(got, want) {
		t.Fatalf("received %d events after a retried second batch, want each of the %d exactly once", len(got), len(want))
	}
}

func TestAWindowOfOnlyUnreadableLinesDoesNotPinTheValidTail(t *testing.T) {
	sink := &recordingSink{}
	tail := `{"event_id":"tail","seq":1}`
	runTraceToCompletion(t, sink, func(drv *fakeDriver, id string) {
		drv.appendRawTrace(id, strings.Repeat(strings.Repeat("x", 1023)+"\n", 9<<10))
		drv.appendRawTrace(id, tail+"\n")
	})

	waitFor(t, func() bool { return len(sink.received()) >= 1 })
	if got := sink.received(); !slices.Equal(got, []string{tail}) {
		t.Fatalf("received %d events, want only the valid tail", len(got))
	}
}

func TestAnUnterminatedLineWiderThanTheReadWindowDoesNotPinTheValidTail(t *testing.T) {
	sink := &recordingSink{}
	tail := `{"event_id":"tail","seq":1}`
	runTraceToCompletion(t, sink, func(drv *fakeDriver, id string) {
		drv.appendRawTrace(id, `{"padding":"`+strings.Repeat("x", 9<<20)+`"}`+"\n")
		drv.appendRawTrace(id, tail+"\n")
	})

	waitFor(t, func() bool { return len(sink.received()) >= 1 })
	if got := sink.received(); !slices.Equal(got, []string{tail}) {
		t.Fatalf("received %d events, want only the valid tail", len(got))
	}
}
